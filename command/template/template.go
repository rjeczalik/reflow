package template

import (
	"fmt"
	"io"
	"os"

	"rafal.dev/reflow/command"
	c "rafal.dev/reflow/pkg/context"
	"rafal.dev/reflow/pkg/template"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCommand(app *command.App) *cobra.Command {
	m := &templateCmd{
		App: app,
	}

	cmd := &cobra.Command{
		Use:   "template",
		Short: "Templating",
		Args:  cobra.NoArgs,
		RunE:  m.run,
	}

	m.register(cmd.Flags())

	return cmd
}

type templateCmd struct {
	*command.App
	exclude bool
}

func (m *templateCmd) register(f *pflag.FlagSet) {
	f.BoolVarP(&m.exclude, "exclude", "e", false, "List of keys to exclude")
}

func (m *templateCmd) run(_ *cobra.Command, args []string) error {
	obj, err := c.Build(m.Context())
	if err != nil {
		return fmt.Errorf("build error: %w", err)
	}

	if m.exclude {
		v, err := c.Get[[]any](obj, "exclude")
		if err != nil {
			return fmt.Errorf("excluding keys: %w", err)
		}

		for _, v := range v {
			c.Del(obj, fmt.Sprint(v))
		}
	}

	p, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	q, err := template.Execute(string(p), obj)
	if err != nil {
		return fmt.Errorf("execute template error: %w", err)
	}

	os.Stdout.Write(q)

	return nil
}
