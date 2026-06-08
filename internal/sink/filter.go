package sink

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// compileEnv is a representative environment used at compile time so expr can
// type-check expressions. All values are typed as the zero value of their
// runtime type.
var compileEnv = map[string]any{
	"event_type": "",
	"event":      map[string]any{},
	"claims":     map[string]any{},
}

// compiledFilter pairs a compiled program with its original source string so
// FailedExpression can be reported. referencedPaths holds the maximal constant
// member-access paths rooted at "event" or "claims" found in the expression.
type compiledFilter struct {
	program         *vm.Program
	source          string
	referencedPaths []string
}

// Filters is a compiled set of boolean filter expressions for one sink.
type Filters struct {
	filters []compiledFilter
}

// CompileFilters compiles each expression with a boolean result type.
// An empty or nil slice yields an empty *Filters whose Evaluate always forwards.
// On a bad expression it returns an error identifying which one.
func CompileFilters(expressions []string) (*Filters, error) {
	filters := make([]compiledFilter, 0, len(expressions))
	for i, src := range expressions {
		program, err := expr.Compile(src, expr.Env(compileEnv), expr.AsBool())
		if err != nil {
			return nil, fmt.Errorf("filters[%d]: %w", i, err)
		}
		filters = append(filters, compiledFilter{
			program:         program,
			source:          src,
			referencedPaths: extractFieldPaths(src),
		})
	}
	return &Filters{filters: filters}, nil
}

// Result is the outcome of evaluating a Filters set against one SET's claims.
type Result struct {
	Forward          bool     // true => forward this SET to the sink
	FailedExpression string   // source of the first filter that returned false (empty when Forward or Err != nil)
	MultiEvent       bool     // the SET's events map had != 1 entry, so event/event_type were unavailable
	MissingPaths     []string // referenced field paths absent from the SET; event-rooted paths are suppressed when MultiEvent
	Err              error    // a runtime evaluation error; when set, Forward is false (fail closed)
}

// Evaluate builds the {event_type, event, claims} environment from the decoded
// claims and evaluates every filter with short-circuit AND. Empty Filters => Forward true.
func (f *Filters) Evaluate(claims map[string]any) Result {
	if len(f.filters) == 0 {
		return Result{Forward: true}
	}

	env, multiEvent := buildEnv(claims)

	var missingPaths []string
	seen := map[string]bool{}

	addMissing := func(cf compiledFilter) {
		for _, path := range cf.referencedPaths {
			// When multi-event, event-rooted paths are inherently unavailable — suppress them.
			if multiEvent && strings.HasPrefix(path, "event.") {
				continue
			}
			if seen[path] {
				continue
			}
			if !pathPresentInEnv(path, env) {
				seen[path] = true
				missingPaths = append(missingPaths, path)
			}
		}
	}

	for _, cf := range f.filters {
		out, err := expr.Run(cf.program, env)
		addMissing(cf)
		if err != nil {
			return Result{
				Forward:          false,
				FailedExpression: cf.source,
				MissingPaths:     missingPaths,
				Err:              err,
			}
		}
		if !out.(bool) {
			return Result{
				Forward:          false,
				FailedExpression: cf.source,
				MultiEvent:       multiEvent,
				MissingPaths:     missingPaths,
			}
		}
	}

	return Result{Forward: true, MultiEvent: multiEvent, MissingPaths: missingPaths}
}

// extractFieldPaths returns the maximal constant member-access paths in src
// that are rooted at the identifier "event" or "claims". Paths are dot-joined
// segment strings (e.g. "event.current_level", "claims.sub_id.sub"). Only
// paths whose every key step is a constant string are included; dynamic index
// access is skipped. Shorter paths that are strict prefixes of longer ones are
// dropped (only maximal paths are kept). The result is deduped.
func extractFieldPaths(src string) []string {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil
	}

	var all []string
	seen := map[string]bool{}

	visitor := &pathVisitor{collect: func(path string) {
		if !seen[path] {
			seen[path] = true
			all = append(all, path)
		}
	}}
	ast.Walk(&tree.Node, visitor)

	// Drop paths that are a strict prefix of another collected path.
	result := all[:0:len(all)]
	for _, p := range all {
		dominated := false
		for _, other := range all {
			if other != p && strings.HasPrefix(other, p+".") {
				dominated = true
				break
			}
		}
		if !dominated {
			result = append(result, p)
		}
	}
	return result
}

// pathVisitor walks an expr AST and calls collect for each maximal constant
// member-access chain rooted at "event" or "claims".
type pathVisitor struct {
	collect func(string)
}

func (v *pathVisitor) Visit(node *ast.Node) {
	mn, ok := (*node).(*ast.MemberNode)
	if !ok {
		return
	}
	// Only handle constant string properties (not computed access).
	prop, ok := mn.Property.(*ast.StringNode)
	if !ok {
		return
	}

	path, rooted := memberPath(mn.Node, prop.Value)
	if rooted {
		v.collect(path)
	}
}

// memberPath recursively builds a dot-joined path from a MemberNode chain.
// It returns (path, true) if the chain bottoms out at an IdentifierNode whose
// value is "event" or "claims". The prop argument is the property being
// appended at this level.
func memberPath(node ast.Node, prop string) (string, bool) {
	switch n := node.(type) {
	case *ast.IdentifierNode:
		if n.Value == "event" || n.Value == "claims" {
			return n.Value + "." + prop, true
		}
		return "", false
	case *ast.MemberNode:
		parentProp, ok := n.Property.(*ast.StringNode)
		if !ok {
			return "", false
		}
		prefix, rooted := memberPath(n.Node, parentProp.Value)
		if !rooted {
			return "", false
		}
		return prefix + "." + prop, true
	default:
		return "", false
	}
}

// pathPresentInEnv reports whether the dot-separated path (e.g. "event.current_level"
// or "claims.sub_id.sub") resolves to a present key in env. The first segment
// names a key in env; each subsequent segment descends into a map[string]any.
func pathPresentInEnv(path string, env map[string]any) bool {
	segments := strings.SplitN(path, ".", 2)
	root, ok := env[segments[0]]
	if !ok {
		return false
	}
	if len(segments) == 1 {
		return true
	}
	return pathPresentInMap(segments[1], root)
}

// pathPresentInMap descends the remaining dot-joined path through nested maps.
func pathPresentInMap(path string, val any) bool {
	m, ok := val.(map[string]any)
	if !ok {
		return false
	}
	parts := strings.SplitN(path, ".", 2)
	child, exists := m[parts[0]]
	if !exists {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	return pathPresentInMap(parts[1], child)
}

// buildEnv constructs the runtime environment map from decoded SET claims.
// It returns the env and whether the events map had more than one entry (MultiEvent).
func buildEnv(claims map[string]any) (map[string]any, bool) {
	var eventType string
	var event map[string]any
	var multiEvent bool

	if events, ok := claims["events"].(map[string]any); ok {
		switch len(events) {
		case 1:
			for k, v := range events {
				eventType = k
				event, _ = v.(map[string]any)
			}
		default:
			if len(events) > 1 {
				multiEvent = true
			}
		}
	}

	return map[string]any{
		"event_type": eventType,
		"event":      event,
		"claims":     claims,
	}, multiEvent
}
