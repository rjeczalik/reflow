package reflow

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rafal.dev/reflow/internal/misc"
	c "rafal.dev/reflow/pkg/context"
	"rafal.dev/reflow/pkg/debug"
	f "rafal.dev/reflow/pkg/fmt"
	"rafal.dev/reflow/pkg/template"

	"github.com/google/go-github/v43/github"
	"gopkg.in/yaml.v3"
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
		runOutputs   = filepath.Join(runHome, "outputs", "outputs.json")

		home          = cl.Home
		homeContext   = filepath.Join(home, "context")
		homeTemplates = filepath.Join(home, "templates")
	)

	b := c.SeqBuilder{
		&c.DirBuilder{Dir: os.DirFS(runContext)},
		&c.ReflowBuilder{Client: cl.GitHub},
		&c.DirBuilder{Dir: os.DirFS(homeContext), Exclude: c.Builtin},
		&c.DirBuilder{Dir: os.DirFS(homeTemplates), Conv: c.Template, Exclude: c.Builtin},
		&c.DirBuilder{Dir: os.DirFS(runTemplates), Conv: c.Template},
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

	arts, _, err := cl.GitHub.Actions.ListWorkflowRunArtifacts(ctx, wrk.Owner, wrk.Repo, *workflow.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("list workflow jobs error: %w", err)
	}

	fmt.Println("[DEBUG]", len(arts.Artifacts))
	// var output *url.URL

	const max = 1 * 1024 * 1024 // 1MiB

	for _, art := range arts.Artifacts {
		debug.Logf(ctx, `looking up "reflow-outputs" artifact: %q"`, *art.Name)

		if *art.Name == "reflow-outputs" {
			u, _, err := cl.GitHub.Actions.DownloadArtifact(ctx, wrk.Owner, wrk.Repo, *art.ID, true)
			if err != nil {
				return nil, fmt.Errorf("download artifact error: %w", err)
			}

			resp, err := http.Get(u.String())
			if err != nil {
				return nil, fmt.Errorf("get artifact error: %w", err)
			}
			defer resp.Body.Close()

			p, err := io.ReadAll(io.LimitReader(resp.Body, max))
			if err != nil {
				return nil, fmt.Errorf("read artifact error: %w", err)
			}

			r, err := zip.NewReader(bytes.NewReader(p), int64(len(p)))
			if err != nil {
				return nil, fmt.Errorf("open zip archive error: %w", err)
			}

			for _, f := range r.File {
				debug.Logf(ctx, "reading artifact files: %q", f.Name)

				fr, err := f.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open file %q: %w", f.Name, err)
				}

				var (
					m   map[string]any
					dec = yaml.NewDecoder(fr)
					key = strings.TrimSuffix(filepath.Base(f.Name), filepath.Ext(f.Name))
				)

				err = dec.Decode(&m)
				_ = fr.Close()
				if err != nil {
					return nil, fmt.Errorf("failed to decode file %q: %w", f.Name, err)
				}

				if key == "outputs" {
					for k, v := range m {
						outputs[k] = v
					}
				} else {
					outputs[key] = m
				}
			}

			break
		}
	}

	p, err := json.Marshal(outputs)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	if err := os.WriteFile(runOutputs, p, 0644); err != nil {
		return nil, fmt.Errorf("file %q write error: %w", runOutputs, err)
	}

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
