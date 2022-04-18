package reflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"rafal.dev/reflow/command"

	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

func NewCallCommand(app *command.App) *cobra.Command {
	m := &callCmd{
		App:   app,
		token: os.Getenv("GITHUB_TOKEN"),
	}

	cmd := &cobra.Command{
		Use:   "call",
		Short: "Call manual workflow",
		Args:  cobra.NoArgs,
		RunE:  m.run,
	}

	m.register(cmd.Flags())

	command.Use(cmd, m.pre)

	return cmd
}

type callCmd struct {
	*command.App

	interval time.Duration
	timeout  time.Duration
	perpage  int
	token    string
	typ      string
	branch   string
	input    string
	workflow string
	follow   bool
	inputs   map[string]any
}

func (m *callCmd) register(f *pflag.FlagSet) {
	f.StringVarP(&m.typ, "type", "t", "yaml", "Encoding type of the inputs")
	f.StringVarP(&m.input, "input", "n", "-", "Inputs")
	f.StringVarP(&m.branch, "branch", "b", "heads/master", "Workflow tree reference")
	f.StringVarP(&m.workflow, "workflow", "w", "", "Path of the workflow to run")
	f.IntVarP(&m.perpage, "pages", "p", 10, "Per page limit while listing workflows")
	f.DurationVarP(&m.interval, "interval", "i", 30*time.Second, "Poll interval to check on dispatched workflow")
	f.DurationVarP(&m.timeout, "max-lookup", "x", 3*time.Minute, "Max time for looking up a workflow run")
	f.BoolVarP(&m.follow, "follow", "f", true, "Follow the dispatched workflow run")
}

func (m *callCmd) read(file string) ([]byte, error) {
	switch file {
	case "-":
		return io.ReadAll(os.Stdin)
	default:
		return ioutil.ReadFile(file)
	}
}

func (m *callCmd) unmarshal(p []byte, v interface{}) error {
	switch strings.ToLower(m.typ) {
	case "json":
		if err := json.Unmarshal(p, &m.inputs); err != nil {
			return fmt.Errorf("json unmarshal: %w", err)
		}
	case "yaml":
		if err := yaml.Unmarshal(p, &m.inputs); err != nil {
			return fmt.Errorf("yaml unmarshal: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %q", m.typ)
	}

	return nil
}

func (m *callCmd) pre(next command.CobraFunc) command.CobraFunc {
	p, err := m.read(m.input)
	if err != nil {
		return command.Errorf("read error: %w", err)
	}

	if err := m.unmarshal(p, &m.inputs); err != nil {
		return command.Errorf("unmarshal error: %w", err)
	}

	data := m.Template.JSON()

	for k, v := range m.inputs {
		var s string
		switch v := v.(type) {
		case nil:
			s = ""
		case int:
			s = strconv.Itoa(v)
		case string:
			s = v
		default:
			s = fmt.Sprint(v)
		}

		if data != nil {
			if t, err := template.New(k).Parse(s); err == nil {
				var buf bytes.Buffer

				if err := t.Execute(&buf, data); err == nil {
					s = buf.String()
				}
			}
		}

		m.inputs[k] = s
	}

	return next
}

func (m *callCmd) run(*cobra.Command, []string) error {
	anchor := "reflow/" + uuid.New().String()

	ref, _, err := m.GitHub.Git.GetRef(m.Context(), m.Owner, m.Repo, m.branch)
	if err != nil {
		return fmt.Errorf("get ref error: %w", err)
	}

	branch := &github.Reference{
		Ref:    github.String("refs/heads/" + anchor),
		Object: ref.Object,
	}

	ref, _, err = m.GitHub.Git.CreateRef(m.Context(), m.Owner, m.Repo, branch)
	if err != nil {
		return fmt.Errorf("create ref error: %w", err)
	}
	defer m.GitHub.Git.DeleteRef(m.Context(), m.Owner, m.Repo, *ref.Ref)

	req := github.CreateWorkflowDispatchEventRequest{
		Ref:    anchor,
		Inputs: m.inputs,
	}

	_, err = m.GitHub.Actions.CreateWorkflowDispatchEventByFileName(m.Context(), m.Owner, m.Repo, m.workflow, req)
	if err != nil {
		return fmt.Errorf("dispatch workflow run error: %w", err)
	}

	fmt.Fprintf(os.Stderr, "🛠  Workflow %q dispatched successfully: anchor %q\n", m.workflow, anchor)

	opts := &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{
			Page:    1,
			PerPage: m.perpage,
		},
	}

	var (
		workflow *github.WorkflowRun
		tick     = time.NewTicker(m.interval)
		timeout  = time.NewTimer(m.timeout)
	)

	defer tick.Stop()
	defer timeout.Stop()

	time.Sleep(10 * time.Second) // warmup

	for {
		w, _, err := m.GitHub.Actions.ListWorkflowRunsByFileName(m.Context(), m.Owner, m.Repo, m.workflow, opts)
		if err != nil {
			return fmt.Errorf("list workflow runs error: %w", err)
		}

		for _, w := range w.WorkflowRuns {
			if *w.HeadBranch == anchor {
				workflow = w
				break
			}
		}

		if workflow != nil {
			break
		}

		select {
		case <-m.Context().Done():
			return m.Context().Err()
		case <-timeout.C:
			return fmt.Errorf("looking for workflow run has timed out after %s", m.timeout)
		case <-tick.C:
			// continue
		}
	}

	fmt.Fprintf(os.Stderr, "🛠  The dispatched workflow is runnng at %s\n", *workflow.HTMLURL)

	if !m.follow {
		return nil
	}

poll:
	for {
		select {
		case <-tick.C:
			workflow, _, err = m.GitHub.Actions.GetWorkflowRunByID(m.Context(), m.Owner, m.Repo, *workflow.ID)
			if err != nil {
				return fmt.Errorf("get workflow run error: %w", err)
			}

			fmt.Fprintf(os.Stderr, "🛠  Workflow status: %q [%s]\n", *workflow.Status, *workflow.HTMLURL)

			if c := workflow.Conclusion; c != nil && *c != "" {
				break poll
			}
		case <-m.Context().Done():
			return m.Context().Err()
		}
	}

	if got, want := *workflow.Conclusion, "success"; got != want {
		return fmt.Errorf("undesired workflow status: got %q, want %q [%s]", got, want, *workflow.Status)
	}

	return nil
}

func (m *callCmd) render(v interface{}) error {
	switch strings.ToLower(m.typ) {
	case "yaml":
		p, err := yaml.Marshal(v)
		if err != nil {
			return fmt.Errorf("yaml marshal: %w", err)
		}

		fmt.Printf("%s\n", p)
	case "json":
		p, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("json marshal: %w", err)
		}

		fmt.Printf("%s\n", p)
	default:
		return fmt.Errorf("unsupported format: %q", m.typ)
	}

	return nil
}
