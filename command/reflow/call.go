package reflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rafal.dev/reflow/command"

	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewCallCommand(app *command.App) *cobra.Command {
	m := &callCmd{
		App: app,
	}

	cmd := &cobra.Command{
		Use:   "call",
		Short: "Call manual workflow",
		Args:  cobra.NoArgs,
		RunE:  m.run,
	}

	m.register(cmd)

	command.Use(cmd, m.pre)

	return cmd
}

type workflow struct {
	Owner  string
	Repo   string
	File   string
	Branch string
}

var reUses = regexp.MustCompile(`(?P<owner>[^/]+)/(?P<repo>[^/]+)/.github/workflows/(?P<file>[^@]+)@(?P<branch>[^$]+)`)

func parseWorkflow(s string) (*workflow, error) {
	var (
		x = reUses.FindStringSubmatch(s)
		v = make(map[string]string)
	)

	for i, group := range reUses.SubexpNames()[0:] {
		if i != 0 && group != "" {
			v[group] = x[i]
		}
	}

	p, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	var w workflow

	if err := json.Unmarshal(p, &w); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &w, nil
}

func (w *workflow) String() string {
	return w.Owner + "/" + w.Repo + "/.github/workflows/" + w.File + "@" + w.Branch
}

type callCmd struct {
	*command.App

	interval time.Duration
	timeout  time.Duration

	uses    string
	input   string
	perpage int

	inputs map[string]any
	work   *workflow
}

func (m *callCmd) register(cmd *cobra.Command) {
	f := cmd.Flags()

	f.StringVarP(&m.input, "inputs", "i", "-", "Inputs")
	f.StringVarP(&m.uses, "uses", "u", "", "Inputs")
	f.IntVarP(&m.perpage, "pages", "p", 10, "Per page limit while listing workflows")
	f.DurationVarP(&m.interval, "interval", "y", 30*time.Second, "Poll interval to check on dispatched workflow")
	f.DurationVarP(&m.timeout, "max-lookup", "x", 3*time.Minute, "Max time for looking up a workflow run")

	cmd.MarkFlagRequired("uses")
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
	switch strings.ToLower(m.Format) {
	case "json":
		if err := json.Unmarshal(p, &m.inputs); err != nil {
			return fmt.Errorf("json unmarshal: %w", err)
		}
	case "yaml":
		if err := yaml.Unmarshal(p, &m.inputs); err != nil {
			return fmt.Errorf("yaml unmarshal: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %q", m.Format)
	}

	return nil
}

func (m *callCmd) pre(next command.CobraFunc) command.CobraFunc {
	return func(cmd *cobra.Command, args []string) error {
		if !cmd.Flag("inputs").Changed {
			if s := os.Getenv("REFLOW_INPUTS"); s != "" {
				m.input = s
			}
		}

		p, err := m.read(m.input)
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		if err := m.unmarshal(p, &m.inputs); err != nil {
			return fmt.Errorf("unmarshal error: %w", err)
		}

		if m.work, err = parseWorkflow(m.uses); err != nil {
			return fmt.Errorf("error parsing workflow: %w", err)
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
				if t, err := template.New(k).Funcs(m.Funcs).Parse(s); err == nil {
					var buf bytes.Buffer

					if err := t.Execute(&buf, data); err == nil {
						s = buf.String()
					}
				}
			}

			m.inputs[k] = s
		}

		return next(cmd, args)
	}
}

func (m *callCmd) run(*cobra.Command, []string) error {
	anchor := "reflow/" + uuid.New().String()

	ref, _, err := m.GitHub.Git.GetRef(m.Context(), m.work.Owner, m.work.Repo, m.work.Branch)
	if err != nil {
		return fmt.Errorf("get ref error: %w", err)
	}

	branch := &github.Reference{
		Ref:    github.String("refs/heads/" + anchor),
		Object: ref.Object,
	}

	ref, _, err = m.GitHub.Git.CreateRef(m.Context(), m.work.Owner, m.work.Repo, branch)
	if err != nil {
		return fmt.Errorf("create ref error: %w", err)
	}
	defer m.GitHub.Git.DeleteRef(m.Context(), m.work.Owner, m.work.Repo, *ref.Ref)

	req := github.CreateWorkflowDispatchEventRequest{
		Ref:    anchor,
		Inputs: m.inputs,
	}

	_, err = m.GitHub.Actions.CreateWorkflowDispatchEventByFileName(m.Context(), m.work.Owner, m.work.Repo, m.work.File, req)
	if err != nil {
		return fmt.Errorf("dispatch workflow run error: %w", err)
	}

	fmt.Fprintf(os.Stderr, "🛠  Workflow %q dispatched successfully: anchor %q\n", m.work.File, anchor)

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
		w, _, err := m.GitHub.Actions.ListWorkflowRunsByFileName(m.Context(), m.work.Owner, m.work.Repo, m.work.File, opts)
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

poll:
	for {
		select {
		case <-tick.C:
			workflow, _, err = m.GitHub.Actions.GetWorkflowRunByID(m.Context(), m.work.Owner, m.work.Repo, *workflow.ID)
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
	switch strings.ToLower(m.Format) {
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
		return fmt.Errorf("unsupported format: %q", m.Format)
	}

	return nil
}
