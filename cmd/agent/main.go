package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aristath/claude-swarm/internal/workflow"
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
			{
				Name:      "file-read",
				Usage:     "Read a file via orchestrator",
				ArgsUsage: "<path>",
				Action:    fileRead,
			},
			{
				Name:      "file-write",
				Usage:     "Write a file via orchestrator",
				ArgsUsage: "<path> <content>",
				Action:    fileWrite,
			},
			{
				Name:      "file-edit",
				Usage:     "Edit a file via orchestrator",
				ArgsUsage: "<path>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "old",
						Usage:    "Old string to replace",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "new",
						Usage:    "New string to insert",
						Required: true,
					},
				},
				Action: fileEdit,
			},
			{
				Name:      "bash",
				Usage:     "Execute bash command via orchestrator",
				ArgsUsage: "<command>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "dir",
						Usage: "Working directory",
					},
				},
				Action: bashCommand,
			},
			{
				Name:      "glob",
				Usage:     "Search files with glob pattern via orchestrator",
				ArgsUsage: "<pattern>",
				Action:    globPattern,
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

func fileRead(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	path := c.Args().First()
	if path == "" {
		return fmt.Errorf("file path is required")
	}

	// Generate message ID
	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	// Create message
	msg := workflow.Message{
		ID:        msgID,
		Type:      workflow.MessageTypeReadFile,
		Path:      path,
		Timestamp: time.Now(),
	}

	// Write message
	messagesDir := filepath.Join(agentDir, "messages")
	msgFile := filepath.Join(messagesDir, fmt.Sprintf("%s.json", msgID))

	msgData, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(msgFile, msgData, 0644); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Wait for response
	responsesDir := filepath.Join(agentDir, "responses")
	responseFile := filepath.Join(responsesDir, fmt.Sprintf("%s-result.json", msgID))

	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for response (30 seconds)")

		case <-ticker.C:
			if _, err := os.Stat(responseFile); err == nil {
				// Response exists, read it
				respData, err := os.ReadFile(responseFile)
				if err != nil {
					return fmt.Errorf("failed to read response: %w", err)
				}

				var resp workflow.Response
				if err := json.Unmarshal(respData, &resp); err != nil {
					return fmt.Errorf("failed to parse response: %w", err)
				}

				if resp.Status == "error" {
					return fmt.Errorf("orchestrator error: %s", resp.Error)
				}

				fmt.Printf("%s", resp.Data)
				return nil
			}
		}
	}
}

func fileWrite(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	if c.Args().Len() < 2 {
		return fmt.Errorf("path and content are required")
	}

	path := c.Args().Get(0)
	content := c.Args().Get(1)

	// Generate message ID
	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	// Create message
	msg := workflow.Message{
		ID:        msgID,
		Type:      workflow.MessageTypeWriteFile,
		Path:      path,
		Content:   content,
		Timestamp: time.Now(),
	}

	// Write message
	messagesDir := filepath.Join(agentDir, "messages")
	msgFile := filepath.Join(messagesDir, fmt.Sprintf("%s.json", msgID))

	msgData, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(msgFile, msgData, 0644); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Wait for response
	responsesDir := filepath.Join(agentDir, "responses")
	responseFile := filepath.Join(responsesDir, fmt.Sprintf("%s-result.json", msgID))

	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for response (30 seconds)")

		case <-ticker.C:
			if _, err := os.Stat(responseFile); err == nil {
				// Response exists, read it
				respData, err := os.ReadFile(responseFile)
				if err != nil {
					return fmt.Errorf("failed to read response: %w", err)
				}

				var resp workflow.Response
				if err := json.Unmarshal(respData, &resp); err != nil {
					return fmt.Errorf("failed to parse response: %w", err)
				}

				if resp.Status == "error" {
					return fmt.Errorf("orchestrator error: %s", resp.Error)
				}

				fmt.Printf("%s\n", resp.Data)
				return nil
			}
		}
	}
}

