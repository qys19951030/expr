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
		{`a.b + a.b.c`, map[string]int{"a.b": 1, "a.b.c": 1}, nil},
		{`a.b[c].d`, map[string]int{"a.b.d": 1, "c": 1}, nil},
		{`a[i]`, map[string]int{"a": 1, "i": 1}, nil},
		{`a["key"].b`, map[string]int{"a.b": 1}, nil},
		{`a.b[0] + a.b`, map[string]int{"a.b": 2}, nil},
		{`a[1:2]`, map[string]int{"a": 1}, nil},
		{`a[i:j]`, map[string]int{"a": 1, "i": 1, "j": 1}, nil},
		{`foo.bar.baz.qux`, map[string]int{"foo.bar.baz.qux": 1}, nil},

		// Builtin functions are not dependencies, their arguments are.
		{`len(a)`, map[string]int{"a": 1}, nil},
		{`mean(items) + stats.avg`, map[string]int{"items": 1, "stats.avg": 1}, nil},
		{`sum(items, # * rate)`, map[string]int{"items": 1, "rate": 1}, nil},
		{`len(a.b) + len(c)`, map[string]int{"a.b": 1, "c": 1}, nil},
		{`filter(items, .Price > 100)`, map[string]int{"items": 1}, nil},
		{`map(items, .Price * tax)`, map[string]int{"items": 1, "tax": 1}, nil},

		// Calls of non-builtin names are dependencies of their own.
		{`fn(a)`, map[string]int{"fn": 1, "a": 1}, nil},
		{`fn(a) + fn(b)`, map[string]int{"fn": 2, "a": 1, "b": 1}, nil},
		{`a + myFn()`, map[string]int{"a": 1, "myFn": 1}, nil},
		{`foo(bar(baz))`, map[string]int{"foo": 1, "bar": 1, "baz": 1}, nil},
		{`items | filter(.Price > limit)`, map[string]int{"items": 1, "limit": 1}, nil},

		// Let bindings are local and do not leak as dependencies.
		{`let x = 1; x + y`, map[string]int{"y": 1}, nil},
		{`let x = a; x.b`, map[string]int{"a": 1}, nil},
		{`let x = a; x + x`, map[string]int{"a": 1}, nil},
		{`let x = 1; let y = x + z; x + y`, map[string]int{"z": 1}, nil},
		// Shadowing: the nearest binding wins, outer names do not leak.
		{`let x = a; let x = b; x + x`, map[string]int{"a": 1, "b": 1}, nil},
		{`let x = a; (let x = b; x) + x`, map[string]int{"a": 1, "b": 1}, nil},
		{`let x = a; let x = x + 1; x`, map[string]int{"a": 1}, nil},
		{`let x = x; x`, map[string]int{"x": 1}, nil},
		{`let x = 1; if x > 0 {let y = 2; y + z} else {x + w}`, map[string]int{"z": 1, "w": 1}, nil},
		{`let x = 1; (let y = 2; y) + x`, map[string]int{}, nil},
		{`map(items, let x = .a; let x = x + 1; x)`, map[string]int{"items": 1}, nil},
		{`let fn = a; fn()`, map[string]int{"a": 1}, nil},
		{`let x = a; x.Foo()`, map[string]int{"a": 1}, map[string]int{"Foo": 1}},
		// Local bindings apply inside predicates.
		{`map(items, let p = .Price; p * qty)`, map[string]int{"items": 1, "qty": 1}, nil},
		{`map(items, let x = tax; .Price * x)`, map[string]int{"items": 1, "tax": 1}, nil},
		{`let x = a; map(items, x + y)`, map[string]int{"a": 1, "items": 1, "y": 1}, nil},
		{`map(1..3, let x = #; let y = x * x; y * y)`, map[string]int{}, nil},
		// Predicate parameters (#, #index, #acc) are not dependencies.
		{`filter(items, # > threshold)`, map[string]int{"items": 1, "threshold": 1}, nil},
		{`reduce(items, #acc + # + extra, init)`, map[string]int{"items": 1, "extra": 1, "init": 1}, nil},
		{`all(1..3, { all(1..3, { # > 0 }) and # > 0 })`, map[string]int{}, nil},

		// Literals and builtin constants are not dependencies, but variables
		// nested inside literals are still discovered.
		{`nil`, map[string]int{}, nil},
		{`true && false`, map[string]int{}, nil},
		{`42 + 3.5`, map[string]int{}, nil},
		{`"a.b.c"`, map[string]int{}, nil},
		{`[1, 2, 3]`, map[string]int{}, nil},
		{`{"k": 1}`, map[string]int{}, nil},
		{`[a, b.c, [d.e]]`, map[string]int{"a": 1, "b.c": 1, "d.e": 1}, nil},
		{`{"k": a.b, "j": [c]}`, map[string]int{"a.b": 1, "c": 1}, nil},
		{`a > 0 ? b : c`, map[string]int{"a": 1, "b": 1, "c": 1}, nil},
		{`(a + 1) * 2`, map[string]int{"a": 1}, nil},
		{`!a`, map[string]int{"a": 1}, nil},
		{`-a`, map[string]int{"a": 1}, nil},
		{`a ?? b`, map[string]int{"a": 1, "b": 1}, nil},
		{`1..n`, map[string]int{"n": 1}, nil},
		{`a in b.c`, map[string]int{"a": 1, "b.c": 1}, nil},
		{`not a.b`, map[string]int{"a.b": 1}, nil},
		{`[1, 2, 3][0]`, map[string]int{}, nil},
		{`{"a": 1}.a`, map[string]int{}, nil},

		// Method calls: receiver is a dependency, method name is a call marker.
		{`a.Foo()`, map[string]int{"a": 1}, map[string]int{"Foo": 1}},
		{`a.b.Foo() + a.b.Bar()`, map[string]int{"a.b": 2}, map[string]int{"Foo": 1, "Bar": 1}},
		{`a.Foo(b.c)`, map[string]int{"a": 1, "b.c": 1}, map[string]int{"Foo": 1}},
		{`a.Foo() + a.Foo()`, map[string]int{"a": 2}, map[string]int{"Foo": 2}},
		{`a.Bar().Baz()`, map[string]int{"a": 1}, map[string]int{"Bar": 1, "Baz": 1}},
		{`a[0].Foo()`, map[string]int{"a": 1}, map[string]int{"Foo": 1}},
		{`x.Foo().Bar()`, map[string]int{"x": 1}, map[string]int{"Foo": 1, "Bar": 1}},
		{`f(x).Foo()`, map[string]int{"f": 1, "x": 1}, map[string]int{"Foo": 1}},
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

