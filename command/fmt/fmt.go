package fmt

import (
	"rafal.dev/reflow/command"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand(app *command.App) *cobra.Command {
	m := &fmtCmd{App: app}

	cmd := &cobra.Command{
		Use:   "fmt",
		Short: "Formatting",
		Args:  cobra.NoArgs,
		RunE:  m.run,
	}

	cmd.AddCommand(
		NewMergeCommand(app),
	)

	m.register(cmd.Flags())

	return cmd
}

type fmtCmd struct {
	*command.App
}

func (*fmtCmd) register(*pflag.FlagSet) {
}

func (*fmtCmd) run(*cobra.Command, []string) error {
	return nil
}
