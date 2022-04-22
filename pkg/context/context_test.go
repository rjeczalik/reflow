package context

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGetSetDel(t *testing.T) {
	const (
		set = 1 << iota
		get
		del
	)

	m := make(map[string]any)

	cases := []struct {
		op int
		k  string
		v  any
		ok bool
	}{
		0: {
			op: set,
			k:  "git.head",
			v:  "sha",
			ok: false,
		},
		1: {
			op: set,
			k:  "git.ref",
			v:  "bar",
			ok: false,
		},
		2: {
			op: get,
			k:  "git.head",
			v:  "sha",
			ok: true,
		},
		3: {
			op: set,
			k:  "git.head",
			v:  "sha2",
			ok: true,
		},
		4: {
			op: set,
			k:  "github.event.pull_request.issue.number",
			v:  "1",
			ok: false,
		},
		5: {
			op: get,
			k:  "github.event.pull_request.issue.number",
			v:  "1",
			ok: true,
		},
		6: {
			op: set,
			k:  "github.event.pull_request",
			v:  nil,
			ok: true,
		},
		7: {
			op: del,
			k:  "github.event",
			ok: true,
		},
		8: {
			op: get,
			k:  "github.event",
			ok: false,
		},
	}

	for i, cas := range cases {
		t.Run("", func(t *testing.T) {
			switch cas.op {
			case set:
				if ok := Set(m, cas.k, cas.v); ok != cas.ok {
					t.Errorf("Set(): got %t, want %t", ok, cas.ok)
				}
			case get:
				switch v, err := Get[string](m, cas.k); {
				case (cas.ok && err != nil) || (!cas.ok && err == nil):
					t.Errorf("Get(): got %v, want %t", err, cas.ok)
				case cas.ok:
					if !cmp.Equal(v, cas.v) {
						t.Errorf("Get(): got != want:\n%s", cmp.Diff(v, cas.v))
					}
				}
			case del:
				if ok := Del(m, cas.k); ok != cas.ok {
					t.Errorf("Del(): got %t, want %t", ok, cas.ok)
				}
			default:
				panic(fmt.Errorf("%d: unrecognized op: %d", i, cas.op))
			}
		})
	}
}
