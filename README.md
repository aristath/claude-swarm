# Claude Swarm

A conversational multi-agent orchestration system where Claude A (orchestrator) manages multiple Claude agent instances with bidirectional communication.

## Overview

Claude Swarm enables sophisticated multi-agent workflows where:
- **Orchestrator (Claude A)** coordinates multiple agents, maintains plan context, and answers questions
- **Worker Agents (Claude B, C, D...)** execute tasks autonomously, ask for guidance when needed
- **Bidirectional Communication** via filesystem (event-driven with fsnotify, no polling)
- **Dependency Management** ensures tasks execute in correct order
- **Context Preservation** maintains original plan throughout execution

## Architecture

### Key Components

1. **Orchestrator** (`internal/orchestrator/`)
   - Event-driven file monitoring using fsnotify
   - Autonomous question answering based on plan context
   - Agent spawning and coordination
   - State management and persistence

2. **Workflow System** (`internal/workflow/`)
   - YAML-based workflow definitions
   - Task dependency resolution
   - Circular dependency detection
   - Variable interpolation (`{task-id.output}`)

3. **State Management** (`internal/state/`)
   - In-memory state with JSON persistence
   - Thread-safe operations
   - Event logging and history

4. **Agent Helper CLI** (`cmd/agent/`)
   - `swarm-agent ask` - Ask orchestrator questions
   - `swarm-agent complete` - Mark task complete
   - `swarm-agent check-followup` - Check for orchestrator questions

## Installation

```bash
# Clone the repository
git clone git@github.com:aristath/claude-swarm.git
cd claude-swarm

# Build binaries
go build -o swarm ./cmd/swarm
go build -o swarm-agent ./cmd/agent

# Install to PATH (optional)
sudo cp swarm /usr/local/bin/
sudo cp swarm-agent /usr/local/bin/
```

## Usage

### 1. Initialize a Session

```bash
swarm init
```

This creates:
- Session directory at `~/.claude-swarm/swarm-<timestamp>/`
- `plan.md` - Describe your plan here
- `workflow.yaml` - Define your task workflow

### 2. Edit Plan and Workflow

**plan.md** - Describe what you want to accomplish:
```markdown
# My Plan

## Goal
Refactor API endpoints for better organization

## Approach
1. Analyze existing endpoints
2. Group by functionality
3. Create refactoring plan
4. Implement changes
5. Test

## Success Criteria
- All endpoints work
- Better organized code
- Tests pass
```

**workflow.yaml** - Define tasks with dependencies:
```yaml
name: "API Refactoring"
description: "Refactor and improve API structure"

tasks:
  - id: "analyze"
    agent_type: "Explore"
    description: "Analyze existing API endpoints"
    prompt: |
      Find all API endpoints in the codebase.
      Document their paths, methods, and purposes.
    depends_on: []

  - id: "plan"
    agent_type: "Plan"
    description: "Create refactoring plan"
    prompt: |
      Based on this analysis: {analyze.output}

      Create a detailed refactoring plan to improve organization.
    depends_on: ["analyze"]

  - id: "implement"
    agent_type: "general-purpose"
    description: "Implement refactoring"
    prompt: |
      Implement this plan: {plan.output}

      Original analysis: {analyze.output}
    depends_on: ["plan"]
```

### 3. Run the Workflow

```bash
swarm run --workflow ~/.claude-swarm/swarm-*/workflow.yaml --plan ~/.claude-swarm/swarm-*/plan.md
```

The orchestrator will:
1. Parse the workflow
2. Spawn agents for tasks with satisfied dependencies
3. Monitor for questions and completion
4. Answer agent questions based on the plan
5. Spawn dependent tasks when prerequisites complete

### 4. Spawning Agents (Claude A)

When the orchestrator is ready to spawn an agent, it will output:

```
[SPAWN_AGENT] analyze
Type: Explore
Directory: ~/.claude-swarm/swarm-123/agents/agent-analyze/

Prompt:
You are Agent 'analyze' in a Claude Swarm...
[full context and instructions]

[ORCHESTRATOR] Please use the Task tool to spawn this agent with the above prompt.
```

