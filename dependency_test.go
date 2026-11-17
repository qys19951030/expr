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
		// filled in later
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
