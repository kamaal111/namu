package cli

import (
	"strconv"

	"github.com/spf13/cobra"
)

// registerBoolPair shares one value so each parsed flag overrides the previous one.
func registerBoolPair(cmd *cobra.Command, name string, enabled bool, help, negativeHelp string) {
	for _, flag := range []struct {
		name, help string
		negated    bool
	}{
		{name, help, false},
		{"no-" + name, negativeHelp, true},
	} {
		cmd.Flags().Var(&pairedBool{value: &enabled, negated: flag.negated}, flag.name, flag.help)
		cmd.Flags().Lookup(flag.name).NoOptDefVal = "true"
	}
}

type pairedBool struct {
	value   *bool
	negated bool
}

func (v *pairedBool) Set(raw string) error {
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return err
	}
	*v.value = parsed != v.negated
	return nil
}

func (v *pairedBool) String() string { return strconv.FormatBool(*v.value != v.negated) }
func (v *pairedBool) Type() string   { return "bool" }
