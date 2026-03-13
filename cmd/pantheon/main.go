package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/zkupu/pantheon/internal/agent"
	"github.com/zkupu/pantheon/internal/config"
	"github.com/zkupu/pantheon/internal/gateway"
	"github.com/zkupu/pantheon/internal/memory"
	"github.com/zkupu/pantheon/internal/observe"
	"github.com/zkupu/pantheon/internal/orchestrate"
	"github.com/zkupu/pantheon/internal/tool"
)

func main() {
	config.LoadEnvFile()
	dir := config.SkillsDir()
	client := gateway.NewClient(config.GatewayURL(), config.APIKey())
	verbose := config.Verbose()

	if len(os.Args) < 2 {
		cmdWarRoom(dir, client, verbose)
		return
	}

	switch os.Args[1] {
	case "warroom":
		cmdWarRoom(dir, client, verbose)
	case "list":
		cmdList(dir, client)
	case "chat":
		requireArgs(3, "pantheon chat <agent>")
		cmdChat(dir, client, os.Args[2], verbose)
	case "ask":
		requireArgs(4, "pantheon ask <agent> <message...>")
		cmdAsk(dir, client, os.Args[2], strings.Join(os.Args[3:], " "), verbose)
	case "run":
		requireArgs(4, "pantheon run <agent> <task...>")
		cmdRun(dir, client, os.Args[2], strings.Join(os.Args[3:], " "), verbose)
	case "team":
		requireArgs(4, "pantheon team <coordinator> <task...>")
		cmdTeam(dir, client, os.Args[2], strings.Join(os.Args[3:], " "), verbose)
	case "pipe":
		requireArgs(4, "pantheon pipe <a1,a2,...> <input...>")
		cmdPipe(dir, client, os.Args[2], strings.Join(os.Args[3:], " "), verbose)
	case "review":
		requireArgs(4, "pantheon review <r1,r2,...> <input...>")
		cmdReview(dir, client, os.Args[2], strings.Join(os.Args[3:], " "), verbose)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func requireArgs(n int, usage string) {
	if len(os.Args) < n {
		fmt.Fprintf(os.Stderr, "usage: %s\n", usage)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `pantheon — agentic AI toolkit with tools, orchestration, and pipelines

Usage:
  pantheon                                Launch the interactive War Room
  pantheon warroom                        Launch the interactive War Room
  pantheon list                           List all agents
  pantheon chat   <agent>                 Interactive chat (no tools)
  pantheon ask    <agent> <message>       One-shot (no tools)
  pantheon run    <agent> <task>          ReAct loop with tools
  pantheon team   <coordinator> <task>    Coordinator delegates to specialists
  pantheon pipe   <a1,a2,...> <input>     Sequential pipeline
  pantheon review <r1,r2,...> <input>     Parallel review → synthesizer

Environment:
  GATEWAY_URL    OpenAI-compatible endpoint (default: NVIDIA gateway)
  API_KEY        Bearer token
  SKILLS_DIR     Skills directory (default: .agents/skills)
  VERBOSE        Show tool calls and tokens (set to 1)
`)
}

func loadAll(dir string, client *gateway.Client) map[string]*agent.Agent {
	agents, err := agent.LoadAll(dir, client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading agents: %v\n", err)
		os.Exit(1)
	}
	return agents
}

func sortedNames(agents map[string]*agent.Agent) []string {
	names := make([]string, 0, len(agents))
	for n := range agents {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func equip(a *agent.Agent) {
	agent.EquipTools(a, tool.Builtins())
}

func logHandler(verbose bool) func(agent.Event) {
	return observe.NewLogger(verbose).Handler()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- commands ---

func cmdList(dir string, client *gateway.Client) {
	agents := loadAll(dir, client)
	names := sortedNames(agents)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPERSONA\tMODEL\tTOOLS\tDELEGATES\tUSE FOR")
	fmt.Fprintln(w, "----\t-------\t-----\t-----\t---------\t-------")
	for _, n := range names {
		a := agents[n]
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n",
			a.Name(), a.Persona(), a.Model(),
			len(a.ToolNames()), len(a.Delegates()),
			truncate(a.UseFor(), 60))
	}
	w.Flush()
}

func cmdChat(dir string, client *gateway.Client, name string, verbose bool) {
	agents := loadAll(dir, client)
	a, ok := agents[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "agent %q not found\n", name)
		os.Exit(1)
	}

	store, err := memory.NewFileStore(config.MemoryDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open session store: %v\n", err)
	}
	sessionID := memory.SessionID(name, "interactive")

	if store != nil {
		if msgs, loadErr := store.Load(sessionID); loadErr == nil && len(msgs) > 0 {
			a.SetHistory(msgs)
			fmt.Printf("[restored session %s with %d messages]\n", sessionID[:8], len(msgs))
		}
	}

	fmt.Printf("Chatting with %s (%s) — model: %s\n", a.Name(), a.Persona(), a.Model())
	fmt.Println("Commands: /reset  /save  /quit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("you> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/quit" || input == "/exit" {
			break
		}
		if input == "/reset" {
			a.Reset()
			fmt.Println("[history cleared]")
			continue
		}
		if input == "/save" {
			if store != nil {
				_ = store.Save(sessionID, a.History())
				fmt.Printf("[session saved: %s]\n", sessionID[:8])
			}
			continue
		}
		fmt.Printf("\n%s> ", a.Name())
		_, err := a.SendStream(context.Background(), input, func(chunk string) { fmt.Print(chunk) })
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
			continue
		}
		fmt.Print("\n\n")
	}

	if store != nil {
		_ = store.Save(sessionID, a.History())
	}
}

func cmdAsk(dir string, client *gateway.Client, name, msg string, verbose bool) {
	agents := loadAll(dir, client)
	a := mustGet(agents, name)
	_, err := a.SendStream(context.Background(), msg, func(chunk string) { fmt.Print(chunk) })
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
}

func cmdRun(dir string, client *gateway.Client, name, task string, verbose bool) {
	agents := loadAll(dir, client)
	a := mustGet(agents, name)
	equip(a)
	a.OnEvent = logHandler(verbose)

	reply, err := a.Run(context.Background(), task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(reply)
}

func cmdTeam(dir string, client *gateway.Client, coordName, task string, verbose bool) {
	agents := loadAll(dir, client)
	coord := mustGet(agents, coordName)
	equip(coord)

	var specialists []*agent.Agent
	for _, dn := range coord.Delegates() {
		if s, ok := agents[dn]; ok {
			equip(s)
			specialists = append(specialists, s)
		} else {
			fmt.Fprintf(os.Stderr, "warning: delegate %q not found\n", dn)
		}
	}
	if len(specialists) == 0 {
		fmt.Fprintln(os.Stderr, "no delegates configured — use 'run' instead")
		os.Exit(1)
	}

	team := orchestrate.NewTeam(coord, specialists)
	team.OnEvent = logHandler(verbose)
	team.Setup()

	fmt.Fprintf(os.Stderr, "Team: %s coordinating [%s]\n\n",
		coord.Name(), strings.Join(coord.Delegates(), ", "))

	reply, err := team.Run(context.Background(), task)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(reply)
}

func cmdPipe(dir string, client *gateway.Client, agentList, input string, verbose bool) {
	agents := loadAll(dir, client)
	names := strings.Split(agentList, ",")
	var stages []*agent.Agent
	for _, n := range names {
		n = strings.TrimSpace(n)
		a := mustGet(agents, n)
		equip(a)
		if verbose {
			a.OnEvent = logHandler(true)
		}
		stages = append(stages, a)
	}

	p := orchestrate.NewPipeline("cli", stages...)
	fmt.Fprintf(os.Stderr, "Pipeline: %s\n\n", strings.Join(names, " → "))

	result, err := p.Run(context.Background(), input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result)
}

func cmdReview(dir string, client *gateway.Client, agentList, input string, verbose bool) {
	agents := loadAll(dir, client)
	names := strings.Split(agentList, ",")
	if len(names) < 2 {
		fmt.Fprintln(os.Stderr, "review needs at least 2 agents (reviewers + synthesizer)")
		os.Exit(1)
	}

	synthName := strings.TrimSpace(names[len(names)-1])
	synth := mustGet(agents, synthName)
	equip(synth)

	var reviewers []*agent.Agent
	for _, n := range names[:len(names)-1] {
		n = strings.TrimSpace(n)
		a := mustGet(agents, n)
		equip(a)
		if verbose {
			a.OnEvent = logHandler(true)
		}
		reviewers = append(reviewers, a)
	}

	rev := orchestrate.NewReview(synth, reviewers...)
	reviewerNames := make([]string, len(names)-1)
	copy(reviewerNames, names[:len(names)-1])
	fmt.Fprintf(os.Stderr, "Review: [%s] → %s\n\n", strings.Join(reviewerNames, ", "), synthName)

	result, err := rev.Run(context.Background(), input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result)
}

func mustGet(agents map[string]*agent.Agent, name string) *agent.Agent {
	a, ok := agents[name]
	if !ok {
		available := sortedNames(agents)
		fmt.Fprintf(os.Stderr, "agent %q not found — available: %s\n", name, strings.Join(available, ", "))
		os.Exit(1)
	}
	return a
}

// --- War Room ---

func cmdWarRoom(dir string, client *gateway.Client, verbose bool) {
	agents := loadAll(dir, client)
	for _, a := range agents {
		equip(a)
	}
	names := sortedNames(agents)

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║                THE WAR ROOM                     ║")
	fmt.Println("  ║     Your pantheon stands ready for battle.      ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()
	for _, n := range names {
		a := agents[n]
		fmt.Printf("    %-12s  %s\n", a.Name(), a.Persona())
	}
	fmt.Println()
	fmt.Println("  @<name> <msg>   Speak to an agent")
	fmt.Println("  /all <msg>      Broadcast to all")
	fmt.Println("  /list           Show agents")
	fmt.Println("  /quit           Exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for {
		fmt.Print("  you> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if input == "/quit" || input == "/exit" {
			break
		}
		if input == "/list" {
			for _, n := range names {
				a := agents[n]
				fmt.Printf("    %-12s  %-28s  %s\n", a.Name(), a.Persona(), a.Model())
			}
			fmt.Println()
			continue
		}
		if strings.HasPrefix(input, "/all ") {
			msg := strings.TrimSpace(input[5:])
			broadcastAll(agents, names, msg, verbose)
			continue
		}
		if strings.HasPrefix(input, "@") {
			parts := strings.SplitN(input[1:], " ", 2)
			name := strings.ToLower(parts[0])
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				fmt.Printf("  Usage: @%s <message>\n", name)
				continue
			}
			a, ok := agents[name]
			if !ok {
				fmt.Printf("  Unknown agent %q\n", name)
				continue
			}
			if verbose {
				a.OnEvent = logHandler(true)
			}
			fmt.Printf("\n  %s> ", a.Name())
			if len(a.ToolNames()) > 0 {
				reply, err := a.Run(context.Background(), strings.TrimSpace(parts[1]))
				if err != nil {
					fmt.Fprintf(os.Stderr, "\n  [error: %v]\n", err)
				} else {
					fmt.Println(reply)
				}
			} else {
				_, err := a.SendStream(context.Background(), strings.TrimSpace(parts[1]), func(c string) { fmt.Print(c) })
				if err != nil {
					fmt.Fprintf(os.Stderr, "\n  [error: %v]\n", err)
				}
			}
			fmt.Print("\n\n")
			continue
		}

		fmt.Println("  Use @<name> to address an agent, /all to broadcast, /quit to exit.")
	}
	fmt.Println("\n  The war room goes dark.")
}

func broadcastAll(agents map[string]*agent.Agent, names []string, msg string, verbose bool) {
	fmt.Printf("\n  Broadcasting to %d agents...\n", len(names))

	type resp struct {
		name, persona, reply string
		err                  error
		elapsed              time.Duration
	}

	results := make([]resp, len(names))
	var wg sync.WaitGroup
	for i, n := range names {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			a := agents[name]
			start := time.Now()
			if verbose {
				a.OnEvent = logHandler(true)
			}
			var reply string
			var err error
			if len(a.ToolNames()) > 0 {
				reply, err = a.Run(context.Background(), msg)
			} else {
				reply, err = a.Send(context.Background(), msg)
			}
			results[idx] = resp{a.Name(), a.Persona(), reply, err, time.Since(start)}
		}(i, n)
	}
	wg.Wait()

	fmt.Println()
	for _, r := range results {
		fmt.Printf("  ┌─ %s (%s) [%s]\n", r.name, r.persona, r.elapsed.Round(time.Millisecond))
		if r.err != nil {
			fmt.Printf("  │ [error: %v]\n", r.err)
		} else {
			for _, line := range strings.Split(r.reply, "\n") {
				fmt.Printf("  │ %s\n", line)
			}
		}
		fmt.Println("  └─")
		fmt.Println()
	}
}
