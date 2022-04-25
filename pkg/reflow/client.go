package reflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"rafal.dev/reflow/internal/misc"
	c "rafal.dev/reflow/pkg/context"
	f "rafal.dev/reflow/pkg/fmt"
	"rafal.dev/reflow/pkg/template"

	"github.com/google/go-github/v43/github"
)

type Client struct {
	GitHub *github.Client
	Fmt    *f.Formater

	Home      string
	PerPage   int
	Interval  time.Duration
	MaxLookup time.Duration
	token     string
}

func New() *Client {
	return &Client{
		GitHub:    misc.GitHub(context.Background()),
		Fmt:       f.DefaultFormater,
		Home:      misc.Home(),
		PerPage:   10,
		Interval:  30 * time.Second,
		MaxLookup: 3 * time.Minute,
		token:     misc.GitHubToken(),
	}
}

func (cl *Client) Run(ctx context.Context, runID string) (outputs map[string]any, err error) {
	var (
		runHome      = filepath.Join(cl.Home, "runs", runID)
		runContext   = filepath.Join(runHome, "context")
		runTemplates = filepath.Join(runHome, "templates")
		runInputs    = filepath.Join(runHome, "inputs", "inputs.yaml")

		home          = cl.Home
		homeContext   = filepath.Join(home, "context")
		homeTemplates = filepath.Join(home, "templates")
		excludes      = []string{
			"manifest",
			"github",
			"values",
			"reflow",
		}
	)

	b := c.SeqBuilder{
		&c.DirBuilder{Dir: os.DirFS(runContext)},
		&c.ReflowBuilder{Client: cl.GitHub},
		&c.DirBuilder{Dir: os.DirFS(runTemplates), Conv: c.Template},
		&c.DirBuilder{Dir: os.DirFS(homeContext), Exclude: excludes},
		&c.DirBuilder{Dir: os.DirFS(homeTemplates), Conv: c.Template, Exclude: excludes},
	}

	m := make(map[string]any)

	if err := b.Build(ctx, m); err != nil {
		return nil, fmt.Errorf("building context: %w", err)
	}

	uses, err := c.Get[string](m, "manifest.uses")
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	wrk, err := parseWorkflow(uses)
	if err != nil {
		return nil, fmt.Errorf("parse workflow: %w", err)
	}

	inputs := make(map[string]any)

	if err := cl.Fmt.Unmarshal(runInputs, &inputs); err != nil {
		return nil, fmt.Errorf("unmarshal inputs: %w", err)
	}

	c.Set(m, "reflow.token", cl.token)

	if err := cl.templateInputs(ctx, inputs, m); err != nil {
		return nil, fmt.Errorf("template inputs: %w", err)
	}

	anchor := "reflow/" + runID

	ref, _, err := cl.GitHub.Git.GetRef(ctx, wrk.Owner, wrk.Repo, wrk.Branch)
	if err != nil {
		return nil, fmt.Errorf("get ref error: %w", err)
	}

	branch := &github.Reference{
		Ref:    github.String("refs/heads/" + anchor),
		Object: ref.Object,
	}

	ref, _, err = cl.GitHub.Git.CreateRef(ctx, wrk.Owner, wrk.Repo, branch)
	if err != nil {
		return nil, fmt.Errorf("create ref error: %w", err)
	}
	defer cl.GitHub.Git.DeleteRef(ctx, wrk.Owner, wrk.Repo, *ref.Ref)

	req := github.CreateWorkflowDispatchEventRequest{
		Ref:    anchor,
		Inputs: inputs,
	}

	_, err = cl.GitHub.Actions.CreateWorkflowDispatchEventByFileName(ctx, wrk.Owner, wrk.Repo, wrk.File, req)
	if err != nil {
		return nil, fmt.Errorf("dispatch workflow run error: %w", err)
	}

	fmt.Fprintf(os.Stderr, "🛠  Workflow %q dispatched successfully: anchor %q\n", wrk.File, anchor)

	opts := &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{
			Page:    1,
			PerPage: cl.PerPage,
		},
	}

	var (
		workflow *github.WorkflowRun
		tick     = time.NewTicker(cl.Interval)
		timeout  = time.NewTimer(cl.MaxLookup)
	)

	defer tick.Stop()
	defer timeout.Stop()

	time.Sleep(10 * time.Second) // warmup

	for {
		w, _, err := cl.GitHub.Actions.ListWorkflowRunsByFileName(ctx, wrk.Owner, wrk.Repo, wrk.File, opts)
		if err != nil {
			return nil, fmt.Errorf("list workflow runs error: %w", err)
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
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, fmt.Errorf("looking for workflow run has timed out after %s", cl.MaxLookup)
		case <-tick.C:
			// continue
		}
	}

	fmt.Fprintf(os.Stderr, "🛠  The dispatched workflow is runnng at %s\n", *workflow.HTMLURL)

poll:
	for {
		select {
		case <-tick.C:
			workflow, _, err = cl.GitHub.Actions.GetWorkflowRunByID(ctx, wrk.Owner, wrk.Repo, *workflow.ID)
			if err != nil {
				return nil, fmt.Errorf("get workflow run error: %w", err)
			}

			fmt.Fprintf(os.Stderr, "🛠  Workflow status: %q [%s]\n", *workflow.Status, *workflow.HTMLURL)

			if c := workflow.Conclusion; c != nil && *c != "" {
				break poll
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if got, want := *workflow.Conclusion, "success"; got != want {
		return nil, fmt.Errorf("undesired workflow status: got %q, want %q [%s]", got, want, *workflow.Status)
	}

	outputs = make(map[string]any)

	jobs, _, err := cl.GitHub.Actions.ListWorkflowJobs(ctx, wrk.Owner, wrk.Repo, *workflow.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("list workflow jobs error: %w", err)
	}

	_ = jobs

	return outputs, nil
}

func (cl *Client) templateInputs(ctx context.Context, inputs, m map[string]any) error {
	for k, v := range inputs {
		var s string
		if v != nil {
			s = fmt.Sprint(v)
		}

		p, err := template.Execute(s, m)
		if err != nil {
			return fmt.Errorf("%s: template error: %w", k, err)
		}

		inputs[k] = string(p)
	}

	return nil
}
