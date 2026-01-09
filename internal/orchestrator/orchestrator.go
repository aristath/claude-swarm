package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aristath/claude-swarm/internal/state"
	"github.com/aristath/claude-swarm/internal/workflow"
)

// Orchestrator coordinates the swarm execution
type Orchestrator struct {
	swarmDir       string
	state          *state.SwarmState
	monitor        *FileMonitor
	persistence    *state.Persistence
	parser         *workflow.Parser
	messageHandler *MessageHandler
	done           chan bool
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator(swarmDir string, swarmState *state.SwarmState) (*Orchestrator, error) {
	monitor, err := NewFileMonitor(swarmDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create file monitor: %w", err)
	}

	orch := &Orchestrator{
		swarmDir:    swarmDir,
		state:       swarmState,
		monitor:     monitor,
		persistence: state.NewPersistence(swarmDir),
		parser:      workflow.NewParser(),
		done:        make(chan bool),
	}

	// Initialize message handler (needs reference to orchestrator)
	orch.messageHandler = NewMessageHandler(orch)

	return orch, nil
}

// Run starts the orchestrator
func (o *Orchestrator) Run() error {
	// Start file monitor
	if err := o.monitor.Start(); err != nil {
		return fmt.Errorf("failed to start file monitor: %w", err)
	}

	// Spawn initial tasks
	if err := o.spawnReadyAgents(); err != nil {
		return fmt.Errorf("failed to spawn initial agents: %w", err)
	}

	// Main event loop
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.done:
			return nil

		case event := <-o.monitor.Events():
			if err := o.handleEvent(event); err != nil {
				fmt.Printf("Error handling event: %v\n", err)
			}

		case err := <-o.monitor.Errors():
			fmt.Printf("Monitor error: %v\n", err)

		case <-ticker.C:
			// Periodic tasks
			o.spawnReadyAgents()

			// Save state
			if err := o.persistence.Save(o.state); err != nil {
				fmt.Printf("Failed to save state: %v\n", err)
			}

			// Check if workflow is complete
			if o.state.IsComplete() {
				o.state.MarkComplete()
				o.done <- true
				return nil
			}
		}
	}
}

// Stop stops the orchestrator
func (o *Orchestrator) Stop() {
	o.monitor.Stop()
	close(o.done)
}

// handleEvent processes a file event
func (o *Orchestrator) handleEvent(event workflow.FileEvent) error {
	switch event.Type {
	case workflow.EventQuestionAsked:
		return o.handleQuestionAsked(event)

	case workflow.EventTaskCompleted:
		return o.handleTaskCompleted(event)

	case workflow.EventFollowUpAnswered:
		return o.handleFollowUpAnswered(event)

	case workflow.EventFileOperationRequest:
		return o.messageHandler.HandleMessage(event.FilePath)

	case workflow.EventAgentStatusUpdate:
		// Just log it, state updates happen elsewhere
		return nil

	default:
		return nil
	}
}

// handleQuestionAsked handles a question from an agent
func (o *Orchestrator) handleQuestionAsked(event workflow.FileEvent) error {
	// Read the question
	question, err := os.ReadFile(event.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read question: %w", err)
	}

	// Extract question number from filename (e.g., q-1.txt -> 1)
	filename := filepath.Base(event.FilePath)
	qNum := o.extractQuestionNumber(filename)

	// Add to state
	o.state.AddQuestion(event.AgentID, string(question))

	// Formulate answer
	answer := o.formulateAnswer(event.AgentID, string(question))

	// Write answer file
	answerFile := strings.Replace(event.FilePath, "q-", "a-", 1)
	if err := os.WriteFile(answerFile, []byte(answer), 0644); err != nil {
		return fmt.Errorf("failed to write answer: %w", err)
	}

	// Update state
	o.state.AnswerQuestion(event.AgentID, qNum, answer)

	fmt.Printf("[%s] Question from agent %s: %s\n", time.Now().Format("15:04:05"), event.AgentID, string(question))
	fmt.Printf("[%s] Answer: %s\n", time.Now().Format("15:04:05"), answer)

	return nil
}

