package observe

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zkupu/pantheon/internal/agent"
	"github.com/zkupu/pantheon/internal/gateway"
)

// Trace captures the full lifecycle of an agent invocation.
type Trace struct {
	ID         string        `json:"id"`
	Agent      string        `json:"agent"`
	StartedAt  time.Time     `json:"started_at"`
	Spans      []Span        `json:"spans"`
	TotalUsage gateway.Usage `json:"total_usage"`
	DurationMS float64       `json:"duration_ms"`
}

type Span struct {
	Kind      string    `json:"kind"`
	Agent     string    `json:"agent"`
	Tool      string    `json:"tool,omitempty"`
	Content   string    `json:"content,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Tracker collects traces across multiple agent invocations.
type Tracker struct {
	mu     sync.Mutex
	traces map[string]*Trace
}

func NewTracker() *Tracker {
	return &Tracker{traces: make(map[string]*Trace)}
}

func (t *Tracker) Start(traceID, agentName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.traces[traceID] = &Trace{
		ID: traceID, Agent: agentName, StartedAt: time.Now(),
	}
}

func (t *Tracker) AddSpan(traceID string, span Span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if trace, ok := t.traces[traceID]; ok {
		span.Timestamp = time.Now()
		trace.Spans = append(trace.Spans, span)
	}
}

func (t *Tracker) Finish(traceID string, usage gateway.Usage) *Trace {
	t.mu.Lock()
	defer t.mu.Unlock()
	trace, ok := t.traces[traceID]
	if !ok {
		return nil
	}
	trace.TotalUsage = usage
	trace.DurationMS = float64(time.Since(trace.StartedAt).Milliseconds())
	return trace
}

func (t *Tracker) EventHandler(traceID string) func(agent.Event) {
	return func(e agent.Event) {
		t.AddSpan(traceID, Span{
			Kind: string(e.Kind), Agent: e.Agent, Tool: e.Tool,
			Content: truncate(e.Content, 500),
		})
	}
}

// Logger writes structured event output to stderr.
type Logger struct {
	Verbose bool
}

func NewLogger(verbose bool) *Logger {
	return &Logger{Verbose: verbose}
}

func (l *Logger) Handler() func(agent.Event) {
	return func(e agent.Event) {
		switch e.Kind {
		case agent.EventToolCall:
			fmt.Fprintf(os.Stderr, "  [tool] %s(%s)\n", e.Tool, truncate(e.Content, 120))
		case agent.EventToolResult:
			if l.Verbose {
				fmt.Fprintf(os.Stderr, "  [result] %s\n", truncate(e.Content, 200))
			}
		case agent.EventError:
			fmt.Fprintf(os.Stderr, "  [error] %s\n", e.Content)
		case agent.EventReply:
			if e.Usage.TotalTokens > 0 {
				fmt.Fprintf(os.Stderr, "  [tokens] prompt=%d completion=%d total=%d\n",
					e.Usage.PromptTokens, e.Usage.CompletionTokens, e.Usage.TotalTokens)
			}
		}
	}
}

// CombineHandlers merges multiple event handlers into one.
func CombineHandlers(handlers ...func(agent.Event)) func(agent.Event) {
	return func(e agent.Event) {
		for _, h := range handlers {
			if h != nil {
				h(e)
			}
		}
	}
}

// CostEstimate returns a rough USD cost for a model and token usage.
func CostEstimate(model string, usage gateway.Usage) float64 {
	var inRate, outRate float64
	switch {
	case strings.Contains(model, "opus"):
		inRate, outRate = 15.0, 75.0
	case strings.Contains(model, "codex"):
		inRate, outRate = 6.0, 24.0
	case strings.Contains(model, "nano"):
		inRate, outRate = 0.10, 0.40
	default:
		inRate, outRate = 3.0, 15.0
	}
	return (float64(usage.PromptTokens)*inRate + float64(usage.CompletionTokens)*outRate) / 1_000_000
}

// PrintTrace dumps a trace as formatted JSON to stderr.
func PrintTrace(trace *Trace) {
	data, _ := json.MarshalIndent(trace, "", "  ")
	fmt.Fprintln(os.Stderr, string(data))
}

func truncate(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "..."
}
