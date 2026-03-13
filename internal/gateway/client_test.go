package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mockServer(handler http.HandlerFunc) (*httptest.Server, *Client) {
	srv := httptest.NewServer(handler)
	client := NewClient(srv.URL, "test-key")
	return srv, client
}

func TestChatSuccess(t *testing.T) {
	srv, client := mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		resp := ChatResponse{
			ID: "resp-1",
			Choices: []ChatChoice{{
				Message:      Message{Role: "assistant", Content: "hello back"},
				FinishReason: "stop",
			}},
			Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	resp, err := client.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "hello back" {
		t.Errorf("expected 'hello back', got %q", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestChatWithToolCalls(t *testing.T) {
	srv, client := mockServer(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatResponse{
			Choices: []ChatChoice{{
				Message: Message{
					Role: "assistant",
					ToolCalls: []ToolCall{{
						ID:       "tc-1",
						Type:     "function",
						Function: FunctionCall{Name: "get_weather", Arguments: `{"location":"NYC"}`},
					}},
				},
				FinishReason: "tool_calls",
			}},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	resp, err := client.ChatWithTools(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "weather?"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: FunctionSchema{
				Name: "get_weather", Description: "Get weather",
				Parameters: map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatal("expected 1 tool call")
	}
	if resp.Choices[0].Message.ToolCalls[0].Function.Name != "get_weather" {
		t.Error("wrong tool name")
	}
}

func TestChatRetryOn429(t *testing.T) {
	attempt := 0
	srv, client := mockServer(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			fmt.Fprint(w, "rate limited")
			return
		}
		resp := ChatResponse{
			Choices: []ChatChoice{{
				Message: Message{Role: "assistant", Content: "finally"},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	resp, err := client.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if resp.Choices[0].Message.Content != "finally" {
		t.Errorf("unexpected content: %q", resp.Choices[0].Message.Content)
	}
	if attempt != 3 {
		t.Errorf("expected 3 attempts, got %d", attempt)
	}
}

func TestChatNonRetryableError(t *testing.T) {
	srv, client := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, "bad request")
	})
	defer srv.Close()

	_, err := client.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected 400 in error, got: %v", err)
	}
}

func TestChatStreamContent(t *testing.T) {
	srv, client := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{"Hel", "lo ", "world"}
		for i, c := range chunks {
			chunk := ChatResponse{
				Choices: []ChatChoice{{Delta: Message{Content: c}}},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			_ = i
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	defer srv.Close()

	var received []string
	full, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk string) {
		received = append(received, chunk)
	})
	if err != nil {
		t.Fatal(err)
	}
	if full != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", full)
	}
	if len(received) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(received))
	}
}

func TestMessageSerialization(t *testing.T) {
	msg := Message{
		Role:       "assistant",
		Content:    "text",
		ToolCalls:  []ToolCall{{ID: "tc-1", Type: "function", Function: FunctionCall{Name: "test", Arguments: "{}"}}},
		ToolCallID: "tc-1",
		Name:       "test",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ToolCallID != "tc-1" {
		t.Errorf("tool_call_id not preserved: %q", decoded.ToolCallID)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatal("tool_calls not preserved")
	}
}
