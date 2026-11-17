package expr_test

import (
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/internal/testify/require"
)

func TestExamineDependencies(t *testing.T) {
	tests := []struct {
		code  string
		vars  map[string]int
		calls map[string]int
	}{
		// Nested fields and indices.
		{`a`, map[string]int{"a": 1}, nil},
		{`a.b.c`, map[string]int{"a.b.c": 1}, nil},
		{`a.b.c + a.b.c`, map[string]int{"a.b.c": 2}, nil},
		{`a[0]`, map[string]int{"a": 1}, nil},
		{`a[0].b`, map[string]int{"a.b": 1}, nil},
		{`a[0].b[1].c`, map[string]int{"a.b.c": 1}, nil},
		{`a[i]`, map[string]int{"a": 1, "i": 1}, nil},
		{`a["key"].b`, map[string]int{"a.key.b": 1}, nil},
		{`a.b[0] + a.b`, map[string]int{"a.b": 2}, nil},
		{`a[1:2]`, map[string]int{"a": 1}, nil},
		{`a[i:j]`, map[string]int{"a": 1, "i": 1, "j": 1}, nil},
		{`foo.bar.baz.qux`, map[string]int{"foo.bar.baz.qux": 1}, nil},

		// Builtin functions are not dependencies, their arguments are.
		{`len(a)`, map[string]int{"a": 1}, nil},
		{`len(a.b) + len(c)`, map[string]int{"a.b": 1, "c": 1}, nil},
		{`filter(items, .Price > 100)`, map[string]int{"items": 1}, nil},
		{`map(items, .Price * tax)`, map[string]int{"items": 1, "tax": 1}, nil},

		// Calls of non-builtin names are dependencies of their own.
		{`fn(a)`, map[string]int{"fn": 1, "a": 1}, nil},
		{`fn(a) + fn(b)`, map[string]int{"fn": 2, "a": 1, "b": 1}, nil},
		{`a + myFn()`, map[string]int{"a": 1, "myFn": 1}, nil},

		// Method calls: receiver is a dependency, method name is a call marker.
		{`a.Foo()`, map[string]int{"a": 1}, map[string]int{"Foo": 1}},
		{`a.b.Foo() + a.b.Bar()`, map[string]int{"a.b": 2}, map[string]int{"Foo": 1, "Bar": 1}},
		{`a.Foo(b.c)`, map[string]int{"a": 1, "b.c": 1}, map[string]int{"Foo": 1}},
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

			calls := map[string]int{}
			for _, call := range deps.Calls {
				calls[call.Name] = call.Count
			}
			if tt.calls == nil {
				require.Empty(t, calls)
			} else {
				require.Equal(t, tt.calls, calls)
			}
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
