// Package vm coordinates explicitly requested guest execution. It does not own Git creation.
package vm

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kamaal111/namu/internal/vm/agents"
	"github.com/kamaal111/namu/internal/vm/guest"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"github.com/kamaal111/namu/internal/vm/resources"
	"github.com/kamaal111/namu/internal/vm/state"
	"github.com/kamaal111/namu/internal/vm/tart"
)

type Confirm func(context.Context, string) error

// Manager's dependencies are scoped to one invocation; a shell never holds its lifecycle lock.
type Manager struct {
	Store              state.Store
	Tart               tart.Provider
	Runner             host.Runner
	Confirm            Confirm
	Input              io.Reader
	Output             io.Writer
	NoReuseCredentials bool
	SourceDirectory    string
	Resources          resources.Options
	profile            string
	profileVM          state.VM
}

func DefaultData() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "namu", "vm"), nil
}
func New(data string, r host.Runner, confirm Confirm, in io.Reader, out io.Writer) *Manager {
	return &Manager{Store: state.Store{Data: data}, Tart: tart.Provider{Runner: r, Data: data}, Runner: r, Confirm: confirm, Input: in, Output: out}
}
func ValidateOS(osName string) error {
	_, err := registry.Resolve(osName)
	return err
}
func (m *Manager) approve(ctx context.Context, summary string) error {
	if m.Confirm == nil {
		return fmt.Errorf("interactive confirmation required: %s", summary)
	}
	if err := m.Confirm(ctx, summary); err != nil {
		return err
	}
	return ctx.Err()
}
func (m *Manager) progress(format string, args ...any) {
	if m.Output != nil {
		fmt.Fprintf(m.Output, format+"\n", args...)
	}
}

func (m *Manager) Exists(name string) (bool, error) {
	r, err := m.Store.Read()
	if err != nil {
		return false, err
	}
	for _, v := range r.VMs {
		if v.Name == name {
			return true, nil
		}
	}
	return false, nil
}
func (m *Manager) save(r *state.Registry, index int, v state.VM) error {
	entries := append([]state.VM(nil), r.VMs...)
	entries[index] = v
	updated := state.Registry{Version: r.Version, VMs: entries}
	if err := m.Store.Write(updated); err != nil {
		return err
	}
	*r = updated
	return nil
}

