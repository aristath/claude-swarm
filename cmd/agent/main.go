package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "swarm-agent",
		Usage: "Claude Swarm agent helper CLI",
		Commands: []*cli.Command{
			{
				Name:   "ask",
				Usage:  "Ask the orchestrator a question",
				Action: askQuestion,
			},
			{
				Name:  "complete",
				Usage: "Mark task as complete",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "output",
						Usage:    "Task output/results",
						Required: true,
					},
				},
				Action: completeTask,
			},
			{
				Name:   "check-followup",
				Usage:  "Check for orchestrator follow-up questions",
				Action: checkFollowup,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func askQuestion(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	question := c.Args().First()
	if question == "" {
		return fmt.Errorf("question text is required")
	}

	// Find next question number
	questionsDir := filepath.Join(agentDir, "questions")
	files, err := filepath.Glob(filepath.Join(questionsDir, "q-*.txt"))
	if err != nil {
		return fmt.Errorf("failed to list questions: %w", err)
	}
	qNum := len(files) + 1

	// Write question file
	qFile := filepath.Join(questionsDir, fmt.Sprintf("q-%d.txt", qNum))
	if err := os.WriteFile(qFile, []byte(question), 0644); err != nil {
		return fmt.Errorf("failed to write question: %w", err)
	}

	fmt.Printf("Question sent to orchestrator. Waiting for answer...\n")

	// Wait for answer (with timeout)
	aFile := filepath.Join(questionsDir, fmt.Sprintf("a-%d.txt", qNum))
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for answer (5 minutes)")

		case <-ticker.C:
			if _, err := os.Stat(aFile); err == nil {
				// Answer file exists, read it
				answer, err := os.ReadFile(aFile)
				if err != nil {
					return fmt.Errorf("failed to read answer: %w", err)
				}

				fmt.Printf("\n=== Orchestrator's Answer ===\n")
				fmt.Printf("%s\n", string(answer))
				fmt.Printf("============================\n\n")

				return nil
			}
		}
	}
}

func completeTask(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	output := c.String("output")

	// Write output file
	outputFile := filepath.Join(agentDir, "output.txt")
	if err := os.WriteFile(outputFile, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	// Write status file
	statusFile := filepath.Join(agentDir, "status.txt")
	if err := os.WriteFile(statusFile, []byte("completed"), 0644); err != nil {
		return fmt.Errorf("failed to write status: %w", err)
	}

	// Create COMPLETE marker
	completeFile := filepath.Join(agentDir, "COMPLETE")
	if err := os.WriteFile(completeFile, []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to create COMPLETE marker: %w", err)
	}

	fmt.Printf("Task marked as complete. Output saved.\n")
	fmt.Printf("Orchestrator will detect completion and spawn dependent tasks.\n")

	return nil
}

func checkFollowup(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	followupDir := filepath.Join(agentDir, "followup")

	// Check for unanswered follow-up questions
	files, err := filepath.Glob(filepath.Join(followupDir, "q-*.txt"))
	if err != nil {
		return fmt.Errorf("failed to list follow-up questions: %w", err)
	}

	for _, qFile := range files {
		aFile := filepath.Base(qFile)
		aFile = filepath.Join(followupDir, "a-"+aFile[2:]) // q-N.txt -> a-N.txt

		// Check if already answered
		if _, err := os.Stat(aFile); err == nil {
			continue
		}

		// Read question
		question, err := os.ReadFile(qFile)
		if err != nil {
			return fmt.Errorf("failed to read follow-up question: %w", err)
		}

		fmt.Printf("\n=== Orchestrator Follow-Up Question ===\n")
		fmt.Printf("%s\n", string(question))
		fmt.Printf("=====================================\n\n")
		fmt.Printf("Please provide your answer:\n")

		// Read answer from stdin
		var answer string
		fmt.Scanln(&answer)

		// Write answer file
		if err := os.WriteFile(aFile, []byte(answer), 0644); err != nil {
			return fmt.Errorf("failed to write answer: %w", err)
		}

		fmt.Printf("Answer sent to orchestrator.\n")
	}

	if len(files) == 0 {
		fmt.Printf("No pending follow-up questions.\n")
	}

	return nil
}