// handleTaskCompleted handles task completion
func (o *Orchestrator) handleTaskCompleted(event workflow.FileEvent) error {
	// Read output file
	outputFile := filepath.Join(filepath.Dir(event.FilePath), "output.txt")
	output, err := os.ReadFile(outputFile)
	if err != nil {
		return fmt.Errorf("failed to read output: %w", err)
	}

	// Mark task as completed
	if err := o.state.CompleteTask(event.AgentID, string(output)); err != nil {
		return fmt.Errorf("failed to complete task: %w", err)
	}

	fmt.Printf("[%s] Task completed: %s\n", time.Now().Format("15:04:05"), event.AgentID)

	// Spawn dependent tasks
	return o.spawnReadyAgents()
}

// handleFollowUpAnswered handles a follow-up answer from an agent
func (o *Orchestrator) handleFollowUpAnswered(event workflow.FileEvent) error {
	// Read the answer
	answer, err := os.ReadFile(event.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read follow-up answer: %w", err)
	}

	fmt.Printf("[%s] Follow-up answer from %s: %s\n", time.Now().Format("15:04:05"), event.AgentID, string(answer))

	return nil
}

// formulateAnswer generates an answer based on the plan and context
func (o *Orchestrator) formulateAnswer(agentID, question string) string {
	// Get the task
	task := o.state.GetTask(agentID)
	if task == nil {
		return "Task not found"
	}

	// Get the agent state
	agent := o.state.GetAgent(agentID)
	if agent == nil {
		return "Agent not found"
	}

	// This is where Claude A would formulate the answer based on:
	// - The original plan (o.state.Plan)
	// - The task requirements (task)
	// - Previous Q&A history (agent.Questions)
	// - Overall workflow context

	// For now, return a placeholder that Claude A will see and can respond to
	return fmt.Sprintf(`[ORCHESTRATOR NEEDS TO FORMULATE ANSWER]

Question from agent '%s': %s

Context:
- Task: %s
- Task Description: %s
- Original Plan: See plan.md
- Previous Questions: %d

Please formulate an answer based on the plan and context.`,
		agentID,
		question,
		task.ID,
		task.Description,
		len(agent.Questions))
}

// spawnReadyAgents spawns agents for tasks that are ready
func (o *Orchestrator) spawnReadyAgents() error {
	readyTasks := o.state.GetReadyTasks()

	for _, task := range readyTasks {
		if err := o.spawnAgent(task); err != nil {
			fmt.Printf("Failed to spawn agent for task %s: %v\n", task.ID, err)
			continue
		}
	}

	return nil
}

