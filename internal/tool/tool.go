package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/zkupu/pantheon/internal/gateway"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() Schema
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

func (r *Registry) Definitions(strict bool) []gateway.ToolDefinition {
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
				Strict:      strict,
			},
		})
	}
	slices.SortFunc(defs, func(a, b gateway.ToolDefinition) int {
		if a.Function.Name < b.Function.Name {
			return -1
		}
		if a.Function.Name > b.Function.Name {
			return 1
		}
		return 0
	})
	return defs
}

func (r *Registry) Execute(ctx context.Context, call gateway.ToolCall) gateway.Message {
	if call.Function.Name == "" {
		return gateway.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    "error: tool call missing function name",
		}
	}
	t, ok := r.Get(call.Function.Name)
	if !ok {
		return gateway.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("error: unknown tool %q", call.Function.Name),
		}
	}
	if err := validateRequired(t.Parameters(), call.Function.Arguments); err != nil {
		return gateway.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("error: invalid arguments for %q: %v", call.Function.Name, err),
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
	params     Schema
	fn         func(ctx context.Context, argsJSON string) (string, error)
}

func NewFunc(name, desc string, params Schema, fn func(context.Context, string) (string, error)) *Func {
	return &Func{name: name, desc: desc, params: params, fn: fn}
}

func (f *Func) Name() string                                             { return f.name }
func (f *Func) Description() string                                      { return f.desc }
func (f *Func) Parameters() Schema                                       { return f.params }
func (f *Func) Execute(ctx context.Context, args string) (string, error) { return f.fn(ctx, args) }

type Schema struct {
	Type                 string            `json:"type"`
	Desc                 string            `json:"description,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	Required             []string          `json:"required,omitempty"`
	AdditionalProperties *bool             `json:"additionalProperties,omitempty"`
}

func boolPtr(b bool) *bool { return &b }

// validateRequired checks that all required fields in the schema are present in the JSON args.
func validateRequired(schema Schema, argsJSON string) error {
	if len(schema.Required) == 0 {
		return nil
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &parsed); err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	for _, field := range schema.Required {
		val, exists := parsed[field]
		if !exists || string(val) == "null" {
			return fmt.Errorf("missing required field %q", field)
		}
	}
	return nil
}

// StrictSchema creates a Schema with additionalProperties set to false,
// conforming to OpenAI/Anthropic strict mode requirements.
func StrictSchema(properties map[string]Schema, required []string) Schema {
	return Schema{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: boolPtr(false),
	}
}

func ParseArgs[T any](argsJSON string) (T, error) {
	var v T
	if err := json.Unmarshal([]byte(argsJSON), &v); err != nil {
		return v, fmt.Errorf("parse args: %w", err)
	}
	return v, nil
}
