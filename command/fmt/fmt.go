package fmt

import (
	"rafal.dev/reflow/command"
	f "rafal.dev/reflow/pkg/fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand(app *command.App) *cobra.Command {
	m := &fmtCmd{
		App:      app,
		Formater: f.DefaultFormater,
	}

	cmd := &cobra.Command{
		Use:   "fmt",
		Short: "Formatting",
		Args:  cobra.ExactArgs(2),
		RunE:  m.run,
	}

	m.register(cmd.Flags())

	return cmd
}

type fmtCmd struct {
	*command.App
	*f.Formater
	mask bool
}

func (m *fmtCmd) register(f *pflag.FlagSet) {
	f.BoolVarP(&m.mask, "mask", "m", false, "List of keys to mask")
}

func (m *fmtCmd) run(_ *cobra.Command, args []string) error {
	return m.Formater.Format(m.Context(), args[0], args[1], m.mask)
}
