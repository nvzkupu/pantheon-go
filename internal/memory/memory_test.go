package memory

import (
	"os"
	"testing"

	"github.com/zkupu/pantheon/internal/gateway"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir, err := os.MkdirTemp("", "pantheon-memory-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	messages := []gateway.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!", ToolCalls: []gateway.ToolCall{
			{ID: "tc-1", Type: "function", Function: gateway.FunctionCall{Name: "test", Arguments: `{"x":1}`}},
		}},
		{Role: "tool", Content: "result", ToolCallID: "tc-1"},
		{Role: "assistant", Content: "Done."},
	}

	if err := store.Save("sess-1", messages); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load("sess-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded) != len(messages) {
		t.Fatalf("expected %d messages, got %d", len(messages), len(loaded))
	}

	for i, m := range loaded {
		if m.Role != messages[i].Role {
			t.Errorf("msg %d: role %q != %q", i, m.Role, messages[i].Role)
		}
		if m.Content != messages[i].Content {
			t.Errorf("msg %d: content %q != %q", i, m.Content, messages[i].Content)
		}
	}

	if len(loaded[2].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call on msg 2, got %d", len(loaded[2].ToolCalls))
	}
	if loaded[2].ToolCalls[0].ID != "tc-1" {
		t.Errorf("tool call ID: got %q, want %q", loaded[2].ToolCalls[0].ID, "tc-1")
	}
	if loaded[3].ToolCallID != "tc-1" {
		t.Errorf("tool_call_id: got %q, want %q", loaded[3].ToolCallID, "tc-1")
	}
}

func TestFileStoreLoadMissing(t *testing.T) {
	dir, err := os.MkdirTemp("", "pantheon-memory-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Load("nonexistent")
	if err == nil {
		t.Error("expected error loading nonexistent session")
	}
}

func TestFileStoreList(t *testing.T) {
	dir, err := os.MkdirTemp("", "pantheon-memory-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	_ = store.Save("a", []gateway.Message{{Role: "user", Content: "hi"}})
	_ = store.Save("b", []gateway.Message{{Role: "user", Content: "hello"}})

	infos, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(infos))
	}
}

func TestWindowTrimmer(t *testing.T) {
	msgs := []gateway.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a3"},
	}

	trimmer := &WindowTrimmer{MaxPairs: 2}
	trimmed := trimmer.Trim(msgs)

	if len(trimmed) != 5 {
		t.Fatalf("expected 5 messages (system + 2 pairs), got %d", len(trimmed))
	}
	if trimmed[0].Role != "system" {
		t.Error("first message should be system")
	}
	if trimmed[1].Content != "u2" {
		t.Errorf("expected u2, got %q", trimmed[1].Content)
	}
}

func TestSessionID(t *testing.T) {
	id1 := SessionID("agent", "label")
	id2 := SessionID("agent", "label")
	id3 := SessionID("agent", "other")

	if id1 != id2 {
		t.Error("same inputs should produce same ID")
	}
	if id1 == id3 {
		t.Error("different inputs should produce different IDs")
	}
}
