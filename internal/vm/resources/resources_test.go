package resources

import "testing"

func TestOptionsOverrideOnlySpecifiedResources(t *testing.T) {
	cpus := 6
	base := Settings{CPUs: 2, MemoryMB: 4096, DiskGB: 100}
	actual := (Options{CPUs: &cpus}).Apply(base)
	if actual != (Settings{CPUs: 6, MemoryMB: 4096, DiskGB: 100}) {
		t.Fatal(actual)
	}
	if base.CPUs != 2 {
		t.Fatal("must not mutate base")
	}
}

func TestValidateResourceBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options Options
	}{
		{"zero-cpus", Options{CPUs: new(0)}},
		{"negative-memory", Options{MemoryMB: new(-1)}},
		{"zero-disk", Options{DiskGB: new(0)}},
		{"cpu-overflow", Options{CPUs: new(65536)}},
		{"disk-overflow", Options{DiskGB: new(65536)}},
		{"memory-overflow", Options{MemoryMB: new(1 << 43)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.options.Validate(); err == nil {
				t.Fatal("must reject invalid resources")
			}
		})
	}
}
