package manifest

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"rafal.dev/reflow/internal/misc"
	c "rafal.dev/reflow/pkg/context"
	f "rafal.dev/reflow/pkg/fmt"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

var DefaultBuilder = &Builder{
	Home:    misc.Home(),
	Context: c.DefaultBuilder,
	Fmt:     f.DefaultFormater,
}

type Builder struct {
	Home    string
	Context c.Builder
	Fmt     *f.Formater
}

func (b *Builder) Build(ctx context.Context, r io.Reader) error {
	var (
		github = make(map[string]any)
		inputs = make(map[string]any)
	)

	dec := yaml.NewDecoder(r)

	if err := dec.Decode(&github); err != nil {
		return fmt.Errorf("decoding github: %w", err)
	}

	if err := dec.Decode(&inputs); err != nil {
		return fmt.Errorf("decoding inputs: %w", err)
	}

	c.Del(github, "token")
	c.Del(inputs, "token")

	var (
		id  = uuid.New().String()
		run = filepath.Join(b.Home, "runs", id)
	)

	for _, dir := range []string{"context", "templates", "inputs", "outputs"} {
		path := filepath.Join(run, dir)

		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("building manifest %q: %w", path, err)
		}
	}

	var (
		githubFile   = filepath.Join(run, "context", "github.json")
		manifestFile = filepath.Join(run, "context", "manifest.yaml")
		valuesFile   = filepath.Join(run, "templates", "values.yaml")
		inputsFile   = filepath.Join(run, "inputs", "inputs.yaml")
	)

	if err := b.Fmt.Marshal(github, githubFile); err != nil {
		return fmt.Errorf("marshal github: %w", err)
	}

	values, err := c.Get[string](inputs, "values")
	if err != nil {
		return fmt.Errorf("building values: %w", err)
	}

	uses, err := c.Get[string](inputs, "uses")
	if err != nil {
		return fmt.Errorf("building manifest: %w", err)
	}

	wrkInputs, err := c.Get[string](inputs, "inputs")
	if err != nil {
		return fmt.Errorf("building inputs: %w", err)
	}

	dbg, err := c.Get[string](inputs, "debug")
	if err != nil {
		return fmt.Errorf("building manifest: %w", err)
	}

	if err := os.WriteFile(valuesFile, []byte(values), 0644); err != nil {
		return fmt.Errorf("writing values: %w", err)
	}

	if err := os.WriteFile(inputsFile, []byte(wrkInputs), 0644); err != nil {
		return fmt.Errorf("writing inputs: %w", err)
	}

	m := map[string]any{
		"uses":  uses,
		"id":    id,
		"debug": dbg,
	}

	if err := b.Fmt.Marshal(m, manifestFile); err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	fmt.Printf("::set-output name=run-id::%s\n", id)

	return nil
}
