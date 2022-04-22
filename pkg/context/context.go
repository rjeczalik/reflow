package context

import (
	"errors"
	"fmt"
	"strings"
)

type KeyError struct {
	Key   string
	Type  string
	Value interface{}
}

func (ke *KeyError) Error() string {
	if ke.Value != nil {
		return fmt.Sprintf("key %q has invalid type: got %T, want %s", ke.Key, ke.Value, ke.Type)
	}

	return fmt.Sprintf("key %q is missing", ke.Key)
}

func Get[T any](m map[string]any, path string) (t T, err error) {
	var (
		keys = strings.Split(path, ".")
		n    = len(keys) - 1
		it   = m
		ok   bool
	)

	if n < 0 {
		return t, errors.New("empty key")
	}

	for _, k := range keys[:n] {
		if it, ok = it[k].(map[string]any); !ok {
			return t, &KeyError{Key: path, Type: fmt.Sprintf("%T", t)}
		}
	}

	v, ok := it[keys[n]]
	if !ok {
		return t, &KeyError{Key: path, Type: fmt.Sprintf("%T", t)}
	}

	if t, ok = v.(T); !ok {
		return t, &KeyError{Key: path, Type: fmt.Sprintf("%T", t), Value: v}
	}

	return t, nil
}

func Set[T any](m map[string]any, path string, t T) (replaced bool) {
	var (
		keys = strings.Split(path, ".")
		n    = len(keys) - 1
		it   = m
	)

	for _, k := range keys[:n] {
		switch m := it[k].(type) {
		case map[string]any:
			if m == nil {
				m = make(map[string]any)
				it[k] = m
			}

			it = m
		case nil:
			n := make(map[string]any)
			it[k] = n
			it = n
		default:
			replaced = true
			n := make(map[string]any)
			it[k] = n
			it = n
		}
	}

	if _, ok := it[keys[n]]; ok {
		replaced = true
	}

	it[keys[n]] = t

	return replaced
}

func Del(m map[string]any, path string) (ok bool) {
	var (
		keys = strings.Split(path, ".")
		n    = len(keys) - 1
		it   = m
	)

	for _, k := range keys[:n] {
		if it, ok = it[k].(map[string]any); !ok {
			return false
		}
	}

	if _, ok = it[keys[n]]; ok {
		delete(it, keys[n])
		return true
	}

	return false
}
