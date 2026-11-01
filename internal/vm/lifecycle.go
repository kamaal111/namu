package vm

import (
	"context"
	"fmt"
	"strings"

	"github.com/kamaal111/namu/internal/vm/state"
)

type Status struct{ Name, OS, State string }

func (m *Manager) List(ctx context.Context) ([]Status, error) {
	r, err := m.Store.Read()
	if err != nil {
		return nil, err
	}
	if len(r.VMs) == 0 {
		return []Status{}, nil
	}
	if err := m.Tart.HostCheck(ctx, "linux"); err != nil {
		return nil, err
	}
	live, err := m.Tart.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Status, 0, len(r.VMs))
	for _, v := range r.VMs {
		status := "missing"
		if t, ok := live[v.Name]; ok {
			status = "stopped"
			if t.Running {
				status = "running"
			}
			if v.Stage != "ready" {
				status += " (" + v.Stage + ")"
			}
		}
		result = append(result, Status{v.Name, v.OS, status})
	}
	return result, nil
}
func (m *Manager) Create(ctx context.Context, name, osName string) (state.VM, error) {
	if err := state.ValidateName(name); err != nil {
		return state.VM{}, err
	}
	if osName == "" {
		osName = "macos"
	}
	if err := ValidateOS(osName); err != nil {
		return state.VM{}, err
	}
	return m.ensure(ctx, name, osName, nil, true, true)
}
func (m *Manager) Start(ctx context.Context, name string) (state.VM, error) {
	return m.Ensure(ctx, name, "", nil, false)
}
func (m *Manager) Stop(ctx context.Context, name string) error {
	return m.modify(ctx, name, func(r state.Registry, index int) error {
		live, err := m.Tart.List(ctx)
		if err != nil {
			return err
		}
		if current, ok := live[name]; ok && current.Running {
			if err := m.Tart.Stop(ctx, name); err != nil {
				return err
			}
		}
		v := r.VMs[index]
		v.Applied = nil
		return m.save(&r, index, v)
	})
}
func (m *Manager) Delete(ctx context.Context, name string) error {
	return m.modify(ctx, name, func(r state.Registry, index int) error {
		v := r.VMs[index]
		live, err := m.Tart.List(ctx)
		if err != nil {
			return err
		}
		current, exists := live[name]
		if exists && current.Running {
			return fmt.Errorf("stop VM %s before deletion: namu vm stop %s", name, name)
		}
		summary := fmt.Sprintf("Permanently delete VM %s, its guest files and installed tools? Host repositories are retained.\nRegistered shares:\n%s", name, strings.Join(v.Shares, "\n"))
		if err := m.approve(ctx, summary); err != nil {
			return err
		}
		if exists {
			if err := m.Tart.Delete(ctx, name); err != nil {
				return err
			}
		}
		r.VMs = append(r.VMs[:index:index], r.VMs[index+1:]...)
		return m.Store.Write(r)
	})
}
func (m *Manager) modify(ctx context.Context, name string, fn func(state.Registry, int) error) error {
	if err := state.ValidateName(name); err != nil {
		return err
	}
	unlock, err := m.Store.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	r, err := m.Store.Read()
	if err != nil {
		return err
	}
	index, err := state.Find(r, name)
	if err != nil {
		return err
	}
	if err := m.Tart.HostCheck(ctx, "linux"); err != nil {
		return err
	}
	return fn(r, index)
}
