package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

type cmd struct {
	start    time.Time
	interval time.Duration
	timeout  time.Duration
	perpage  int
	token    string
	typ      string
	branch   string
	owner    string
	repo     string
	workflow string
	follow   bool
	inputs   map[string]any
	client   *github.Client
}

func (m *cmd) register(f *flag.FlagSet) {
	f.StringVar(&m.typ, "t", "yaml", "Encoding type of the inputs")
	f.StringVar(&m.owner, "o", "", "Repository owner")
	f.StringVar(&m.repo, "r", "", "Repository name")
	f.StringVar(&m.branch, "b", "heads/master", "Workflow tree reference")
	f.StringVar(&m.workflow, "w", "", "Path of the workflow to run")
	f.IntVar(&m.perpage, "p", 10, "Per page limit while listing workflows")
	f.DurationVar(&m.interval, "i", 30*time.Second, "Poll interval to check on dispatched workflow")
	f.DurationVar(&m.timeout, "x", 3*time.Minute, "Max time for looking up a workflow run")
	f.BoolVar(&m.follow, "f", true, "Follow the dispatched workflow run")
}

func die(format string, v ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", v...)
	os.Exit(1)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	m := &cmd{
		token: os.Getenv("GITHUB_TOKEN"),
		start: time.Now(),
	}

	m.register(flag.CommandLine)

	flag.Parse()

	p, err := io.ReadAll(os.Stdin)
	if err != nil {
		die("read error: %+v", err)
	}

	if err := m.init(ctx, p); err != nil {
		die("init failed: %+v", err)
	}

	if err := m.run(ctx); err != nil {
		die("run error: %+v", err)
	}
}

func (m *cmd) init(ctx context.Context, p []byte) error {
	m.client = github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: m.token,
		},
	)))

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

	for k, v := range m.inputs {
		switch v := v.(type) {
		case nil:
			m.inputs[k] = ""
		case int:
			m.inputs[k] = strconv.Itoa(v)
		case string:
			m.inputs[k] = v
		default:
			m.inputs[k] = fmt.Sprint(v)
		}
	}

	return nil
}

func (m *cmd) run(ctx context.Context) error {
	anchor := "reflow/" + uuid.New().String()

	ref, _, err := m.client.Git.GetRef(ctx, m.owner, m.repo, m.branch)
	if err != nil {
		return fmt.Errorf("get ref error: %w", err)
	}

	branch := &github.Reference{
		Ref:    github.String("refs/heads/" + anchor),
		Object: ref.Object,
	}

	ref, _, err = m.client.Git.CreateRef(ctx, m.owner, m.repo, branch)
	if err != nil {
		return fmt.Errorf("create ref error: %w", err)
	}
	defer m.client.Git.DeleteRef(ctx, m.owner, m.repo, *ref.Ref)

	req := github.CreateWorkflowDispatchEventRequest{
		Ref:    anchor,
		Inputs: m.inputs,
	}

	_, err = m.client.Actions.CreateWorkflowDispatchEventByFileName(ctx, m.owner, m.repo, m.workflow, req)
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
		w, _, err := m.client.Actions.ListWorkflowRunsByFileName(ctx, m.owner, m.repo, m.workflow, opts)
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
		case <-ctx.Done():
			return ctx.Err()
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
			workflow, _, err = m.client.Actions.GetWorkflowRunByID(ctx, m.owner, m.repo, *workflow.ID)
			if err != nil {
				return fmt.Errorf("get workflow run error: %w", err)
			}

			fmt.Fprintf(os.Stderr, "🛠  Workflow status: %q [%s]\n", *workflow.Status, *workflow.HTMLURL)

			if c := workflow.Conclusion; c != nil && *c != "" {
				break poll
			}
		case <-ctx.Done():
			return ctx.Err()

		}
	}

	if got, want := *workflow.Conclusion, "success"; got != want {
		return fmt.Errorf("undesired workflow status: got %q, want %q [%s]", got, want, *workflow.Status)
	}

	return nil
}

func (m *cmd) render(v interface{}) error {
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
