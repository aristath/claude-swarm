package workflow

import "time"

// Workflow represents a complete workflow definition
type Workflow struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tasks       []Task `yaml:"tasks"`
}

// Task represents a single task in the workflow
type Task struct {
	ID          string   `yaml:"id"`
	AgentType   string   `yaml:"agent_type"`
	Description string   `yaml:"description"`
	Prompt      string   `yaml:"prompt"`
	DependsOn   []string `yaml:"depends_on"`
}

// TaskStatus represents the current status of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// AgentState represents the state of an agent working on a task
type AgentState struct {
	TaskID     string
	Status     TaskStatus
	StartedAt  time.Time
	Output     string
	Error      string
	Questions  []Question
	FollowUps  []FollowUp
	WorkingDir string
}

// Question represents a question asked by an agent to the orchestrator
type Question struct {
	ID         int
	Text       string
	AskedAt    time.Time
	Answer     string
	AnsweredAt time.Time
}

// FollowUp represents a follow-up question from orchestrator to agent
type FollowUp struct {
	ID         int
	Text       string
	AskedAt    time.Time
	Answer     string
	AnsweredAt time.Time
}

// Event types for file monitoring
type EventType string

const (
	EventQuestionAsked         EventType = "question_asked"
	EventQuestionAnswered      EventType = "question_answered"
	EventFollowUpAsked         EventType = "followup_asked"
	EventFollowUpAnswered      EventType = "followup_answered"
	EventTaskStarted           EventType = "task_started"
	EventTaskCompleted         EventType = "task_completed"
	EventTaskFailed            EventType = "task_failed"
	EventAgentStatusUpdate     EventType = "agent_status_update"
	EventFileOperationRequest  EventType = "file_operation_request"
)

// FileEvent represents a file system event detected by the monitor
type FileEvent struct {
	Type     EventType
	AgentID  string
	FilePath string
	Time     time.Time
}
