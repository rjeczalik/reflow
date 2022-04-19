package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"rafal.dev/refmt/object"

	"github.com/Masterminds/sprig/v3"
	"github.com/google/go-github/v43/github"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

type App struct {
	ctx context.Context

	GitHubContext string
	ValuesContext string

	Owner  string
	Repo   string
	Format string

	GitHub   *github.Client
	Template *Template
	Funcs    template.FuncMap

	token string
}

func envMarshal(v interface{}, prefix string) ([]byte, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, errors.New("envCoded: cannot marshal non-object value")
	}

	var (
		envs = object.Flatten(m, "_")
		keys = object.Keys(envs)
		buf  bytes.Buffer
	)

	for _, k := range keys {
		fmt.Fprintf(&buf, "%s%s=%s\n", prefix, strings.ToUpper(k), envs[k])
	}

	return buf.Bytes(), nil
}

var funcs = map[string]any{
	"toYaml": func(v interface{}) string {
		p, _ := yaml.Marshal(v)
		return string(p)
	},
	"mustToYaml": func(v interface{}) (string, error) {
		p, err := yaml.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(p), nil
	},
	"fromYaml": func(s string) (v interface{}) {
		yaml.Unmarshal([]byte(s), &v)
		return v
	},
	"mustFromYaml": func(s string) (v interface{}, err error) {
		if err = yaml.Unmarshal([]byte(s), &v); err != nil {
			return nil, err
		}
		return v, nil
	},
	"toEnv": func(v interface{}) string {
		p, _ := envMarshal(v, "")
		return string(p)
	},
	"mustToEnv": func(v interface{}) (string, error) {
		p, err := envMarshal(v, "")
		if err != nil {
			return "", err
		}
		return string(p), nil
	},
	"toEnvPrefix": func(prefix string, v interface{}) string {
		p, _ := envMarshal(v, prefix)
		return string(p)
	},
	"mustToEnvPrefix": func(prefix string, v interface{}) (string, error) {
		p, err := envMarshal(v, prefix)
		if err != nil {
			return "", err
		}
		return string(p), nil
	},
}

func NewApp(ctx context.Context) *App {
	app := &App{
		ctx:   ctx,
		Funcs: template.FuncMap(sprig.FuncMap()),
		token: os.Getenv("GITHUB_TOKEN"),
	}

	for name, fn := range funcs {
		app.Funcs[name] = fn
	}

	return app
}

func (app *App) Context() context.Context {
	return app.ctx
}

func (app *App) Register(f *pflag.FlagSet) {
	f.StringVar(&app.GitHubContext, "context-github", "", `The "github" context encoded as JSON`)
	f.StringVar(&app.ValuesContext, "context-values", "", `The "values" context encoded as JSON`)
	f.StringVarP(&app.Owner, "owner", "o", "", "Repository owner")
	f.StringVarP(&app.Repo, "repo", "r", "", "Repository name")
	f.StringVarP(&app.Format, "format", "f", "yaml", "Format encoding of the output.")
}

func (app *App) Init(next CobraFunc) CobraFunc {
	app.GitHub = github.NewClient(oauth2.NewClient(app.Context(), oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: app.token,
		},
	)))

	return func(cmd *cobra.Command, args []string) error {
		if !cmd.Flag("context-github").Changed {
			app.GitHubContext = os.Getenv("REFLOW_CONTEXT_GITHUB")
		}

		if !cmd.Flag("context-values").Changed {
			app.ValuesContext = os.Getenv("REFLOW_CONTEXT_VALUES")
		}

		if app.GitHubContext != "" {
			if err := app.buildGithubContext(cmd); err != nil {
				return fmt.Errorf("github context build error: %w", err)
			}
		}

		if app.ValuesContext != "" {
			if err := app.buildValuesContext(cmd); err != nil {
				return fmt.Errorf("values context build error: %w", err)
			}
		}

		return next(cmd, args)
	}
}

func (app *App) buildValuesContext(cmd *cobra.Command) error {
	app.initTemplate()

	p, err := ioutil.ReadFile(app.ValuesContext)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	t, err := template.New(app.ValuesContext).Funcs(app.Funcs).Parse(string(p))
	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	var buf bytes.Buffer

	if err := t.Execute(&buf, app.Template.JSON()); err != nil {
		return fmt.Errorf("error evaluating template: %w", err)
	}

	if err := yaml.Unmarshal(buf.Bytes(), &app.Template.Values); err != nil {
		return fmt.Errorf("error unmarshaling template: %w", err)
	}

	return nil
}

