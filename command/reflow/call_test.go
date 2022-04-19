package reflow

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseWorkflow(t *testing.T) {
	cases := map[string]*workflow{
		"scyllacloud/scylla-cloud/.github/workflows/deploy.yaml@master": {
			"scyllacloud",
			"scylla-cloud",
			"deploy.yaml",
			"master",
		},
		"tectumsh/tectum/.github/workflows/build-and-push-image.yaml@refs/heads/deploy/lab": {
			"tectumsh",
			"tectum",
			"build-and-push-image.yaml",
			"refs/heads/deploy/lab",
		},
		"rjeczalik/clef/.github/workflows/release.yaml@0850e2124b8d32d99d2d30865372e0f722c39a5f": {
			"rjeczalik",
			"clef",
			"release.yaml",
			"0850e2124b8d32d99d2d30865372e0f722c39a5f",
		},
	}

	for s, want := range cases {
		got, err := parseWorkflow(s)
		if err != nil {
			t.Fatalf("%s: parseWorkflow()=%+v", s, err)
		}

		if !cmp.Equal(want, got) {
			t.Fatalf("got != want:\n%s", cmp.Diff(want, got))
		}
	}
}
