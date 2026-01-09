package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aristath/claude-swarm/internal/workflow"
)

// MessageHandler handles messages from agents
type MessageHandler struct {
	orchestrator *Orchestrator
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(orch *Orchestrator) *MessageHandler {
	return &MessageHandler{
		orchestrator: orch,
	}
}

// HandleMessage processes a message from an agent
func (h *MessageHandler) HandleMessage(messagePath string) error {
	// Read message
	data, err := os.ReadFile(messagePath)
	if err != nil {
		return fmt.Errorf("failed to read message: %w", err)
	}

	var msg workflow.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Execute operation
	response := h.executeOperation(&msg)

	// Write response
	agentDir := filepath.Dir(filepath.Dir(messagePath)) // messages/msg-X.json -> agent dir
	responseDir := filepath.Join(agentDir, "responses")
	os.MkdirAll(responseDir, 0755)

	responseFile := filepath.Join(responseDir, fmt.Sprintf("%s-result.json", msg.ID))
	responseData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if err := os.WriteFile(responseFile, responseData, 0644); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	fmt.Printf("[%s] Handled message %s: %s (status: %s)\n",
		time.Now().Format("15:04:05"),
		msg.ID,
		msg.Type,
		response.Status)

	return nil
}

// executeOperation executes the requested operation
func (h *MessageHandler) executeOperation(msg *workflow.Message) workflow.Response {
	response := workflow.Response{
		MessageID: msg.ID,
		Timestamp: time.Now(),
	}

	switch msg.Type {
	case workflow.MessageTypeReadFile:
		content, err := os.ReadFile(msg.Path)
		if err != nil {
			response.Status = "error"
			response.Error = err.Error()
		} else {
			response.Status = "success"
			response.Data = string(content)
		}

	case workflow.MessageTypeWriteFile:
		err := os.WriteFile(msg.Path, []byte(msg.Content), 0644)
		if err != nil {
			response.Status = "error"
			response.Error = err.Error()
		} else {
			response.Status = "success"
			response.Data = fmt.Sprintf("Wrote %d bytes to %s", len(msg.Content), msg.Path)
		}

	case workflow.MessageTypeEditFile:
		err := h.applyEdits(msg.Path, msg.Edits)
		if err != nil {
			response.Status = "error"
			response.Error = err.Error()
		} else {
			response.Status = "success"
			response.Data = fmt.Sprintf("Applied %d edits to %s", len(msg.Edits), msg.Path)
		}

	case workflow.MessageTypeBash:
		output, err := h.executeBash(msg.Command, msg.WorkingDir)
		if err != nil {
			response.Status = "error"
			response.Error = err.Error()
			response.Data = output // Include partial output
		} else {
			response.Status = "success"
			response.Data = output
		}

	case workflow.MessageTypeGlob:
		matches, err := h.executeGlob(msg.Path)
		if err != nil {
			response.Status = "error"
			response.Error = err.Error()
		} else {
			response.Status = "success"
			response.Data = strings.Join(matches, "\n")
		}

	case workflow.MessageTypeGrep:
		results, err := h.executeGrep(msg)
		if err != nil {
			response.Status = "error"
			response.Error = err.Error()
		} else {
			response.Status = "success"
			response.Data = results
		}

	default:
		response.Status = "error"
		response.Error = fmt.Sprintf("unknown message type: %s", msg.Type)
	}

	return response
}

// applyEdits applies edit operations to a file
func (h *MessageHandler) applyEdits(path string, edits []workflow.Edit) error {
	// Read current content
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	result := string(content)

	// Apply each edit
	for i, edit := range edits {
		// Check if old_string exists
		if !strings.Contains(result, edit.OldString) {
			return fmt.Errorf("edit %d: old_string not found in file", i+1)
		}

		// Replace (only first occurrence to match Edit tool behavior)
		result = strings.Replace(result, edit.OldString, edit.NewString, 1)
	}

	// Write back
	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// executeBash executes a bash command
func (h *MessageHandler) executeBash(command, workingDir string) (string, error) {
	cmd := exec.Command("bash", "-c", command)

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// executeGlob executes a glob pattern
func (h *MessageHandler) executeGlob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob failed: %w", err)
	}

	return matches, nil
}

// executeGrep executes a grep search
func (h *MessageHandler) executeGrep(msg *workflow.Message) (string, error) {
	// Simple grep implementation
	// For now, just use bash grep
	cmd := fmt.Sprintf("grep -r '%s' %s", msg.Content, msg.Path)
	if msg.Path == "" {
		cmd = fmt.Sprintf("grep -r '%s' .", msg.Content)
	}

	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	return string(output), err
}
