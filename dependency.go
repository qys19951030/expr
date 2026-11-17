package expr

import (
	"sort"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/builtin"
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

// MethodCall describes a method invoked on a value, like "Foo" in "a.Foo()".
// Method names are reported separately and are not variable paths.
type MethodCall struct {
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
	Calls []MethodCall
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
	vars   map[string]*Dependency
	calls  map[string]*MethodCall
	scopes []map[string]bool
}

func (e *dependencyExaminer) pushScope(name string) {
	e.scopes = append(e.scopes, map[string]bool{name: true})
}

func (e *dependencyExaminer) popScope() {
	e.scopes = e.scopes[:len(e.scopes)-1]
}

// isLocal reports whether a name is bound by an enclosing let or closure
// parameter, with the nearest binding taking precedence.
func (e *dependencyExaminer) isLocal(name string) bool {
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if e.scopes[i][name] {
			return true
		}
	}
	return false
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
		e.walkRef(n)
	case *ast.CallNode:
		e.walkCall(n)
	case *ast.BuiltinNode:
		for _, arg := range n.Arguments {
			e.walk(arg)
		}
	case *ast.PredicateNode:
		e.walk(n.Node)
	case *ast.VariableDeclaratorNode:
		// The value is evaluated before the binding exists, so an outer
		// variable of the same name is still visible there.
		e.walk(n.Value)
		e.pushScope(n.Name)
		e.walk(n.Expr)
		e.popScope()
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

// walkCall records the callee of a call and then walks the arguments.
// Builtin functions are not dependencies, though their arguments still are.
// A call of a non-builtin identifier is one dependency named after the callee.
// A method call counts its receiver as a dependency and records the method
// name as a call marker instead of a variable path.
func (e *dependencyExaminer) walkCall(n *ast.CallNode) {
	for _, arg := range n.Arguments {
		e.walk(arg)
	}

	callee := n.Callee
	for {
		chain, isChain := callee.(*ast.ChainNode)
		if !isChain {
			break
		}
		callee = chain.Node
	}

	if member, isMember := callee.(*ast.MemberNode); isMember {
		e.walkRef(member.Node)
		if property, isField := member.Property.(*ast.StringNode); isField && isFieldAccess(member) {
			e.addCall(property.Value)
		} else {
			e.walk(member.Property)
		}
		return
	}

	if ident, isIdent := callee.(*ast.IdentifierNode); isIdent {
		if _, isBuiltin := builtin.Index[ident.Value]; isBuiltin {
			return
		}
		if !e.isLocal(ident.Value) {
			e.add(ident.Value, false)
		}
		return
	}

	e.walk(callee)
}

// refOf resolves a chain of identifiers and member accesses into a path.
// It returns ok=false for expressions that are not plain references, so the
// caller can fall back to a structural walk of their subexpressions.
func (e *dependencyExaminer) refOf(node ast.Node) ref {
	switch n := node.(type) {
	case *ast.IdentifierNode:
		return ref{path: n.Value, local: e.isLocal(n.Value), ok: true}
	case *ast.ChainNode:
		return e.refOf(n.Node)
	case *ast.MemberNode:
		parent := e.refOf(n.Node)
		if !parent.ok {
			return ref{}
		}
		if property, isField := n.Property.(*ast.StringNode); isField && isFieldAccess(n) {
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
	switch n := node.(type) {
	case *ast.ChainNode:
		e.walkRef(n.Node)
	case *ast.MemberNode:
		e.walk(n.Node)
		e.walk(n.Property)
	case *ast.SliceNode:
		e.walk(n.Node)
		e.walk(n.From)
		e.walk(n.To)
	default:
		e.walk(node)
	}
}

// walkRefIndices walks the subexpressions embedded in a reference chain, such
// as the index of "a[i][j]" or the bounds of "a[1:2]". The identifier and
// field segments themselves are already accounted for by the resolved path.
func (e *dependencyExaminer) walkRefIndices(node ast.Node) {
	switch n := node.(type) {
	case *ast.ChainNode:
		e.walkRefIndices(n.Node)
	case *ast.MemberNode:
		if _, isField := n.Property.(*ast.StringNode); !isField || !isFieldAccess(n) {
			e.walk(n.Property)
		}
		e.walkRefIndices(n.Node)
	case *ast.SliceNode:
		e.walk(n.From)
		e.walk(n.To)
		e.walkRefIndices(n.Node)
	}
}

// isFieldAccess reports whether a member node was written with dot syntax,
// as in "a.b", rather than bracket syntax, as in "a[b]" or "a["b"]".
// The parser points a dot member node at its property token and a bracket
// member node at the opening bracket, so equal offsets mean dot syntax.
func isFieldAccess(n *ast.MemberNode) bool {
	return n.Location().From == n.Property.Location().From
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

func (e *dependencyExaminer) addCall(name string) {
	if e.calls == nil {
		e.calls = map[string]*MethodCall{}
	}
	call, ok := e.calls[name]
	if !ok {
		call = &MethodCall{Name: name}
		e.calls[name] = call
	}
	call.Count++
}

func (e *dependencyExaminer) result() *Dependencies {
	deps := &Dependencies{
		Vars:  []Dependency{},
		Calls: []MethodCall{},
	}
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
