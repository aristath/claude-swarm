package workflow

import "time"

// Message represents a message from an agent to the orchestrator
type Message struct {
	ID          string      `json:"id"`
	Type        MessageType `json:"type"`
	Path        string      `json:"path,omitempty"`
	Content     string      `json:"content,omitempty"`
	Command     string      `json:"command,omitempty"`
	WorkingDir  string      `json:"working_dir,omitempty"`
	Edits       []Edit      `json:"edits,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
}

// MessageType represents the type of operation requested
type MessageType string

const (
	MessageTypeReadFile  MessageType = "read_file"
	MessageTypeWriteFile MessageType = "write_file"
	MessageTypeEditFile  MessageType = "edit_file"
	MessageTypeBash      MessageType = "bash"
	MessageTypeGlob      MessageType = "glob"
	MessageTypeGrep      MessageType = "grep"
)

// Edit represents a file edit operation
type Edit struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// Response represents the orchestrator's response to a message
type Response struct {
	MessageID string    `json:"message_id"`
	Status    string    `json:"status"`
	Data      string    `json:"data,omitempty"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// GlobRequest represents a glob pattern search request
type GlobRequest struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// GrepRequest represents a grep search request
type GrepRequest struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	OutputMode string `json:"output_mode,omitempty"` // content, files_with_matches, count
}
