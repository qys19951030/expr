package expr_test

import (
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/internal/testify/require"
)

func TestExamineDependencies(t *testing.T) {
	tests := []struct {
		code string
		vars map[string]int
	}{
		// Nested fields and indices.
		{`a`, map[string]int{"a": 1}},
		{`a.b.c`, map[string]int{"a.b.c": 1}},
		{`a.b.c + a.b.c`, map[string]int{"a.b.c": 2}},
		{`a[0]`, map[string]int{"a": 1}},
		{`a[0].b`, map[string]int{"a.b": 1}},
		{`a[0].b[1].c`, map[string]int{"a.b.c": 1}},
		{`a[i]`, map[string]int{"a": 1, "i": 1}},
		{`a["key"].b`, map[string]int{"a.key.b": 1}},
		{`a.b[0] + a.b`, map[string]int{"a.b": 2}},
		{`a[1:2]`, map[string]int{"a": 1}},
		{`a[i:j]`, map[string]int{"a": 1, "i": 1, "j": 1}},
		{`foo.bar.baz.qux`, map[string]int{"foo.bar.baz.qux": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			deps, err := expr.ExamineDependencies(tt.code)
			require.NoError(t, err)

			got := map[string]int{}
			for _, dep := range deps.Vars {
				got[dep.Path] = dep.Count
			}
			require.Equal(t, tt.vars, got)
		})
	}
}

func TestExamineDependencies_Indexed(t *testing.T) {
	deps, err := expr.ExamineDependencies(`a.b[0] + a.b`)
	require.NoError(t, err)
	require.Equal(t, []expr.Dependency{
		{Path: "a.b", Count: 2, Indexed: true},
	}, deps.Vars)
}
