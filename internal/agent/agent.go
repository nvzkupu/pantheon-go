package agent

import (
	"context"
	"fmt"

	"github.com/zkupu/pantheon/internal/gateway"
	"github.com/zkupu/pantheon/internal/skill"
	"github.com/zkupu/pantheon/internal/tool"
)

type EventKind string

const (
	EventToolCall   EventKind = "tool_call"
	EventToolResult EventKind = "tool_result"
	EventReply      EventKind = "reply"
	EventError      EventKind = "error"
)

type Event struct {
	Kind    EventKind
	Agent   string
	Tool    string
	Content string
	Usage   gateway.Usage
}

type Agent struct {
	Skill   *skill.Skill
	history []gateway.Message
	client  *gateway.Client
	tools   *tool.Registry
	OnEvent func(Event)
}

func New(s *skill.Skill, client *gateway.Client) *Agent {
	a := &Agent{
		Skill:  s,
		client: client,
		tools:  tool.NewRegistry(),
	}
	a.Reset()
	return a
}

func (a *Agent) Name() string        { return a.Skill.Name }
func (a *Agent) Persona() string     { return a.Skill.Metadata.Persona }
func (a *Agent) Model() string       { return a.Skill.Metadata.Model }
func (a *Agent) UseFor() string      { return a.Skill.Description }
func (a *Agent) ToolNames() []string { return a.Skill.Metadata.Tools }
func (a *Agent) Delegates() []string { return a.Skill.Metadata.Delegates }
func (a *Agent) Tools() *tool.Registry { return a.tools }
func (a *Agent) History() []gateway.Message { return a.history }

func (a *Agent) MaxIterations() int {
	if n := a.Skill.Metadata.MaxIterations; n > 0 {
		return n
	}
	return 10
}

func (a *Agent) Temperature() float64 {
	if t := a.Skill.Metadata.Temperature; t > 0 {
		return t
	}
	return 0.7
}

func (a *Agent) MaxTokens() int {
	if n := a.Skill.Metadata.MaxTokens; n > 0 {
		return n
	}
	return 4096
}

func (a *Agent) emit(e Event) {
	e.Agent = a.Name()
	if a.OnEvent != nil {
		a.OnEvent(e)
	}
}

func (a *Agent) chatReq() gateway.ChatRequest {
	return gateway.ChatRequest{
		Model:       a.Model(),
		Messages:    a.history,
		Temperature: a.Temperature(),
		MaxTokens:   a.MaxTokens(),
	}
}

// Send does a simple non-streaming, non-tool request.
func (a *Agent) Send(msg string) (string, error) {
	a.history = append(a.history, gateway.Message{Role: "user", Content: msg})
	resp, err := a.client.Chat(a.chatReq())
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	reply := resp.Choices[0].Message.Content
	a.history = append(a.history, gateway.Message{Role: "assistant", Content: reply})
	return reply, nil
}

// SendStream sends a message and streams the response token by token.
func (a *Agent) SendStream(msg string, onChunk func(string)) (string, error) {
	a.history = append(a.history, gateway.Message{Role: "user", Content: msg})
	full, err := a.client.ChatStream(a.chatReq(), onChunk)
	if err != nil {
		return "", err
	}
	a.history = append(a.history, gateway.Message{Role: "assistant", Content: full})
	return full, nil
}

// Run executes the ReAct loop: reason → call tools → observe → repeat until
// the model produces a final text response or hits max iterations.
func (a *Agent) Run(ctx context.Context, msg string) (string, error) {
	a.history = append(a.history, gateway.Message{Role: "user", Content: msg})

	toolDefs := a.tools.Definitions()
	hasTools := len(toolDefs) > 0
	var totalUsage gateway.Usage

	for i := 0; i < a.MaxIterations(); i++ {
		req := a.chatReq()
		if hasTools {
			req.Tools = toolDefs
		}

		resp, err := a.client.ChatWithTools(req)
		if err != nil {
			a.emit(Event{Kind: EventError, Content: err.Error()})
			return "", fmt.Errorf("iteration %d: %w", i, err)
		}
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no choices at iteration %d", i)
		}

		choice := resp.Choices[0].Message

		if len(choice.ToolCalls) > 0 {
			a.history = append(a.history, choice)
			for _, tc := range choice.ToolCalls {
				a.emit(Event{Kind: EventToolCall, Tool: tc.Function.Name, Content: tc.Function.Arguments})
			}
			results := a.tools.ExecuteAll(ctx, choice.ToolCalls)
			for _, r := range results {
				a.emit(Event{Kind: EventToolResult, Content: r.Content})
				a.history = append(a.history, r)
			}
			continue
		}

		reply := choice.Content
		a.history = append(a.history, gateway.Message{Role: "assistant", Content: reply})
		a.emit(Event{Kind: EventReply, Content: reply, Usage: totalUsage})
		return reply, nil
	}

	return "", fmt.Errorf("agent %q hit max iterations (%d)", a.Name(), a.MaxIterations())
}

func (a *Agent) Reset() {
	a.history = []gateway.Message{{Role: "system", Content: a.Skill.Body}}
}

// LoadAll discovers skills and returns a map of ready-to-use agents.
func LoadAll(dir string, client *gateway.Client) (map[string]*Agent, error) {
	skills, err := skill.DiscoverMap(dir)
	if err != nil {
		return nil, err
	}
	agents := make(map[string]*Agent, len(skills))
	for name, s := range skills {
		agents[name] = New(s, client)
	}
	return agents, nil
}

// EquipTools attaches built-in tools that match the agent's skill config.
func EquipTools(a *Agent, builtins *tool.Registry) {
	for _, name := range a.ToolNames() {
		if t, ok := builtins.Get(name); ok {
			a.tools.Register(t)
		}
	}
}
