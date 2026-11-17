package expr

import (
	"sort"

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

type dependencyExaminer struct {
	vars  map[string]*Dependency
	calls map[string]*Call
}

// ref is a resolved reference expression, such as "a.b.c" or "a[0]".
type ref struct {
	path    string
	indexed bool
	local   bool
	ok      bool
}

func (e *dependencyExaminer) walk(node ast.Node) {
	switch n := node.(type) {
	case nil:
	case *ast.NilNode, *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode,
		*ast.StringNode, *ast.BytesNode, *ast.ConstantNode, *ast.PointerNode:
		// Literals, builtin constants and predicate pointers are not dependencies.
	case *ast.IdentifierNode:
		e.walkRef(n)
	case *ast.UnaryNode:
		e.walkRef(n.Node)
	case *ast.BinaryNode:
		e.walkRef(n.Left)
		e.walkRef(n.Right)
	case *ast.ChainNode:
		e.walkRef(n.Node)
	case *ast.MemberNode:
		e.walkRef(n)
	case *ast.SliceNode:
		e.walkRef(n.Node)
		e.walk(n.From)
		e.walk(n.To)
	case *ast.CallNode:
		e.walk(n.Callee)
		for _, arg := range n.Arguments {
			e.walk(arg)
		}
	case *ast.BuiltinNode:
		for _, arg := range n.Arguments {
			e.walk(arg)
		}
	case *ast.PredicateNode:
		e.walk(n.Node)
	case *ast.VariableDeclaratorNode:
		e.walk(n.Value)
		e.walk(n.Expr)
	case *ast.SequenceNode:
		for _, n := range n.Nodes {
			e.walk(n)
		}
	case *ast.ConditionalNode:
		e.walkRef(n.Cond)
		e.walk(n.Exp1)
		e.walk(n.Exp2)
	case *ast.ArrayNode:
		for _, n := range n.Nodes {
			e.walk(n)
		}
	case *ast.MapNode:
		for _, pair := range n.Pairs {
			e.walk(pair)
		}
	case *ast.PairNode:
		e.walk(n.Key)
		e.walk(n.Value)
	}
}

// refOf resolves a chain of identifiers and member accesses into a path.
// It returns ok=false for expressions that are not plain references, so the
// caller can fall back to a structural walk of their subexpressions.
func (e *dependencyExaminer) refOf(node ast.Node) ref {
	switch n := node.(type) {
	case *ast.IdentifierNode:
		return ref{path: n.Value, ok: true}
	case *ast.ChainNode:
		return e.refOf(n.Node)
	case *ast.MemberNode:
		parent := e.refOf(n.Node)
		if !parent.ok {
			return ref{}
		}
		if property, isField := n.Property.(*ast.StringNode); isField {
			return ref{
				path:    parent.path + "." + property.Value,
				indexed: parent.indexed,
				local:   parent.local,
				ok:      true,
			}
		}
		// Index access does not join the path; it only marks it.
		return ref{
			path:    parent.path,
			indexed: true,
			local:   parent.local,
			ok:      true,
		}
	case *ast.SliceNode:
		parent := e.refOf(n.Node)
		if !parent.ok {
			return ref{}
		}
		return ref{
			path:    parent.path,
			indexed: true,
			local:   parent.local,
			ok:      true,
		}
	}
	return ref{}
}

// walkRef handles a reference position: it records the resolved path, or
// walks the expression structurally when it is not a plain reference.
func (e *dependencyExaminer) walkRef(node ast.Node) {
	if r := e.refOf(node); r.ok {
		if !r.local {
			e.add(r.path, r.indexed)
		}
		e.walkRefIndices(node)
		return
	}
	e.walk(node)
}

// walkRefIndices walks the subexpressions embedded in a reference chain, such
// as the index of "a[i][j]" or the bounds of "a[1:2]". The identifier and
// field segments themselves are already accounted for by the resolved path.
func (e *dependencyExaminer) walkRefIndices(node ast.Node) {
	switch n := node.(type) {
	case *ast.ChainNode:
		e.walkRefIndices(n.Node)
	case *ast.MemberNode:
		if _, isField := n.Property.(*ast.StringNode); !isField {
			e.walk(n.Property)
		}
		e.walkRefIndices(n.Node)
	case *ast.SliceNode:
		e.walk(n.From)
		e.walk(n.To)
		e.walkRefIndices(n.Node)
	}
}

func (e *dependencyExaminer) add(path string, indexed bool) {
	if e.vars == nil {
		e.vars = map[string]*Dependency{}
	}
	dep, ok := e.vars[path]
	if !ok {
		dep = &Dependency{Path: path}
		e.vars[path] = dep
	}
	dep.Count++
	if indexed {
		dep.Indexed = true
	}
}

func (e *dependencyExaminer) result() *Dependencies {
	deps := &Dependencies{}
	for _, dep := range e.vars {
		deps.Vars = append(deps.Vars, *dep)
	}
	sort.Slice(deps.Vars, func(i, j int) bool {
		return deps.Vars[i].Path < deps.Vars[j].Path
	})
	for _, call := range e.calls {
		deps.Calls = append(deps.Calls, *call)
	}
	sort.Slice(deps.Calls, func(i, j int) bool {
		return deps.Calls[i].Name < deps.Calls[j].Name
	})
	return deps
}
