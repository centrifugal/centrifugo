package messagefilter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// Filter represents a compiled CEL filter
type Filter struct {
	program cel.Program
	env     *cel.Env
}

// FilterManager manages CEL filters with caching
type FilterManager struct {
	filters map[string]*Filter
	mu      sync.RWMutex
}

// NewFilterManager creates a new filter manager
func NewFilterManager() *FilterManager {
	return &FilterManager{
		filters: make(map[string]*Filter),
	}
}

// GetOrCreateFilter gets an existing filter or creates a new one
func (fm *FilterManager) GetOrCreateFilter(expression string) (*Filter, error) {
	if expression == "" {
		return nil, nil
	}

	fm.mu.RLock()
	if filter, exists := fm.filters[expression]; exists {
		fm.mu.RUnlock()
		return filter, nil
	}
	fm.mu.RUnlock()

	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Double-check after acquiring write lock
	if filter, exists := fm.filters[expression]; exists {
		return filter, nil
	}

	filter, err := compileFilter(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to compile CEL expression: %w", err)
	}

	fm.filters[expression] = filter
	return filter, nil
}

// compileFilter compiles a CEL expression into a filter
func compileFilter(expression string) (*Filter, error) {
	// Create CEL environment with meta data type
	env, err := cel.NewEnv(
		cel.Variable("meta", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Parse and check the expression
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("CEL compilation error: %w", issues.Err())
	}

	// Create program
	program, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program: %w", err)
	}

	return &Filter{
		program: program,
		env:     env,
	}, nil
}

// Evaluate evaluates the filter against message metadata
func (f *Filter) Evaluate(ctx context.Context, meta []byte) (bool, error) {
	if f == nil {
		return true, nil // No filter means accept all messages
	}

	// Parse meta data as JSON
	var metaData map[string]interface{}
	if len(meta) > 0 {
		if err := json.Unmarshal(meta, &metaData); err != nil {
			// If meta is not valid JSON, treat it as empty map
			metaData = make(map[string]interface{})
		}
	} else {
		// If no meta data, use empty map
		metaData = make(map[string]interface{})
	}

	// Create activation with meta data
	activation := map[string]interface{}{
		"meta": metaData,
	}

	// Evaluate the expression
	result, _, err := f.program.Eval(activation)
	if err != nil {
		// Log error but don't fail - treat as filter not passing
		return false, nil
	}

	// Convert result to boolean
	if result.Type() != types.BoolType {
		return false, nil
	}

	return bool(result.(types.Bool)), nil
}

// Clear clears all cached filters
func (fm *FilterManager) Clear() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.filters = make(map[string]*Filter)
}

// GetStats returns filter manager statistics
func (fm *FilterManager) GetStats() map[string]interface{} {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	return map[string]interface{}{
		"cached_filters": len(fm.filters),
	}
}
