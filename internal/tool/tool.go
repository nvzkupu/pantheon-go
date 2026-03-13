package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/zkupu/pantheon/internal/gateway"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() interface{}
	Execute(ctx context.Context, argsJSON string) (string, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Definitions() []gateway.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]gateway.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, gateway.ToolDefinition{
			Type: "function",
			Function: gateway.FunctionSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

func (r *Registry) Execute(ctx context.Context, call gateway.ToolCall) gateway.Message {
	t, ok := r.Get(call.Function.Name)
	if !ok {
		return gateway.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("error: unknown tool %q", call.Function.Name),
		}
	}
	result, err := t.Execute(ctx, call.Function.Arguments)
	if err != nil {
		return gateway.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("error: %v", err),
		}
	}
	return gateway.Message{Role: "tool", ToolCallID: call.ID, Content: result}
}

func (r *Registry) ExecuteAll(ctx context.Context, calls []gateway.ToolCall) []gateway.Message {
	results := make([]gateway.Message, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c gateway.ToolCall) {
			defer wg.Done()
			results[idx] = r.Execute(ctx, c)
		}(i, call)
	}
	wg.Wait()
	return results
}

type Func struct {
	name, desc string
	params     interface{}
	fn         func(ctx context.Context, argsJSON string) (string, error)
}

func NewFunc(name, desc string, params interface{}, fn func(context.Context, string) (string, error)) *Func {
	return &Func{name: name, desc: desc, params: params, fn: fn}
}

func (f *Func) Name() string                                              { return f.name }
func (f *Func) Description() string                                       { return f.desc }
func (f *Func) Parameters() interface{}                                   { return f.params }
func (f *Func) Execute(ctx context.Context, args string) (string, error)  { return f.fn(ctx, args) }

type Schema struct {
	Type       string            `json:"type"`
	Desc       string            `json:"description,omitempty"`
	Properties map[string]Schema `json:"properties,omitempty"`
	Required   []string          `json:"required,omitempty"`
}

func ParseArgs[T any](argsJSON string) (T, error) {
	var v T
	if err := json.Unmarshal([]byte(argsJSON), &v); err != nil {
		return v, fmt.Errorf("parse args: %w", err)
	}
	return v, nil
}
