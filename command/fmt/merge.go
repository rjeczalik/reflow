package fmt

import (
	"rafal.dev/reflow/command"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewMergeCommand(app *command.App) *cobra.Command {
	m := &mergeCmd{App: app}

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge documents",
		Args:  cobra.NoArgs,
		RunE:  m.run,
	}

	m.register(cmd.Flags())

	return cmd
}

type mergeCmd struct {
	*command.App
}

func (*mergeCmd) register(*pflag.FlagSet) {
}

func (*mergeCmd) run(*cobra.Command, []string) error {
	return nil
}