func (app *App) initTemplate() {
	if app.Template == nil {
		app.Template = &Template{
			Git: new(Git),
			GitHub: Map{
				"token": app.token,
			},
		}
	}
}

func (app *App) buildGithubContext(cmd *cobra.Command) error {
	app.initTemplate()

	f, err := os.Open(app.GitHubContext)
	if err != nil {
		return fmt.Errorf("error reading: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.UseNumber()

	if err := dec.Decode(&app.Template.GitHub); err != nil {
		return fmt.Errorf("error unmarshaling: %w", err)
	}

	app.Template.GitHub["token"] = app.token

	switch r, ok := Get[string](app.gh(), "repository"); ok {
	case true:
		v := strings.Split(r, "/")
		if len(v) != 2 {
			break
		}

		if !cmd.Flag("owner").Changed {
			app.Owner = v[0]
		}

		if !cmd.Flag("repo").Changed {
			app.Repo = v[1]
		}
	}

	event, ok := Get[string](app.gh(), "event_name")
	if !ok {
		return fmt.Errorf("unable to read: %q", "event_name")
	}

	switch event {
	case "issue_comment":
		if err := app.buildIssueComment(cmd); err != nil {
			return fmt.Errorf("error building: %w", err)
		}
	case "push":
		if err := app.buildPush(cmd); err != nil {
			return fmt.Errorf("error building: %w", err)
		}
	case "pull_request":
		if err := app.buildPullRequest(cmd); err != nil {
			return fmt.Errorf("error building: %w", err)
		}
	default:
		return fmt.Errorf("unsupported event type: %q", err)
	}

	return nil
}

func (app *App) buildIssueComment(cmd *cobra.Command) error {
	if m, ok := Get[map[string]any](app.gh(), "event.issue.pull_request"); !ok || len(m) == 0 {
		return errors.New("issue is not a pull request")
	}

	v, ok := Get[json.Number](app.gh(), "event.issue.number")
	if !ok {
		return errors.New(`unable to read "event.issue.number"`)
	}

	num, err := v.Int64()
	if err != nil {
		return fmt.Errorf(`unable to read "event.issue.number": %w`, err)
	}

	pr, _, err := app.GitHub.PullRequests.Get(app.Context(), app.Owner, app.Repo, int(num))
	if err != nil {
		return fmt.Errorf("error getting pull request: %w", err)
	}

	app.Template.Git.Head.Ref = *pr.Head.Ref
	app.Template.Git.Head.SHA = *pr.Head.SHA

	return nil
}

func (app *App) buildPush(cmd *cobra.Command) error {
	app.Template.Git.Head.Ref, _ = Get[string](app.gh(), "ref")
	app.Template.Git.Head.SHA, _ = Get[string](app.gh(), "sha")

	return nil
}

func (app *App) buildPullRequest(cmd *cobra.Command) error {
	app.Template.Git.Head.Ref, _ = Get[string](app.gh(), "event.pull_request.head.ref")
	app.Template.Git.Head.SHA, _ = Get[string](app.gh(), "event.pull_request.head.sha")

	return nil
}

func (app *App) gh() Map {
	return app.Template.GitHub
}

type Template struct {
	Git    *Git `json:"git,omitempty"`
	GitHub Map  `json:"github,omitempty"`
	Values Map  `json:"values,omitempty"`
}

func (t *Template) JSON() any {
	if t == nil {
		return nil
	}

	p, err := json.Marshal(t)
	if err != nil {
		panic("unexpected error: " + err.Error())
	}

	var v any
	if err := json.Unmarshal(p, &v); err != nil {
		panic("unexpected error: " + err.Error())
	}

	return v
}

type Git struct {
	Head struct {
		Ref string `json:"ref,omitempty"`
		SHA string `json:"sha,omitempty"`
	} `json:"head,omitempty"`
}

type Map map[string]any

func Get[T any](m Map, path string) (t T, ok bool) {
	var it map[string]any = m
	keys := strings.Split(path, ".")

	for _, k := range keys[:len(keys)-1] {
		if it, ok = it[k].(map[string]any); !ok {
			return t, false
		}
	}

	t, ok = it[keys[len(keys)-1]].(T)
	return t, ok
}
