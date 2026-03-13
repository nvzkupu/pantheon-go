package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	Role       string     `json:"role" yaml:"role"`
	Content    string     `json:"content" yaml:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty" yaml:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty" yaml:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty" yaml:"name,omitempty"`
}

type ToolCall struct {
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

type FunctionSchema struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
	Strict      bool   `json:"strict,omitempty"`
}

type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  any              `json:"tool_choice,omitempty"`
}

type ChatChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Delta        Message `json:"delta"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	StrictTools bool
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	return c.doChat(ctx, req)
}

func (c *Client) ChatWithTools(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	return c.doChat(ctx, req)
}

// StreamResult holds the accumulated result of a streaming response,
// including both text content and any tool calls.
type StreamResult struct {
	Content   string
	ToolCalls []ToolCall
}

// ChatStreamFull streams a response, accumulating both content deltas and tool call deltas.
// The onChunk callback is invoked for each content delta (may be nil).
func (c *Client) ChatStreamFull(ctx context.Context, req ChatRequest, onChunk func(string)) (*StreamResult, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gateway request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway %d: %s", resp.StatusCode, string(respBody))
	}

	var content strings.Builder
	toolCalls := make(map[int]*ToolCall)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk ChatResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if onChunk != nil {
				onChunk(delta.Content)
			}
		}
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if existing, ok := toolCalls[idx]; ok {
				existing.Function.Arguments += tc.Function.Arguments
			} else {
				call := ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
				toolCalls[idx] = &call
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var calls []ToolCall
	for _, tc := range toolCalls {
		calls = append(calls, *tc)
	}

	return &StreamResult{Content: content.String(), ToolCalls: calls}, nil
}

func (c *Client) ChatStream(ctx context.Context, req ChatRequest, onChunk func(string)) (string, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gateway request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gateway %d: %s", resp.StatusCode, string(respBody))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk ChatResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			full.WriteString(content)
			if onChunk != nil {
				onChunk(content)
			}
		}
	}
	return full.String(), scanner.Err()
}

const (
	maxRetries     = 3
	baseRetryDelay = 1 * time.Second
	maxRetryDelay  = 30 * time.Second
)

func isRetryable(statusCode int) bool {
	return statusCode == 429 || statusCode == 500 || statusCode == 502 ||
		statusCode == 503 || statusCode == 504
}

func retryDelay(attempt int, resp *http.Response) time.Duration {
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				return time.Duration(secs) * time.Second
			}
		}
	}
	delay := time.Duration(float64(baseRetryDelay) * math.Pow(2, float64(attempt)))
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	jitter := time.Duration(rand.Int64N(int64(delay) / 2))
	return delay + jitter
}

func (c *Client) doChat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := range maxRetries {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		c.setHeaders(httpReq)

		resp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("gateway request: %w", err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay(attempt, nil)):
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var chatResp ChatResponse
			if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
				return nil, fmt.Errorf("decode response: %w", err)
			}
			return &chatResp, nil
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("gateway %d: %s", resp.StatusCode, string(respBody))

		if !isRetryable(resp.StatusCode) || attempt == maxRetries-1 {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay(attempt, resp)):
		}
	}
	return nil, lastErr
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
}
