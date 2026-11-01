// Package resources owns VM resource defaults, overrides, and validation.
package resources

import "fmt"

type Settings struct {
	CPUs     int `json:"cpus"`
	MemoryMB int `json:"memory_mb"`
	DiskGB   int `json:"disk_gb"`
}

// Options distinguishes an omitted setting from an invalid explicit zero.
type Options struct {
	CPUs     *int `toml:"cpus"`
	MemoryMB *int `toml:"memory_mb"`
	DiskGB   *int `toml:"disk_gb"`
}

func (o Options) Validate() error {
	for _, field := range []struct {
		name  string
		value *int
		max   int
	}{
		{"cpus", o.CPUs, 65535},
		{"memory_mb", o.MemoryMB, (1 << 43) - 1},
		{"disk_gb", o.DiskGB, 65535},
	} {
		if field.value != nil && (*field.value < 1 || *field.value > field.max) {
			return fmt.Errorf("vm.%s must be between 1 and %d", field.name, field.max)
		}
	}
	return nil
}

func (s Settings) Validate() error {
	return (Options{CPUs: &s.CPUs, MemoryMB: &s.MemoryMB, DiskGB: &s.DiskGB}).Validate()
}

func (o Options) Apply(base Settings) Settings {
	if o.CPUs != nil {
		base.CPUs = *o.CPUs
	}
	if o.MemoryMB != nil {
		base.MemoryMB = *o.MemoryMB
	}
	if o.DiskGB != nil {
		base.DiskGB = *o.DiskGB
	}
	return base
}

func (s Settings) String() string {
	return fmt.Sprintf("%d CPUs, %d MiB RAM, %d GB disk", s.CPUs, s.MemoryMB, s.DiskGB)
}
