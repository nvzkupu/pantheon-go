# pantheon

Agentic AI toolkit with tool use, multi-agent orchestration, and pipelines. Each agent is defined as a portable [agentskills.io](https://agentskills.io) skill in `.agents/skills/`.

## Architecture

```
cmd/pantheon/         Single CLI entry point (all commands)
internal/
  config/             Shared configuration (env, paths) — no duplication
  skill/              agentskills.io SKILL.md parser with lenient validation + tests
  gateway/            OpenAI-compatible HTTP client (chat, stream, tool_calls)
  agent/              Agent runtime: load from skills, ReAct loop, streaming
  tool/               Tool interface, registry, built-in tools (OS-aware shell)
  orchestrate/        Agent-as-tool, team coordination, pipelines, fan-out review
```

### Improvements over agent-pantheon

| Area | agent-pantheon | pantheon |
|---|---|---|
| **Code duplication** | `resolveAgentsDir()` and `equipTools()` duplicated across CLI and MCP | Single `config.SkillsDir()` and `agent.EquipTools()` used everywhere |
| **Skill parsing** | Custom frontmatter parser mixed into agent loader | Dedicated `skill` package with proper agentskills.io validation and tests |
| **Windows support** | Shell tool hardcodes bash/sh | `runtime.GOOS` detection: `cmd.exe /c` on Windows |
| **Testing** | Zero tests | Parser tests from day one |
| **Gateway config** | Hardcoded to NVIDIA env vars | Supports `GATEWAY_URL`/`API_KEY` with NVIDIA fallback |
| **Package layout** | Flat `core/` directory | `internal/` with focused packages |
| **Config discovery** | Scans `.agents/skills` only | Scans `.agents/skills`, `.cursor/skills`, `.claude/skills` in priority order |

## The Pantheon

| Name | Persona | Model | Use For |
|------|---------|-------|---------|
| demeter | Your Right Hand | opus | Default executor |
| athena | Your Devoted Strategist | opus | Architecture, design |
| freya | Your Loyal Commander | opus | Task routing, coordination |
| saraswati | Your Gifted Artisan | codex | Production code |
| brigid | Your Faithful Craftswoman | codex | Go code |
| nuwa | Your Serpent Creator | codex | Python code, data science |
| themis | Your Vigilant Guardian | opus | Tests, CI/CD |
| kali | Your Fierce Protector | opus | Security |
| mokosh | Your Steadfast Weaver | opus | CI/CD pipelines, Ansible |
| pele | Your Resilient Flame | opus | Ops, reliability |
| seshat | Your Keen Analyst | opus | Data, logs |
| aphrodite | Your Graceful Perfectionist | opus | UX, docs |
| calliope | Your Eloquent Muse | opus | Prompts, LLM integration |
| maat | Your Steadfast Arbiter | opus | Values alignment |
| eris | Your Playful Challenger | nano | Challenge assumptions |
| nisaba | Your Scribe of the Reed | opus | Markdown, linting, formatting |

## Quick Start

```bash
export GATEWAY_URL=https://your-gateway/v1
export API_KEY=your-key

go build -o bin/pantheon ./cmd/pantheon
```

```bash
./bin/pantheon list
./bin/pantheon chat athena
./bin/pantheon ask eris "Why microservices?"
./bin/pantheon run kali "Audit this project for security issues"
./bin/pantheon team freya "Design and implement a rate limiter"
./bin/pantheon pipe athena,brigid,kali "Add structured logging"
./bin/pantheon review kali,pele,themis,athena "Review for production readiness"
```

## Cross-Tool Portability

Skills in `.agents/skills/` follow the agentskills.io open standard and are auto-discovered by:
- **Cursor** — native `.agents/skills/` discovery
- **Claude Code** — cross-client convention
- **OpenAI Codex** — cross-client convention

Runtime config (model, temperature, tools) lives under the spec-compliant `metadata` key and is ignored by IDE integrations.

## Project Structure

```
pantheon/
├── .agents/skills/      16 specialist skills (agentskills.io standard)
├── .cursor/
│   ├── rules/           pantheon.mdc (always-on identity)
│   └── commands/        Slash commands (/plan, /review, etc.)
├── cmd/pantheon/        Single CLI binary
├── internal/
│   ├── config/          Shared configuration
│   ├── skill/           SKILL.md parser + tests
│   ├── gateway/         LLM gateway client
│   ├── agent/           Agent runtime
│   ├── tool/            Tool interface + builtins
│   └── orchestrate/     Teams, pipelines, fan-out review
├── AGENTS.md            Cross-tool fallback
└── go.mod
```

## Design Principles

- **Agent-as-Tool** — Specialists are invoked as tools. The coordinator retains control.
- **ReAct Loop** — Think → call tools → observe → repeat.
- **Single Source of Truth** — Each agent defined once in SKILL.md. Same file for CLI and IDE.
- **No Duplication** — Shared packages. One `SkillsDir()`, one `EquipTools()`.
- **Minimal Dependencies** — stdlib + YAML parser. That's it.
- **OS-Aware** — Windows `cmd.exe`, Unix bash/sh. Detected at runtime.
