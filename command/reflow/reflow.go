package reflow

import (
	"rafal.dev/reflow/command"
	"rafal.dev/reflow/command/fmt"
	"rafal.dev/reflow/command/manifest"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand(app *command.App) *cobra.Command {
	m := &reflowCmd{App: app}

	cmd := &cobra.Command{
		Use:   "reflow",
		Short: "Formatting",
		Args:  cobra.NoArgs,
		RunE:  m.run,
	}

	cmd.AddCommand(
		fmt.NewCommand(app),
		manifest.NewCommand(app),
		NewRunCommand(app),
	)

	m.register(cmd.Flags())

	return cmd
}

type reflowCmd struct {
	*command.App
}

func (m *reflowCmd) register(f *pflag.FlagSet) {
}

func (*reflowCmd) run(*cobra.Command, []string) error {
	return nil
}
