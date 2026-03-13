package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zkupu/pantheon/internal/gateway"
)

func echoTool() *Func {
	return NewFunc("echo", "Echo args back",
		StrictSchema(map[string]Schema{
			"msg": {Type: "string", Desc: "Message"},
		}, []string{"msg"}),
		func(_ context.Context, argsJSON string) (string, error) {
			args, err := ParseArgs[struct{ Msg string `json:"msg"` }](argsJSON)
			if err != nil {
				return "", err
			}
			return args.Msg, nil
		},
	)
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := echoTool()
	r.Register(tool)

	got, ok := r.Get("echo")
	if !ok {
		t.Fatal("expected tool 'echo' to be registered")
	}
	if got.Name() != "echo" {
		t.Errorf("got name %q, want %q", got.Name(), "echo")
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected tool to not be found")
	}
}

func TestRegistryDefinitions(t *testing.T) {
	r := NewRegistry()
	r.Register(echoTool())
	defs := r.Definitions(true)
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Type != "function" {
		t.Errorf("expected type 'function', got %q", defs[0].Type)
	}
	if !defs[0].Function.Strict {
		t.Error("expected strict: true on definition")
	}
}

func TestRegistryExecuteSuccess(t *testing.T) {
	r := NewRegistry()
	r.Register(echoTool())

	msg := r.Execute(context.Background(), gateway.ToolCall{
		ID:       "call-1",
		Type:     "function",
		Function: gateway.FunctionCall{Name: "echo", Arguments: `{"msg":"hello"}`},
	})

	if msg.Role != "tool" {
		t.Errorf("expected role 'tool', got %q", msg.Role)
	}
	if msg.ToolCallID != "call-1" {
		t.Errorf("expected tool_call_id 'call-1', got %q", msg.ToolCallID)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", msg.Content)
	}
}

func TestRegistryExecuteUnknownTool(t *testing.T) {
	r := NewRegistry()
	msg := r.Execute(context.Background(), gateway.ToolCall{
		ID:       "call-1",
		Function: gateway.FunctionCall{Name: "nope", Arguments: "{}"},
	})
	if !strings.Contains(msg.Content, "unknown tool") {
		t.Errorf("expected unknown tool error, got %q", msg.Content)
	}
}

func TestRegistryExecuteMissingRequired(t *testing.T) {
	r := NewRegistry()
	r.Register(echoTool())
	msg := r.Execute(context.Background(), gateway.ToolCall{
		ID:       "call-1",
		Function: gateway.FunctionCall{Name: "echo", Arguments: `{}`},
	})
	if !strings.Contains(msg.Content, "missing required field") {
		t.Errorf("expected missing field error, got %q", msg.Content)
	}
}

func TestRegistryExecuteAll(t *testing.T) {
	r := NewRegistry()
	r.Register(echoTool())

	calls := []gateway.ToolCall{
		{ID: "c1", Function: gateway.FunctionCall{Name: "echo", Arguments: `{"msg":"a"}`}},
		{ID: "c2", Function: gateway.FunctionCall{Name: "echo", Arguments: `{"msg":"b"}`}},
	}
	results := r.ExecuteAll(context.Background(), calls)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Content != "a" || results[1].Content != "b" {
		t.Errorf("expected [a, b], got [%s, %s]", results[0].Content, results[1].Content)
	}
}

func TestStrictSchema(t *testing.T) {
	s := StrictSchema(map[string]Schema{
		"name": {Type: "string", Desc: "A name"},
	}, []string{"name"})

	if s.Type != "object" {
		t.Errorf("expected type 'object', got %q", s.Type)
	}
	if s.AdditionalProperties == nil || *s.AdditionalProperties != false {
		t.Error("expected additionalProperties to be false")
	}

	data, _ := json.Marshal(s)
	if !strings.Contains(string(data), `"additionalProperties":false`) {
		t.Errorf("JSON should contain additionalProperties:false, got %s", data)
	}
}

func TestValidateRequired(t *testing.T) {
	schema := StrictSchema(map[string]Schema{
		"a": {Type: "string"},
		"b": {Type: "string"},
	}, []string{"a", "b"})

	if err := validateRequired(schema, `{"a":"x","b":"y"}`); err != nil {
		t.Errorf("valid args should pass: %v", err)
	}
	if err := validateRequired(schema, `{"a":"x"}`); err == nil {
		t.Error("missing 'b' should fail")
	}
	if err := validateRequired(schema, `{"a":"x","b":null}`); err == nil {
		t.Error("null 'b' should fail")
	}
	if err := validateRequired(schema, `not json`); err == nil {
		t.Error("malformed JSON should fail")
	}
	emptySchema := Schema{}
	if err := validateRequired(emptySchema, `{}`); err != nil {
		t.Error("empty Schema should pass validation")
	}
}

func TestParseArgs(t *testing.T) {
	type args struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	v, err := ParseArgs[args](`{"name":"test","age":42}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Name != "test" || v.Age != 42 {
		t.Errorf("unexpected result: %+v", v)
	}
}

func TestBuiltinsRegistry(t *testing.T) {
	r := Builtins()
	expected := []string{"shell_exec", "read_file", "write_file", "list_dir", "search_files"}
	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected builtin %q to be registered", name)
		}
	}
}
