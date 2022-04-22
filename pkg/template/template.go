package template

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"rafal.dev/refmt/object"

	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

var globalFuncs = FuncMap()

func FuncMap() template.FuncMap {
	return merge(sprig.HermeticTxtFuncMap(),
		map[string]any{
			"toYaml": func(v any) string {
				p, _ := yaml.Marshal(v)
				return string(p)
			},
			"mustToYaml": func(v any) (string, error) {
				p, err := yaml.Marshal(v)
				if err != nil {
					return "", err
				}
				return string(p), nil
			},
			"fromYaml": func(s string) (v any) {
				yaml.Unmarshal([]byte(s), &v)
				return v
			},
			"mustFromYaml": func(s string) (v any, err error) {
				if err = yaml.Unmarshal([]byte(s), &v); err != nil {
					return nil, err
				}
				return v, nil
			},
			"toEnv": func(v any) string {
				p, _ := envMarshal(v, "")
				return string(p)
			},
			"mustToEnv": func(v any) (string, error) {
				p, err := envMarshal(v, "")
				if err != nil {
					return "", err
				}
				return string(p), nil
			},
			"toEnvPrefix": func(prefix string, v any) string {
				p, _ := envMarshal(v, prefix)
				return string(p)
			},
			"mustToEnvPrefix": func(prefix string, v any) (string, error) {
				p, err := envMarshal(v, prefix)
				if err != nil {
					return "", err
				}
				return string(p), nil
			},
		},
	)
}

func Execute(s string, v any) ([]byte, error) {
	t, err := template.New("").Funcs(globalFuncs).Parse(s)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer

	if err := t.Execute(&buf, v); err != nil {
		return nil, fmt.Errorf("template execute error: %w", err)
	}

	return buf.Bytes(), nil
}

func envMarshal(v any, prefix string) ([]byte, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("envCoded: cannot marshal non-object value")
	}

	var (
		envs = object.Flatten(m, "_")
		keys = object.Keys(envs)
		buf  bytes.Buffer
	)

	for _, k := range keys {
		fmt.Fprintf(&buf, "%s%s=%s\n", prefix, strings.ToUpper(k), fmt.Sprint(envs[k]))
	}

	return bytes.TrimSpace(buf.Bytes()), nil
}

func merge(m, mixin template.FuncMap) template.FuncMap {
	for k, v := range mixin {
		if _, ok := m[k]; ok {
			panic("unexpected: func " + k + " is already defined!")
		}

		m[k] = v
	}

	return m
}
