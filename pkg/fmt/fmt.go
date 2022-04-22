package fmt

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	c "rafal.dev/reflow/pkg/context"
	"rafal.dev/reflow/pkg/template"

	"gopkg.in/yaml.v3"
)

var DefaultFormater = &Formater{
	Builder: c.DefaultBuilder,
}

type Formater struct {
	Builder c.Builder
}

func (f *Formater) Format(ctx context.Context, in, out string, mask bool) error {
	m := make(map[string]any)

	if err := f.Builder.Build(ctx, m); err != nil {
		return fmt.Errorf("formatter error:	%w", err)
	}

	if mask {
		v, err := c.Get[[]any](m, "mask")
		if err != nil {
			return fmt.Errorf("mask keys: %w", err)
		}

		for _, v := range v {
			if key := fmt.Sprint(v); c.Del(m, key) {
				c.Set(m, key, "***")
			}
		}
	}

	p, err := ioutil.ReadFile(in)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if p, err = template.Execute(string(p), m); err != nil {
		return fmt.Errorf("template execute: %w", err)
	}

	var v any

	if err := f.unmarshal(p, in, &v); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	if err := f.Marshal(v, out); err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	return nil
}

func (f *Formater) Marshal(v any, file string) (err error) {
	var p []byte

	switch ext := strings.ToLower(filepath.Ext(file)); ext {
	case ".json":
		p, err = json.Marshal(v)
	case ".yaml", ".yml":
		p, err = yaml.Marshal(v)
	default:
		err = fmt.Errorf("unsupported format: %q", ext)
	}

	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	if err := ioutil.WriteFile(file, p, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (f *Formater) Unmarshal(file string, v any) error {
	return f.unmarshal(nil, file, v)
}

func (f *Formater) unmarshal(p []byte, file string, v any) (err error) {
	if p == nil {
		if p, err = ioutil.ReadFile(file); err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	}

	switch ext := strings.ToLower(filepath.Ext(file)); ext {
	case ".json", ".yaml", ".yml":
		err = yaml.Unmarshal(p, v)
	default:
		err = fmt.Errorf("unsupported format: %q", ext)
	}

	return nil
}
