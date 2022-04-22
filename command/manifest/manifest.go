package manifest

import (
	"os"

	"rafal.dev/reflow/command"
	"rafal.dev/reflow/pkg/manifest"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand(app *command.App) *cobra.Command {
	m := &manifestCmd{
		App:     app,
		Builder: manifest.DefaultBuilder,
	}

	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Creates manifest",
		Args:  cobra.NoArgs,
		RunE:  m.run,
	}

	m.register(cmd.Flags())

	return cmd
}

type manifestCmd struct {
	*command.App
	*manifest.Builder
}

func (*manifestCmd) register(*pflag.FlagSet) {}

func (m *manifestCmd) run(_ *cobra.Command, args []string) error {
	return m.Builder.Build(m.App.Context(), os.Stdin)
}