func fileEdit(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	path := c.Args().First()
	if path == "" {
		return fmt.Errorf("file path is required")
	}

	oldString := c.String("old")
	newString := c.String("new")

	// Generate message ID
	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	// Create message
	msg := workflow.Message{
		ID:   msgID,
		Type: workflow.MessageTypeEditFile,
		Path: path,
		Edits: []workflow.Edit{
			{
				OldString: oldString,
				NewString: newString,
			},
		},
		Timestamp: time.Now(),
	}

	// Write message
	messagesDir := filepath.Join(agentDir, "messages")
	msgFile := filepath.Join(messagesDir, fmt.Sprintf("%s.json", msgID))

	msgData, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(msgFile, msgData, 0644); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Wait for response
	responsesDir := filepath.Join(agentDir, "responses")
	responseFile := filepath.Join(responsesDir, fmt.Sprintf("%s-result.json", msgID))

	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for response (30 seconds)")

		case <-ticker.C:
			if _, err := os.Stat(responseFile); err == nil {
				// Response exists, read it
				respData, err := os.ReadFile(responseFile)
				if err != nil {
					return fmt.Errorf("failed to read response: %w", err)
				}

				var resp workflow.Response
				if err := json.Unmarshal(respData, &resp); err != nil {
					return fmt.Errorf("failed to parse response: %w", err)
				}

				if resp.Status == "error" {
					return fmt.Errorf("orchestrator error: %s", resp.Error)
				}

				fmt.Printf("%s\n", resp.Data)
				return nil
			}
		}
	}
}

func bashCommand(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	command := c.Args().First()
	if command == "" {
		return fmt.Errorf("command is required")
	}

	workingDir := c.String("dir")

	// Generate message ID
	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	// Create message
	msg := workflow.Message{
		ID:         msgID,
		Type:       workflow.MessageTypeBash,
		Command:    command,
		WorkingDir: workingDir,
		Timestamp:  time.Now(),
	}

	// Write message
	messagesDir := filepath.Join(agentDir, "messages")
	msgFile := filepath.Join(messagesDir, fmt.Sprintf("%s.json", msgID))

	msgData, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(msgFile, msgData, 0644); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Wait for response
	responsesDir := filepath.Join(agentDir, "responses")
	responseFile := filepath.Join(responsesDir, fmt.Sprintf("%s-result.json", msgID))

	timeout := time.After(60 * time.Second) // Longer timeout for bash commands
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for response (60 seconds)")

		case <-ticker.C:
			if _, err := os.Stat(responseFile); err == nil {
				// Response exists, read it
				respData, err := os.ReadFile(responseFile)
				if err != nil {
					return fmt.Errorf("failed to read response: %w", err)
				}

				var resp workflow.Response
				if err := json.Unmarshal(respData, &resp); err != nil {
					return fmt.Errorf("failed to parse response: %w", err)
				}

				if resp.Status == "error" {
					// For bash, include output even on error
					fmt.Printf("%s", resp.Data)
					return fmt.Errorf("command failed: %s", resp.Error)
				}

				fmt.Printf("%s", resp.Data)
				return nil
			}
		}
	}
}

func globPattern(c *cli.Context) error {
	agentDir := os.Getenv("SWARM_AGENT_DIR")
	if agentDir == "" {
		return fmt.Errorf("SWARM_AGENT_DIR environment variable not set")
	}

	pattern := c.Args().First()
	if pattern == "" {
		return fmt.Errorf("glob pattern is required")
	}

	// Generate message ID
	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	// Create message
	msg := workflow.Message{
		ID:        msgID,
		Type:      workflow.MessageTypeGlob,
		Path:      pattern,
		Timestamp: time.Now(),
	}

	// Write message
	messagesDir := filepath.Join(agentDir, "messages")
	msgFile := filepath.Join(messagesDir, fmt.Sprintf("%s.json", msgID))

	msgData, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(msgFile, msgData, 0644); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Wait for response
	responsesDir := filepath.Join(agentDir, "responses")
	responseFile := filepath.Join(responsesDir, fmt.Sprintf("%s-result.json", msgID))

	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for response (30 seconds)")

		case <-ticker.C:
			if _, err := os.Stat(responseFile); err == nil {
				// Response exists, read it
				respData, err := os.ReadFile(responseFile)
				if err != nil {
					return fmt.Errorf("failed to read response: %w", err)
				}

				var resp workflow.Response
				if err := json.Unmarshal(respData, &resp); err != nil {
					return fmt.Errorf("failed to parse response: %w", err)
				}

				if resp.Status == "error" {
					return fmt.Errorf("orchestrator error: %s", resp.Error)
				}

				fmt.Printf("%s\n", resp.Data)
				return nil
			}
		}
	}
}
