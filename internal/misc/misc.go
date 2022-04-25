package misc

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v43/github"
	"golang.org/x/oauth2"
)

func GitHub(ctx context.Context) *github.Client {
	return github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: GitHubToken(),
		},
	)))
}

func GitHubToken() string {
	return Nonzero(os.Getenv("PAT"), os.Getenv("GITHUB_TOKEN"))
}

func ParseList(p []byte) ([]string, error) {
	r := csv.NewReader(bytes.NewReader(p))
	r.FieldsPerRecord = -1

	s, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse list: %q", err)
	}

	var list []string

	for _, s := range s {
		for _, s := range s {
			if s = strings.TrimSpace(s); s != "" {
				list = append(list, s)
			}
		}
	}

	return list, nil
}

func Home() string {
	if s := os.Getenv("REFLOW_HOME"); s != "" {
		_ = initHome(s)

		return s
	}

	if s, err := os.UserConfigDir(); err == nil {
		s = filepath.Join(s, "reflow")

		_ = initHome(s)

		return s
	}

	if u, err := user.Current(); err == nil {
		s := filepath.Join(u.HomeDir, ".config", "reflow")

		_ = initHome(s)

		return s
	}

	panic("unable to read REFLOW_HOME")
}

func HomeDir(dir string) fs.FS {
	return os.DirFS(filepath.Join(Home(), dir))
}

func initHome(dir string) error {
	return Nonil(
		os.MkdirAll(filepath.Join(dir, "context"), 0755),
		os.MkdirAll(filepath.Join(dir, "templates"), 0755),
		os.MkdirAll(filepath.Join(dir, "outputs"), 0755),
	)
}

func Nonil(err ...error) error {
	for _, e := range err {
		if e != nil {
			return e
		}
	}
	return nil
}

func Nonzero[T comparable](t ...T) T {
	var zero T

	for _, t := range t {
		if t != zero {
			return t
		}
	}

	return zero
}
