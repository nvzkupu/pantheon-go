package orchestrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/zkupu/pantheon/internal/agent"
	"github.com/zkupu/pantheon/internal/gateway"
	"github.com/zkupu/pantheon/internal/tool"
)

// AgentTool wraps an agent as a tool so coordinators can delegate work.
type AgentTool struct {
	agent *agent.Agent
}

func NewAgentTool(a *agent.Agent) *AgentTool { return &AgentTool{agent: a} }

func (at *AgentTool) Name() string { return "ask_" + at.agent.Name() }

func (at *AgentTool) Description() string {
	return fmt.Sprintf("Delegate to %s (%s). Specializes in: %s",
		at.agent.Name(), at.agent.Persona(), at.agent.UseFor())
}

func (at *AgentTool) Parameters() interface{} {
	return tool.Schema{
		Type: "object",
		Properties: map[string]tool.Schema{
			"task": {Type: "string", Desc: "Task or question for this specialist"},
		},
		Required: []string{"task"},
	}
}

func (at *AgentTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	args, err := tool.ParseArgs[struct{ Task string `json:"task"` }](argsJSON)
	if err != nil {
		return "", err
	}
	at.agent.Reset()
	reply, err := at.agent.Run(ctx, args.Task)
	if err != nil {
		return fmt.Sprintf("specialist %s error: %v", at.agent.Name(), err), nil
	}
	return reply, nil
}

// Team coordinates a lead agent with specialist delegates using agent-as-tool.
type Team struct {
	Lead        *agent.Agent
	Specialists map[string]*agent.Agent
	OnEvent     func(agent.Event)
}

func NewTeam(lead *agent.Agent, specialists []*agent.Agent) *Team {
	m := make(map[string]*agent.Agent, len(specialists))
	for _, s := range specialists {
		m[s.Name()] = s
	}
	return &Team{Lead: lead, Specialists: m}
}

func (t *Team) Setup() {
	reg := t.Lead.Tools()
	for _, spec := range t.Specialists {
		reg.Register(NewAgentTool(spec))
	}

	var roster strings.Builder
	for _, spec := range t.Specialists {
		fmt.Fprintf(&roster, "- ask_%s: %s (%s) — %s\n",
			spec.Name(), spec.Name(), spec.Persona(), spec.UseFor())
	}

	prompt := fmt.Sprintf(`

You coordinate a specialist team. Available specialists:

%s
RULES:
1. Break complex tasks into subtasks and delegate to the best specialist.
2. Synthesize specialist responses into a coherent answer.
3. Attribute insights to the specialist who provided them.
4. For simple questions you can answer directly.`, roster.String())

	t.Lead.Skill.Body += prompt
	t.Lead.Reset()

	if t.OnEvent != nil {
		t.Lead.OnEvent = t.OnEvent
		for _, s := range t.Specialists {
			s.OnEvent = t.OnEvent
		}
	}
}

func (t *Team) Run(ctx context.Context, msg string) (string, error) {
	return t.Lead.Run(ctx, msg)
}

// Pipeline chains agents sequentially: output of each feeds into the next.
type Pipeline struct {
	Name   string
	Stages []*agent.Agent
}

func NewPipeline(name string, stages ...*agent.Agent) *Pipeline {
	return &Pipeline{Name: name, Stages: stages}
}

func (p *Pipeline) Run(ctx context.Context, input string) (string, error) {
	current := input
	for i, a := range p.Stages {
		a.Reset()
		result, err := a.Run(ctx, current)
		if err != nil {
			return "", fmt.Errorf("pipeline %q stage %d (%s): %w", p.Name, i, a.Name(), err)
		}
		current = result
	}
	return current, nil
}

// Review fans out to multiple reviewers in parallel, then a synthesizer
// combines their feedback.
type Review struct {
	Reviewers   []*agent.Agent
	Synthesizer *agent.Agent
}

func NewReview(synthesizer *agent.Agent, reviewers ...*agent.Agent) *Review {
	return &Review{Reviewers: reviewers, Synthesizer: synthesizer}
}

func (r *Review) Run(ctx context.Context, input string) (string, error) {
	type result struct {
		name, persona, output string
		err                   error
	}

	results := make([]result, len(r.Reviewers))
	done := make(chan int, len(r.Reviewers))

	for i, a := range r.Reviewers {
		go func(idx int, ag *agent.Agent) {
			ag.Reset()
			out, err := ag.Run(ctx, input)
			results[idx] = result{ag.Name(), ag.Persona(), out, err}
			done <- idx
		}(i, a)
	}
	for range r.Reviewers {
		<-done
	}

	var reviews strings.Builder
	for _, res := range results {
		fmt.Fprintf(&reviews, "=== %s (%s) ===\n", res.name, res.persona)
		if res.err != nil {
			fmt.Fprintf(&reviews, "ERROR: %v\n", res.err)
		} else {
			reviews.WriteString(res.output)
		}
		reviews.WriteString("\n\n")
	}

	prompt := fmt.Sprintf(`Synthesize these specialist reviews into a single, actionable response.

Original request:
%s

Reviews:
%s

Provide a unified response with the most important points from each reviewer.`, input, reviews.String())

	r.Synthesizer.Reset()
	return r.Synthesizer.Run(ctx, prompt)
}

// CostEstimate returns a rough USD cost for a given model and token usage.
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
