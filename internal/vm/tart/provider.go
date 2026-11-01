// Package tart is the only virtualization provider and owns its pinned downloads.
package tart

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kamaal111/namu/internal/vm/host"
)

//go:embed manifest.json
var manifest []byte

type Manifest struct {
	Images map[string]string                     `json:"images"`
	Tart   struct{ Version, URL, SHA256 string } `json:"tart"`
}

func Pins() (Manifest, error) { var p Manifest; err := json.Unmarshal(manifest, &p); return p, err }

type Provider struct {
	Runner host.Runner
	Data   string
	Host   func() (string, string)
}
type VM struct {
	Name, State string
	Running     bool
}

func (p Provider) Executable() string {
	pins, _ := Pins()
	path := filepath.Join(p.Data, "tools", "tart-"+pins.Tart.Version, "tart.app", "Contents", "MacOS", "tart")
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		return path
	}
	return "tart"
}
func (p Provider) HostCheck(ctx context.Context, osName string) error {
	system, arch := runtime.GOOS, runtime.GOARCH
	if p.Host != nil {
		system, arch = p.Host()
	}
	if system != "darwin" || arch != "arm64" {
		return fmt.Errorf("VM mode requires an Apple Silicon Mac; current host is %s/%s", system, arch)
	}
	b, err := p.Runner.Run(ctx, host.Command{Name: "sw_vers", Args: []string{"-productVersion"}})
	if err != nil {
		return err
	}
	major, err := strconv.Atoi(strings.Split(strings.TrimSpace(string(b)), ".")[0])
	if err != nil || major < 14 || (osName == "macos" && major < 27) {
		return fmt.Errorf("Linux VMs require macOS 14+; macOS 27 VMs require macOS 27+; use --os linux on an older supported host (host: %s)", strings.TrimSpace(string(b)))
	}
	return nil
}

// NeedsInstall returns false only for a compatible existing Tart executable.
func (p Provider) NeedsInstall(ctx context.Context) (bool, error) {
	b, err := p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"--version"}})
	if errors.Is(err, exec.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	pins, err := Pins()
	if err != nil {
		return false, err
	}
	if err := host.CheckVersion(string(b), pins.Tart.Version, 2); err != nil {
		if p.Executable() == "tart" {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
func (p Provider) Install(ctx context.Context, out io.Writer) error {
	pins, err := Pins()
	if err != nil {
		return err
	}
	parent := filepath.Join(p.Data, "tools")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, "tart.tar.gz")
	_, err = p.Runner.Run(ctx, host.Command{Name: "curl", Args: []string{"--disable", "--fail", "--location", "--retry", "3", "--connect-timeout", "20", "--proto", "=https", "--proto-redir", "=https", pins.Tart.URL, "-o", archive}, Out: out, Err: out})
	if err != nil {
		return fmt.Errorf("download private Tart: %w", err)
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	sum := sha256.New()
	_, err = io.Copy(sum, f)
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if hex.EncodeToString(sum.Sum(nil)) != pins.Tart.SHA256 {
		return fmt.Errorf("private Tart checksum mismatch; refusing installation")
	}
	dest := filepath.Join(tmp, "extracted")
	if err := os.Mkdir(dest, 0700); err != nil {
		return err
	}
	if _, err = p.Runner.Run(ctx, host.Command{Name: "tar", Args: []string{"-xzf", archive, "-C", dest}, Err: out}); err != nil {
		return err
	}
	return os.Rename(dest, filepath.Join(parent, "tart-"+pins.Tart.Version))
}
func (p Provider) List(ctx context.Context) (map[string]VM, error) {
	b, err := p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"list", "--source", "local", "--format", "json"}})
	if err != nil {
		return nil, err
	}
	var list []VM
	if err = json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("decode Tart VMs: %w", err)
	}
	result := map[string]VM{}
	for _, v := range list {
		if v.Name == "" {
			return nil, fmt.Errorf("Tart VM has no name")
		}
		result[v.Name] = v
	}
	return result, nil
}
func (p Provider) Clone(ctx context.Context, image, name, osName string, out io.Writer) error {
	if _, err := p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"clone", image, name}, Env: append(os.Environ(), "TART_NO_AUTO_PRUNE=1"), Out: out, Err: out}); err != nil {
		return err
	}
	return p.Configure(ctx, name, osName)
}
func (p Provider) Configure(ctx context.Context, name, osName string) error {
	disk := "80"
	if osName == "linux" {
		disk = "60"
	}
	_, err := p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"set", name, "--cpu", "4", "--memory", "8192", "--disk-size", disk}})
	return err
}
func Tag(root string) string { return fmt.Sprintf("namu-%x", sha256.Sum256([]byte(root)))[:29] }
func (p Provider) Start(ctx context.Context, name string, shares []string) error {
	args := []string{"run", "--no-graphics", "--no-clipboard", "--no-audio"}
	for _, root := range shares {
		args = append(args, "--dir", root+":tag="+Tag(root))
	}
	args = append(args, name)
	return p.Runner.Detach(ctx, host.Command{Name: p.Executable(), Args: args}, filepath.Join(p.Data, "tart-"+name+".log"))
}
func (p Provider) Stop(ctx context.Context, name string) error {
	_, err := p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"stop", name}})
	return err
}
func (p Provider) Delete(ctx context.Context, name string) error {
	_, err := p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"delete", name}})
	return err
}
func (p Provider) IP(ctx context.Context, name string) (string, error) {
	b, err := p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"ip", name, "--wait", "180"}})
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(b))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("Tart returned invalid IP address")
	}
	return ip, nil
}
func (p Provider) ExecRoot(ctx context.Context, name, script string, out io.Writer) ([]byte, error) {
	return p.Runner.Run(ctx, host.Command{Name: p.Executable(), Args: []string{"exec", "-i", name, "sudo", "-n", "/bin/sh", "-s"}, In: strings.NewReader(script), Err: out})
}
func (p Provider) Ready(ctx context.Context, name string) error {
	var last error
	var diagnostics strings.Builder
	for {
		diagnostics.Reset()
		if _, last = p.ExecRoot(ctx, name, "true\n", &diagnostics); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("guest control not ready; VM %s preserved: %w; last attempt: %v; %s", name, ctx.Err(), last, strings.TrimSpace(diagnostics.String()))
		case <-time.After(time.Second):
		}
	}
}
