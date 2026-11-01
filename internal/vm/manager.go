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

	"github.com/kamaal111/namu/internal/vm/guest"
	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/state"
	"github.com/kamaal111/namu/internal/vm/tart"
)

type Confirm func(context.Context, string) error

// Manager's dependencies are scoped to one invocation; a shell never holds its lifecycle lock.
type Manager struct {
	Store   state.Store
	Tart    tart.Provider
	Runner  host.Runner
	Confirm Confirm
	Input   io.Reader
	Output  io.Writer
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
	if osName != "macos" && osName != "linux" {
		return fmt.Errorf("unsupported guest OS %q; use macos or linux", osName)
	}
	return nil
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
			osName = "macos"
		}
		pins, err := tart.Pins()
		if err != nil {
			return v, err
		}
		v = state.VM{ID: rand.Text(), Name: name, OS: osName, Image: pins.Images[osName], Stage: "creating"}
	} else {
		v = r.VMs[index]
		if osName != "" && osName != v.OS {
			return v, fmt.Errorf("VM %s runs %s; incompatible with --os %s", name, v.OS, osName)
		}
	}
	if err := m.Tart.HostCheck(ctx, v.OS); err != nil {
		return v, err
	}
	install, err := m.Tart.NeedsInstall(ctx)
	if err != nil {
		return v, err
	}
	if creating || v.Stage != "ready" || install {
		disk := 80
		if v.OS == "linux" {
			disk = 60
		}
		summary := fmt.Sprintf("Prepare persistent VM %s (%s): 4 CPUs, 8192 MiB RAM, %d GB disk.\nImage: %s\nConfigure dedicated SSH access, Git and directory mounting. Downloads may be several GB.", v.Name, v.OS, disk, v.Image)
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
			if err := m.Tart.Configure(ctx, v.Name, v.OS); err != nil {
				return v, err
			}
		} else {
			m.progress("Downloading image and creating VM %s", v.Name)
			if err := m.Tart.Clone(ctx, v.Image, v.Name, v.OS, m.Output); err != nil {
				return v, fmt.Errorf("creation incomplete; VM %s preserved; retry with namu vm start %s: %w", v.Name, v.Name, err)
			}
		}
		v.Stage = "cloned"
		if err := m.save(&r, index, v); err != nil {
			return v, err
		}
	}
	desired, err := ShareRoots(append(append([]string(nil), v.Shares...), roots...))
	if err != nil {
		return v, err
	}
	if current.Running && !slices.Equal(desired, v.Applied) {
		if err := m.approve(ctx, fmt.Sprintf("Restart VM %s to share these repository directories read/write:\n%s\nAll processes in this guest will stop.", v.Name, strings.Join(desired, "\n"))); err != nil {
			return v, err
		}
		if err := m.Tart.Stop(ctx, v.Name); err != nil {
			return v, err
		}
		current.Running = false
	}
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
		key, err := m.Tart.ExecRoot(ctx, v.Name, guest.BootstrapScript(v.OS, publicKey), m.Output)
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
	if _, err := m.Tart.ExecRoot(ctx, v.Name, guest.AccessScript(v.OS, publicKey), m.Output); err != nil {
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
	if _, err := m.Tart.ExecRoot(ctx, v.Name, guest.MountScript(v.OS, v.Applied), m.Output); err != nil {
		return v, fmt.Errorf("guest directory mounting failed; VM preserved: %w", err)
	}
	m.progress("VM ready: %s (%s)", v.Name, v.OS)
	return v, nil
}
func (m *Manager) connection(ctx context.Context, v state.VM) (guest.SSH, error) {
	if err := guest.CheckSSH(ctx, m.Runner); err != nil {
		return guest.SSH{}, err
	}
	ip, err := m.Tart.IP(ctx, v.Name)
	if err != nil {
		return guest.SSH{}, err
	}
	return guest.SSH{Runner: m.Runner, Data: m.Store.Data, Identity: filepath.Join(m.Store.Data, "ssh", "id_ed25519"), User: "admin", IP: ip, VM: v}, nil
}
func (m *Manager) Run(ctx context.Context, v state.VM, path string, argv []string, out io.Writer) error {
	c, err := m.connection(ctx, v)
	if err != nil {
		return err
	}
	return c.RunWorktree(ctx, path, argv, out)
}

func (m *Manager) Shell(ctx context.Context, v state.VM, path string) error {
	c, err := m.connection(ctx, v)
	if err != nil {
		return err
	}
	return c.Shell(ctx, path, m.Input, m.Output)
}
