package reflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

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

	if len(x) < len(reUses.SubexpNames()) {
		return nil, fmt.Errorf("syntax ill-formed: %q", s)
	}

	for i, group := range reUses.SubexpNames() {
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
		return nil, fmt.Errorf("parseWorkflow: unmarshal error: %w", err)
	}

	if !strings.HasPrefix(w.Branch, "heads/") && !strings.HasPrefix(w.Branch, "tags/") {
		w.Branch = "heads/" + w.Branch
	}

	return &w, nil
}

func (w *workflow) String() string {
	return w.Owner + "/" + w.Repo + "/.github/workflows/" + w.File + "@" + w.Branch
}
