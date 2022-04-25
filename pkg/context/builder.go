package context

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"rafal.dev/reflow/internal/misc"
	"rafal.dev/reflow/pkg/debug"
	"rafal.dev/reflow/pkg/template"

	"gopkg.in/yaml.v3"
)

var Builtin = []string{
	"manifest",
	"github",
	"values",
	"reflow",
}

var DefaultBuilder Builder = SeqBuilder{
	&DirBuilder{Dir: misc.HomeDir("context")},
	&ReflowBuilder{Client: misc.GitHub(context.Background())},
	&DirBuilder{Dir: misc.HomeDir("templates"), Conv: Template, Exclude: Builtin},
}

type SeqBuilder []Builder

var _ Builder = SeqBuilder(nil)

func (seq SeqBuilder) Build(ctx context.Context, m map[string]any) error {
	for _, b := range seq {
		select {
		case <-ctx.Done():
			return fmt.Errorf("%T: %w", b, ctx.Err())
		default:
		}

		debug.Logf(ctx, "building %T", b)

		if err := b.Build(ctx, m); err != nil {
			return fmt.Errorf("%T: %w", b, err)
		}
	}
	return nil
}

type Builder interface {
	Build(context.Context, map[string]any) error
}

var _ Builder = (*DirBuilder)(nil)

func Build(ctx context.Context) (map[string]any, error) {
	m := make(map[string]any)

	return m, DefaultBuilder.Build(ctx, m)
}

func Template(p []byte, m map[string]any) ([]byte, error) {
	q, err := template.Execute(string(p), m)
	if err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}

	return q, nil
}

type DirBuilder struct {
	Dir     fs.FS
	Exclude []string
	Conv    func([]byte, map[string]any) ([]byte, error)
}

func (db *DirBuilder) Build(ctx context.Context, m map[string]any) error {
	entries, err := fs.ReadDir(db.Dir, ".")
	if err != nil {
		return fmt.Errorf("dir loader: %w", err)
	}

	debug.Logf(ctx, "%T: found %d entries", db, len(entries))

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return fmt.Errorf("dir loader: %w", ctx.Err())
		default:
		}

		if entry.IsDir() {
			continue
		}

		debug.Logf(ctx, "%T: building %q", db, entry.Name())

		var (
			unmarshal func([]byte, any) error
			key       = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		)

		if db.isExcluded(key) {
			debug.Logf(ctx, "%T: excluding %q", db, entry.Name())
			continue
		}

		switch ext := strings.ToLower(filepath.Ext(entry.Name())); ext {
		case ".json", ".yaml", ".yml":
			unmarshal = yaml.Unmarshal
		default:
			debug.Logf(ctx, "%T: no unmarshal found for %q", db, ext)
		}

		if unmarshal == nil {
			continue
		}

		p, err := fs.ReadFile(db.Dir, entry.Name())
		if err != nil {
			return fmt.Errorf("dir loader %q: %w", entry.Name(), err)
		}

		if db.Conv != nil {
			if p, err = db.Conv(p, m); err != nil {
				return fmt.Errorf("dir loader %q: %w", entry.Name(), err)
			}
		}

		var v any

		if err := unmarshal(p, &v); err != nil {
			return fmt.Errorf("dir loader %q: %w", entry.Name(), err)
		}

		m[key] = v
	}

	return nil
}

func (db *DirBuilder) isExcluded(s string) bool {
	for _, ex := range db.Exclude {
		if ex == s {
			return true
		}
	}

	return false
}
