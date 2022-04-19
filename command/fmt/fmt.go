package fmt

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"

	"rafal.dev/reflow/command"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand(app *command.App) *cobra.Command {
	m := &fmtCmd{App: app}

	cmd := &cobra.Command{
		Use:   "fmt",
		Short: "Formatting",
		Args:  cobra.ExactArgs(1),
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

func (m *fmtCmd) run(_ *cobra.Command, args []string) error {
	p, err := m.read(args)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	t, err := template.New("").Funcs(m.Funcs).Parse(string(p))
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	if err := t.Execute(os.Stdout, m.Template.JSON()); err != nil {
		return fmt.Errorf("template evaluation error: %w", err)
	}

	return nil
}

func (m *fmtCmd) read(args []string) ([]byte, error) {
	if len(args) == 0 || args[0] == "-" {
		return io.ReadAll(os.Stdin)
	}

	return ioutil.ReadFile(args[0])
}
