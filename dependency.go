package expr

import (
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// Dependency describes a single external input referenced by an expression.
type Dependency struct {
	// Path is the dotted path of the dependency, like "a.b.c".
	// Index access does not participate in path building: "a[0]" yields "a".
	Path string
	// Count is the number of times the path occurs in the expression.
	Count int
	// Indexed reports whether an index or slice access occurred on the path.
	Indexed bool
}

// Call describes a method name invoked on a value, like "Foo" in "a.Foo()".
// Call names are reported separately and are not variable paths.
type Call struct {
	// Name is the name of the called method.
	Name string
	// Count is the number of times the method is called.
	Count int
}

// Dependencies is the result of ExamineDependencies.
type Dependencies struct {
	// Vars lists external variable dependencies in lexicographic path order.
	Vars []Dependency
	// Calls lists method call names in lexicographic order.
	Calls []Call
}

// ExamineDependencies parses the expression and returns all external data
// paths it depends on. Builtin functions, local variable bindings and
// literals are not reported as dependencies.
func ExamineDependencies(input string) (*Dependencies, error) {
	tree, err := parser.Parse(input)
	if err != nil {
		return nil, err
	}

	examiner := &dependencyExaminer{}

	examiner.walk(tree.Node)

	return examiner.result(), nil
}

type dependencyExaminer struct{}

func (e *dependencyExaminer) walk(node ast.Node) {}

func (e *dependencyExaminer) result() *Dependencies {
	return &Dependencies{}
}