func TestExamineDependencies_OrderingAndStability(t *testing.T) {
	code := `z + a.b + m[0] + a.b + c.Foo() + b`

	first, err := expr.ExamineDependencies(code)
	require.NoError(t, err)

	expected := []expr.Dependency{
		{Path: "a.b", Count: 2},
		{Path: "b", Count: 1},
		{Path: "c", Count: 1},
		{Path: "m", Count: 1, Indexed: true},
		{Path: "z", Count: 1},
	}
	require.Equal(t, expected, first.Vars)
	require.Equal(t, []expr.MethodCall{{Name: "Foo", Count: 1}}, first.Calls)

	sortCall, err := expr.ExamineDependencies(`a.Zeta() + a.Alpha() + a.Zeta()`)
	require.NoError(t, err)
	require.Equal(t, []expr.MethodCall{
		{Name: "Alpha", Count: 1},
		{Name: "Zeta", Count: 2},
	}, sortCall.Calls)

	for i := 0; i < 10; i++ {
		next, err := expr.ExamineDependencies(code)
		require.NoError(t, err)
		require.Equal(t, first, next)
	}
}

func TestExamineDependencies_Empty(t *testing.T) {
	deps, err := expr.ExamineDependencies(`1 + 2`)
	require.NoError(t, err)
	require.NotNil(t, deps)
	require.Empty(t, deps.Vars)
	require.Empty(t, deps.Calls)
}

func TestExamineDependencies_InvalidExpression(t *testing.T) {
	_, err := expr.ExamineDependencies(`a +`)
	require.Error(t, err)
}

func TestExamineDependencies_Indexed(t *testing.T) {
	tests := []struct {
		code string
		want []expr.Dependency
	}{
		{`a.b[0] + a.b`, []expr.Dependency{{Path: "a.b", Count: 2, Indexed: true}}},
		{`a["key"]`, []expr.Dependency{{Path: "a", Count: 1, Indexed: true}}},
		{`a["key"].b`, []expr.Dependency{{Path: "a.b", Count: 1, Indexed: true}}},
		{`a[0].b + a.b`, []expr.Dependency{{Path: "a.b", Count: 2, Indexed: true}}},
		{`a[1:2]`, []expr.Dependency{{Path: "a", Count: 1, Indexed: true}}},
		{`a.b`, []expr.Dependency{{Path: "a.b", Count: 1}}},
		{`a["b c"].d`, []expr.Dependency{{Path: "a.d", Count: 1, Indexed: true}}},
		{`a[b].c`, []expr.Dependency{{Path: "a.c", Count: 1, Indexed: true}, {Path: "b", Count: 1}}},
		{`a.b["c"].d`, []expr.Dependency{{Path: "a.b.d", Count: 1, Indexed: true}}},
		{`a?.["k"]`, []expr.Dependency{{Path: "a", Count: 1, Indexed: true}}},
		{`a?.b`, []expr.Dependency{{Path: "a.b", Count: 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			deps, err := expr.ExamineDependencies(tt.code)
			require.NoError(t, err)
			require.Equal(t, tt.want, deps.Vars)
		})
	}
}