// Ensure creates only explicitly allowed defaults. Named VM selection never creates implicitly.
func (m *Manager) Ensure(ctx context.Context, name, osName string, roots []string, allowCreate bool) (state.VM, error) {
	return m.ensure(ctx, name, osName, roots, allowCreate, false)
}
func (m *Manager) ensure(ctx context.Context, name, osName string, roots []string, allowCreate, createOnly bool) (state.VM, error) {
	if err := m.Resources.Validate(); err != nil {
		return state.VM{}, err
	}
	if err := state.ValidateName(name); err != nil {
		return state.VM{}, err
	}
	if osName != "" {
		if err := ValidateOS(osName); err != nil {
			return state.VM{}, err
		}
	}
	roots, err := ShareRoots(roots)
	if err != nil {
		return state.VM{}, err
	}
	unlock, err := m.Store.Lock(ctx)
	if err != nil {
		return state.VM{}, err
	}
	defer unlock()
	r, err := m.Store.Read()
	if err != nil {
		return state.VM{}, err
	}
	index, lookupErr := state.Find(r, name)
	var v state.VM
	creating := lookupErr != nil
	if !creating && createOnly {
		return v, fmt.Errorf("VM %s already exists; use namu vm start %s", name, name)
	}
	if creating {
		if !allowCreate {
			return v, lookupErr
		}
		if osName == "" {
			osName = registry.Default().Name
		}
		pins, err := tart.Pins()
		if err != nil {
			return v, err
		}
		if pins.Images[osName] == "" {
			return v, fmt.Errorf("missing VM image pin for guest OS %s", osName)
		}
		v = state.VM{ID: rand.Text(), Name: name, OS: osName, Image: pins.Images[osName], Stage: "creating"}
	} else {
		v = r.VMs[index]
		if osName != "" && osName != v.OS {
			return v, fmt.Errorf("VM %s runs %s; incompatible with --os %s", name, v.OS, osName)
		}
	}
	definition, err := registry.Resolve(v.OS)
	if err != nil {
		return v, err
	}
	previous := v.Resources
	if previous == (resources.Settings{}) {
		previous = definition.Resources
	}
	desiredResources := m.Resources.Apply(previous)
	if !creating && desiredResources.DiskGB < previous.DiskGB {
		return v, fmt.Errorf("cannot shrink VM %s disk from %d to %d GB; Tart only supports growing disks", v.Name, previous.DiskGB, desiredResources.DiskGB)
	}
	resourcesChanged := !creating && desiredResources != previous
	if creating {
		v.Resources = desiredResources
	}
	if err := m.Tart.GuestHostCheck(ctx, definition.Host); err != nil {
		return v, err
	}
	install, err := m.Tart.NeedsInstall(ctx)
	if err != nil {
		return v, err
	}
	if creating || v.Stage != "ready" || install || v.ToolsRevision != agents.Revision {
		summary := fmt.Sprintf("Prepare persistent VM %s (%s): %s.\nImage: %s\nConfigure dedicated SSH access, Git, directory mounting, Claude, Codex and Cursor. Downloads may be several GB.", v.Name, v.OS, desiredResources, v.Image)
		if !m.NoReuseCredentials {
			summary += "\nVM sessions reuse supported host credentials by default; --no-reuse-credentials selects guest-only logins."
		}
		if install {
			summary += "\nDownload checksum-verified private Tart; license: https://tart.run/licensing/"
		}
		if err := m.approve(ctx, summary); err != nil {
			return v, err
		}
		if install {
			if err := m.Tart.Install(ctx, m.Output); err != nil {
				return v, err
			}
		}
	}
	live, err := m.Tart.List(ctx)
	if err != nil {
		return v, err
	}
	current, found := live[v.Name]
	if creating {
		if found {
			return v, fmt.Errorf("Tart VM %s already exists; refusing automatic adoption", v.Name)
		}
		r.VMs = append(r.VMs, v)
		index = len(r.VMs) - 1
		if err := m.Store.Write(r); err != nil {
			return v, err
		}
	}
	if !found && v.Stage != "creating" {
		return v, fmt.Errorf("owned VM %s is missing; refusing replacement", v.Name)
	}
	if v.Stage == "creating" {
		if found {
			if current.Running {
				return v, fmt.Errorf("incomplete VM %s is running; stop it before retrying setup", v.Name)
			}
			if err := m.Tart.Configure(ctx, v.Name, desiredResources); err != nil {
				return v, err
			}
		} else {
			m.progress("Downloading image and creating VM %s", v.Name)
			if err := m.Tart.Clone(ctx, v.Image, v.Name, desiredResources, m.Output); err != nil {
				return v, fmt.Errorf("creation incomplete; VM %s preserved; retry with namu vm start %s: %w", v.Name, v.Name, err)
			}
		}
		v.Stage = "cloned"
		v.Resources = desiredResources
		resourcesChanged = false
		if err := m.save(&r, index, v); err != nil {
			return v, err
		}
	}
	desired, err := ShareRoots(append(append([]string(nil), v.Shares...), roots...))
	if err != nil {
		return v, err
	}
	if resourcesChanged || (current.Running && !slices.Equal(desired, v.Applied)) {
		summary := fmt.Sprintf("Configure VM %s with %s.\nRepository directories shared read/write:\n%s", v.Name, desiredResources, strings.Join(desired, "\n"))
		if current.Running {
			summary += "\nRestart required. All processes in this guest will stop."
		}
		if err := m.approve(ctx, summary); err != nil {
			return v, err
		}
		if current.Running {
			if err := m.Tart.Stop(ctx, v.Name); err != nil {
				return v, err
			}
			current.Running = false
		}
	}
	if resourcesChanged {
		if err := m.Tart.Configure(ctx, v.Name, desiredResources); err != nil {
			return v, err
		}
	}
	v.Resources = desiredResources
	v.Shares = desired
	if err := m.save(&r, index, v); err != nil {
		return v, err
	}
	if !current.Running {
		// Persist an unknown applied set before spawning: an uncertain startup must never be adopted as correctly mounted.
		v.Applied = nil
		if err := m.save(&r, index, v); err != nil {
			return v, err
		}
		for _, root := range desired {
			if info, err := os.Stat(root); err != nil || !info.IsDir() {
				return v, fmt.Errorf("VM share unavailable: %s; restore it before starting the VM", root)
			}
		}
		if err := m.Tart.Start(ctx, v.Name, desired); err != nil {
			return v, fmt.Errorf("VM startup uncertain; inspect namu vm list before retrying: %w", err)
		}
		v.Applied = append([]string(nil), desired...)
		if err := m.save(&r, index, v); err != nil {
			return v, err
		}
	}
	boot, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	if err := m.Tart.Ready(boot, v.Name); err != nil {
		return v, err
	}
	connection, err := m.connection(boot, v)
	if err != nil {
		return v, err
	}
	publicKey, err := guest.Identity(ctx, m.Store.Data, m.Runner)
	if err != nil {
		return v, err
	}
	if v.Stage == "cloned" {
		key, err := m.Tart.ExecRoot(ctx, v.Name, guest.BootstrapScript(definition, publicKey), m.Output)
		if err != nil {
			return v, fmt.Errorf("SSH bootstrap failed; VM %s preserved: %w", v.Name, err)
		}
		if err := connection.EnrollPublicKey(ctx, string(key), true); err != nil {
			return v, err
		}
		v.Stage = "ssh"
		if err := m.save(&r, index, v); err != nil {
			return v, err
		}
	}
	// Upstream guest boot services may reset authorized_keys on each VM start.
	// Reauthorize only our dedicated public key; the enrolled host key stays unchanged.
	if _, err := m.Tart.ExecRoot(ctx, v.Name, guest.AccessScript(definition, publicKey), m.Output); err != nil {
		return v, fmt.Errorf("restore dedicated guest SSH access: %w", err)
	}
	if err := connection.Ready(boot); err != nil {
		return v, err
	}
	if v.Stage == "ssh" {
		m.progress("Installing shell essentials in %s", v.Name)
		if err := connection.Essentials(ctx, m.Output); err != nil {
			return v, fmt.Errorf("guest essentials failed; retry namu vm start %s: %w", v.Name, err)
		}
		v.Stage = "ready"
		if err := m.save(&r, index, v); err != nil {
			return v, err
		}
	}
	if v.ToolsRevision != agents.Revision {
		script, err := agents.Script(definition)
		if err != nil {
			return v, err
		}
		m.progress("Installing Claude, Codex and Cursor in %s", v.Name)
		if _, err := connection.Run(ctx, script, nil, m.Output, m.Output, false); err != nil {
			return v, fmt.Errorf("agent installation failed; VM preserved; retry namu vm start %s: %w", v.Name, err)
		}
		v.ToolsRevision = agents.Revision
		if err := m.save(&r, index, v); err != nil {
			return v, err
		}
	}
	if _, err := m.Tart.ExecRoot(ctx, v.Name, guest.MountScript(definition, v.Applied), m.Output); err != nil {
		return v, fmt.Errorf("guest directory mounting failed; VM preserved: %w", err)
	}
	m.progress("VM ready: %s (%s)", v.Name, v.OS)
	return v, nil
}
func (m *Manager) connection(ctx context.Context, v state.VM) (guest.SSH, error) {
	definition, err := registry.Resolve(v.OS)
	if err != nil {
		return guest.SSH{}, err
	}
	if err := guest.CheckSSH(ctx, m.Runner); err != nil {
		return guest.SSH{}, err
	}
	ip, err := m.Tart.IP(ctx, v.Name)
	if err != nil {
		return guest.SSH{}, err
	}
	return guest.SSH{Runner: m.Runner, Data: m.Store.Data, Identity: filepath.Join(m.Store.Data, "ssh", "id_ed25519"), User: definition.User, IP: ip, VM: v}, nil
}
func (m *Manager) Run(ctx context.Context, v state.VM, path string, argv []string, out io.Writer) error {
	connection, err := m.sessionConnection(ctx, v)
	if err != nil {
		return err
	}
	return connection.RunWorktree(ctx, path, argv, out)
}
func (m *Manager) Shell(ctx context.Context, v state.VM, path string) error {
	connection, err := m.sessionConnection(ctx, v)
	if err != nil {
		return err
	}
	return connection.Shell(ctx, path, m.Input, m.Output)
}