**Action**: Use the Task tool to spawn a new agent with the provided prompt.

### 5. Working as an Agent (Claude B)

When spawned, you'll see:

```bash
# Read your context
cat ~/.claude-swarm/swarm-123/agents/agent-analyze/context.txt

# Do your work...

# If stuck, ask a question
swarm-agent ask "Should I include internal APIs in the analysis?"

# When done
swarm-agent complete --output "Found 47 endpoints across 12 files..."
```

The orchestrator detects completion via fsnotify and spawns dependent tasks.

## Communication Protocol

### Agent → Orchestrator

```bash
# Agent creates question file
echo "Question?" > questions/q-1.txt

# Orchestrator detects via fsnotify (instant, no polling)
# Reads question, plan, agent context
# Formulates answer based on plan
# Writes answer file

# Agent reads answer
cat questions/a-1.txt
```

### Orchestrator → Agent

```bash
# Orchestrator creates follow-up question
echo "Need clarification..." > followup/q-1.txt

# Agent checks periodically
swarm-agent check-followup

# Agent answers
echo "Here's the clarification..." > followup/a-1.txt
```

## Directory Structure

```
~/.claude-swarm/<session-id>/
├── plan.md                      # Original plan
├── workflow.yaml                # Workflow definition
├── state.json                   # Current state (auto-saved)
├── agents/
│   ├── agent-<task-id>/
│   │   ├── context.txt         # Task context + plan
│   │   ├── questions/          # Agent → Orchestrator Q&A
│   │   │   ├── q-1.txt
│   │   │   ├── a-1.txt
│   │   │   └── ...
│   │   ├── followup/           # Orchestrator → Agent Q&A
│   │   │   ├── q-1.txt
│   │   │   └── a-1.txt
│   │   ├── output.txt          # Final task output
│   │   ├── status.txt          # Status
│   │   └── COMPLETE            # Completion marker
```

## Features

### Event-Driven Architecture
- Uses fsnotify for instant file change detection
- No polling delays - answers appear within seconds
- Efficient resource usage

### Autonomous Question Answering
- Orchestrator maintains plan context
- Formulates answers based on original plan intent
- Agents get guidance without human intervention

### Dependency Management
- Tasks specify dependencies via `depends_on`
- Orchestrator spawns tasks when all dependencies complete
- Detects circular dependencies at parse time

### Variable Interpolation
- Use `{task-id.output}` in prompts
- Automatically replaced with task outputs
- Context flows between dependent tasks

### State Persistence
- State saved to JSON every 5 seconds
- Crash recovery support
- Complete event history

## Current Status

### ✅ Implemented

- Core orchestration engine
- Event-driven file monitoring (fsnotify)
- Workflow parser with validation
- State management and persistence
- Agent spawning logic
- Question/answer protocol
- Agent helper CLI
- Dependency resolution
- Variable interpolation

### 🚧 In Progress

- Split-screen TUI (Bubbletea)
  - Orchestrator view (main area)
  - Agent sidebar (right panel)
  - Real-time event log
  - Progress indicators

### 📋 Planned

- Interactive planning mode in TUI
- Workflow templates
- Error recovery and retries
- Concurrent agent limits
- Web UI
- Distributed mode (SSH)
- CI/CD integration

## Example Workflow

See the test workflow at `~/.claude-swarm/swarm-*/` for a working example.

## Technical Details

### Dependencies

- **fsnotify** - File system event monitoring
- **yaml.v3** - YAML parsing
- **cli/v2** - CLI framework

### Concurrency

- File monitor runs in separate goroutine
- All state operations are thread-safe (sync.RWMutex)
- Agents run independently (spawned via Task tool)

### Error Handling

- Graceful degradation on file errors
- State auto-save on errors
- Agent failures logged and tracked

## Contributing

This project is under active development. The core orchestration system is functional. The TUI is next on the roadmap.

## License

MIT

## Repository

https://github.com/aristath/claude-swarm