// spawnAgent spawns an agent for a task
func (o *Orchestrator) spawnAgent(task workflow.Task) error {
	// Create agent directory
	agentDir := filepath.Join(o.swarmDir, "agents", fmt.Sprintf("agent-%s", task.ID))
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("failed to create agent directory: %w", err)
	}

	// Create subdirectories
	for _, subdir := range []string{"questions", "followup", "messages", "responses"} {
		if err := os.MkdirAll(filepath.Join(agentDir, subdir), 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Watch the agent directory
	if err := o.monitor.WatchAgentDir(agentDir); err != nil {
		return fmt.Errorf("failed to watch agent directory: %w", err)
	}

	// Generate context file
	context := o.generateAgentContext(task)
	contextFile := filepath.Join(agentDir, "context.txt")
	if err := os.WriteFile(contextFile, []byte(context), 0644); err != nil {
		return fmt.Errorf("failed to write context file: %w", err)
	}

	// Add agent to state
	if err := o.state.AddAgent(task.ID, agentDir); err != nil {
		return fmt.Errorf("failed to add agent to state: %w", err)
	}

	// Generate spawn prompt
	prompt := o.generateSpawnPrompt(task, agentDir)

	fmt.Printf("\n[SPAWN_AGENT] %s\n", task.ID)
	fmt.Printf("Type: %s\n", task.AgentType)
	fmt.Printf("Directory: %s\n", agentDir)
	fmt.Printf("\nPrompt:\n%s\n", prompt)
	fmt.Printf("\n[ORCHESTRATOR] Please use the Task tool to spawn this agent with the above prompt.\n\n")

	return nil
}

// generateAgentContext generates the context file for an agent
func (o *Orchestrator) generateAgentContext(task workflow.Task) string {
	// Get outputs from dependencies
	outputs := o.state.GetOutputs()
	previousOutputs := ""

	for _, depID := range task.DependsOn {
		if output, exists := outputs[depID]; exists {
			previousOutputs += fmt.Sprintf("## Output from task: %s\n%s\n\n", depID, output)
		}
	}

	// Interpolate prompt with dependency outputs
	interpolatedPrompt := o.parser.InterpolatePrompt(task.Prompt, outputs)

	return fmt.Sprintf(`# SWARM AGENT - Task: %s

You are part of a Claude Swarm orchestration system.

## Your Environment
- Session: %s
- Working directory: %s
- Swarm directory: %s

## Your Task
%s

## Original Plan
%s

## Context from Previous Tasks
%s

## IMPORTANT: Swarm Protocol

You can communicate with the orchestrator (Claude A) using the swarm-agent CLI:

1. **To ask a question**:
   cd %s
   swarm-agent ask "Your question here"

   The orchestrator will respond based on the original plan.

2. **File Operations via Message Bus** (NO PERMISSION PROMPTS):
   All file operations go through the orchestrator - no permission prompts!

   # Read a file
   swarm-agent file-read /path/to/file

   # Write a file
   swarm-agent file-write /path/to/file "content here"

   # Edit a file (replace text)
   swarm-agent file-edit /path/to/file --old "old text" --new "new text"

   # Execute bash command
   swarm-agent bash "ls -la" --dir /some/directory

   # Search for files with glob pattern
   swarm-agent glob "**/*.go"

   IMPORTANT: Use these commands instead of trying to read/write files directly.
   The orchestrator will execute operations on your behalf.

3. **When you complete your task**:
   swarm-agent complete --output "Your final output here"

   This will mark the task as complete and trigger dependent tasks.

## Instructions
1. Work on your task autonomously
2. Use swarm-agent commands for ALL file operations (no permission prompts)
3. Ask questions if you need guidance (orchestrator has the full plan)
4. Write your output when done using swarm-agent complete
5. Be thorough and follow the plan's intent

Begin your task now.
`,
		task.ID,
		o.state.SessionID,
		filepath.Join(o.swarmDir, "agents", fmt.Sprintf("agent-%s", task.ID)),
		o.swarmDir,
		interpolatedPrompt,
		o.state.Plan,
		previousOutputs,
		filepath.Join(o.swarmDir, "agents", fmt.Sprintf("agent-%s", task.ID)),
	)
}

// generateSpawnPrompt generates the prompt for spawning an agent via Task tool
func (o *Orchestrator) generateSpawnPrompt(task workflow.Task, agentDir string) string {
	contextFile := filepath.Join(agentDir, "context.txt")

	return fmt.Sprintf(`You are Agent '%s' in a Claude Swarm orchestration system.

Read your context and instructions:
cat %s

Your working directory: %s

You have access to the swarm-agent CLI tool for communication:
- swarm-agent ask "question" - Ask the orchestrator for guidance
- swarm-agent file-read <path> - Read a file (no permission prompts)
- swarm-agent file-write <path> <content> - Write a file
- swarm-agent file-edit <path> --old "..." --new "..." - Edit a file
- swarm-agent bash <command> - Execute bash command
- swarm-agent glob <pattern> - Search files with glob pattern
- swarm-agent complete --output "results" - Mark task complete

Environment variables:
export SWARM_SESSION_ID=%s
export SWARM_AGENT_DIR=%s

IMPORTANT: Use swarm-agent commands for ALL file operations to avoid permission prompts.

Begin your task now by reading the context file and following the instructions.
`,
		task.ID,
		contextFile,
		agentDir,
		o.state.SessionID,
		agentDir,
	)
}

// extractQuestionNumber extracts the question number from a filename
func (o *Orchestrator) extractQuestionNumber(filename string) int {
	// Extract number from q-N.txt or a-N.txt
	parts := strings.Split(strings.TrimSuffix(filename, ".txt"), "-")
	if len(parts) >= 2 {
		var num int
		fmt.Sscanf(parts[1], "%d", &num)
		return num
	}
	return 1
}
