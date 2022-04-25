package reflow

import (
	"rafal.dev/reflow/command"
	"rafal.dev/reflow/pkg/reflow"

	"github.com/spf13/cobra"
)

func NewRunCommand(app *command.App) *cobra.Command {
	m := &runCmd{
		App:    app,
		Client: reflow.New(),
	}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run manual workflow",
		Args:  cobra.ExactArgs(1),
		RunE:  m.run,
	}

	m.register(cmd)

	return cmd
}

type runCmd struct {
	*command.App
	*reflow.Client
}

func (m *runCmd) register(cmd *cobra.Command) {
	f := cmd.Flags()

	f.IntVarP(&m.Client.PerPage, "pages", "p", m.Client.PerPage, "Per page limit while listing workflows")
	f.DurationVarP(&m.Client.Interval, "interval", "y", m.Client.Interval, "Poll interval to check on dispatched workflow")
	f.DurationVarP(&m.MaxLookup, "max-lookup", "x", m.Client.MaxLookup, "Max time for looking up a workflow run")
}

func (m *runCmd) run(_ *cobra.Command, args []string) error {
	outputs, err := m.Client.Run(m.App.Context(), args[0])
	if err != nil {
		return err
	}

	_ = outputs

	return nil
}
