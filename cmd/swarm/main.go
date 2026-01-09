package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aristath/claude-swarm/internal/orchestrator"
	"github.com/aristath/claude-swarm/internal/state"
	"github.com/aristath/claude-swarm/internal/workflow"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "swarm",
		Usage: "Claude Swarm orchestrator",
		Commands: []*cli.Command{
			{
				Name:   "init",
				Usage:  "Initialize a new swarm session",
				Action: initSession,
			},
			{
				Name:  "run",
				Usage: "Run an existing workflow",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "workflow",
						Usage:    "Path to workflow.yaml file",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "plan",
						Usage: "Path to plan.md file",
					},
				},
				Action: runWorkflow,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func initSession(c *cli.Context) error {
	// Generate session ID
	sessionID := fmt.Sprintf("swarm-%d", time.Now().Unix())
	swarmDir := filepath.Join(os.Getenv("HOME"), ".claude-swarm", sessionID)

	// Create swarm directory
	if err := os.MkdirAll(swarmDir, 0755); err != nil {
		return fmt.Errorf("failed to create swarm directory: %w", err)
	}

	// Create subdirectories
	for _, subdir := range []string{"agents", "logs"} {
		if err := os.MkdirAll(filepath.Join(swarmDir, subdir), 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Create default files
	planFile := filepath.Join(swarmDir, "plan.md")
	if err := os.WriteFile(planFile, []byte("# Plan\n\nDescribe your plan here...\n"), 0644); err != nil {
		return fmt.Errorf("failed to create plan file: %w", err)
	}

	workflowFile := filepath.Join(swarmDir, "workflow.yaml")
	workflowTemplate := `name: "My Workflow"
description: "Description of the workflow"

tasks:
  - id: "task1"
    agent_type: "general-purpose"
    description: "First task"
    prompt: |
      Describe the task here...
    depends_on: []
`
	if err := os.WriteFile(workflowFile, []byte(workflowTemplate), 0644); err != nil {
		return fmt.Errorf("failed to create workflow file: %w", err)
	}

	fmt.Printf("Swarm session initialized: %s\n", sessionID)
	fmt.Printf("Directory: %s\n\n", swarmDir)
	fmt.Printf("Next steps:\n")
	fmt.Printf("1. Edit the plan: %s\n", planFile)
	fmt.Printf("2. Edit the workflow: %s\n", workflowFile)
	fmt.Printf("3. Run the workflow: swarm run --workflow %s --plan %s\n", workflowFile, planFile)

	return nil
}

func runWorkflow(c *cli.Context) error {
	workflowPath := c.String("workflow")
	planPath := c.String("plan")

	// Parse workflow
	parser := workflow.NewParser()
	wf, err := parser.ParseFile(workflowPath)
	if err != nil {
		return fmt.Errorf("failed to parse workflow: %w", err)
	}

	// Read plan
	var plan string
	if planPath != "" {
		planData, err := os.ReadFile(planPath)
		if err != nil {
			return fmt.Errorf("failed to read plan: %w", err)
		}
		plan = string(planData)
	}

	// Determine swarm directory from workflow path
	swarmDir := filepath.Dir(workflowPath)

	// Generate session ID
	sessionID := filepath.Base(swarmDir)

	// Create state
	swarmState := state.NewSwarmState(sessionID, plan, wf)

	// Create orchestrator
	orch, err := orchestrator.NewOrchestrator(swarmDir, swarmState)
	if err != nil {
		return fmt.Errorf("failed to create orchestrator: %w", err)
	}

	fmt.Printf("Starting orchestration...\n")
	fmt.Printf("Session: %s\n", sessionID)
	fmt.Printf("Workflow: %s\n", wf.Name)
	fmt.Printf("Tasks: %d\n\n", len(wf.Tasks))

	// Run orchestrator
	if err := orch.Run(); err != nil {
		return fmt.Errorf("orchestration failed: %w", err)
	}

	fmt.Printf("\nWorkflow completed successfully!\n")
	fmt.Printf("Check agent outputs in: %s/agents/\n", swarmDir)

	return nil
}
