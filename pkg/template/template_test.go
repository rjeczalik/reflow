package template

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEnvMarshal(t *testing.T) {
	cases := []struct {
		p string
		m map[string]any
		q []byte
	}{
		0: {
			p: "REFLOW_",
			m: map[string]any{"GIT": map[string]any{"HEAD": 123, "REF": "bar"}},
			q: []byte("REFLOW_GIT_HEAD=123\nREFLOW_GIT_REF=bar"),
		},
	}

	for i, cas := range cases {
		t.Run("", func(t *testing.T) {
			q, err := envMarshal(cas.m, cas.p)
			if err != nil {
				t.Fatalf("%d: envMarshal()=%+v", i, err)
			}

			if got, want := string(q), string(cas.q); !cmp.Equal(got, want) {
				t.Fatalf("%d: got != want:\n%s", i, cmp.Diff(got, want))
			}
		})
	}
}
