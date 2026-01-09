package state

import (
	"fmt"
	"sync"
	"time"

	"github.com/aristath/claude-swarm/internal/workflow"
)

// SwarmState represents the complete state of a swarm orchestration session
type SwarmState struct {
	SessionID       string
	Plan            string
	Workflow        *workflow.Workflow
	Agents          map[string]*workflow.AgentState
	CompletedTasks  []string
	Events          []workflow.FileEvent
	StartedAt       time.Time
	CompletedAt     *time.Time
	mu              sync.RWMutex
	outputsCache    map[string]string // Cache of task outputs
}

// NewSwarmState creates a new swarm state
func NewSwarmState(sessionID string, plan string, wf *workflow.Workflow) *SwarmState {
	return &SwarmState{
		SessionID:    sessionID,
		Plan:         plan,
		Workflow:     wf,
		Agents:       make(map[string]*workflow.AgentState),
		CompletedTasks: []string{},
		Events:       []workflow.FileEvent{},
		StartedAt:    time.Now(),
		outputsCache: make(map[string]string),
	}
}

// AddAgent adds a new agent to the state
func (s *SwarmState) AddAgent(taskID, workingDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Agents[taskID]; exists {
		return fmt.Errorf("agent for task %s already exists", taskID)
	}

	s.Agents[taskID] = &workflow.AgentState{
		TaskID:     taskID,
		Status:     workflow.TaskStatusRunning,
		StartedAt:  time.Now(),
		Questions:  []workflow.Question{},
		FollowUps:  []workflow.FollowUp{},
		WorkingDir: workingDir,
	}

	s.addEvent(workflow.EventTaskStarted, taskID, "")

	return nil
}

// CompleteTask marks a task as completed
func (s *SwarmState) CompleteTask(taskID, output string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, exists := s.Agents[taskID]
	if !exists {
		return fmt.Errorf("agent for task %s not found", taskID)
	}

	agent.Status = workflow.TaskStatusCompleted
	agent.Output = output

	s.CompletedTasks = append(s.CompletedTasks, taskID)
	s.outputsCache[taskID] = output

	s.addEvent(workflow.EventTaskCompleted, taskID, "")

	return nil
}

// FailTask marks a task as failed
func (s *SwarmState) FailTask(taskID, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, exists := s.Agents[taskID]
	if !exists {
		return fmt.Errorf("agent for task %s not found", taskID)
	}

	agent.Status = workflow.TaskStatusFailed
	agent.Error = errorMsg

	s.addEvent(workflow.EventTaskFailed, taskID, "")

	return nil
}

// AddQuestion adds a question from an agent
func (s *SwarmState) AddQuestion(taskID, questionText string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, exists := s.Agents[taskID]
	if !exists {
		return 0, fmt.Errorf("agent for task %s not found", taskID)
	}

	qID := len(agent.Questions) + 1
	question := workflow.Question{
		ID:      qID,
		Text:    questionText,
		AskedAt: time.Now(),
	}

	agent.Questions = append(agent.Questions, question)

	s.addEvent(workflow.EventQuestionAsked, taskID, "")

	return qID, nil
}

// AnswerQuestion adds an answer to a question
func (s *SwarmState) AnswerQuestion(taskID string, qID int, answer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, exists := s.Agents[taskID]
	if !exists {
		return fmt.Errorf("agent for task %s not found", taskID)
	}

	if qID < 1 || qID > len(agent.Questions) {
		return fmt.Errorf("question %d not found for task %s", qID, taskID)
	}

	agent.Questions[qID-1].Answer = answer
	agent.Questions[qID-1].AnsweredAt = time.Now()

	s.addEvent(workflow.EventQuestionAnswered, taskID, "")

	return nil
}

// GetReadyTasks returns tasks that are ready to be spawned
func (s *SwarmState) GetReadyTasks() []workflow.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ready := []workflow.Task{}

	// Check which tasks have all dependencies completed
	for _, task := range s.Workflow.Tasks {
		// Skip if already spawned
		if _, exists := s.Agents[task.ID]; exists {
			continue
		}

		// Check if all dependencies are completed
		allDepsCompleted := true
		for _, depID := range task.DependsOn {
			if !s.isTaskCompleted(depID) {
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted {
			ready = append(ready, task)
		}
	}

	return ready
}

// isTaskCompleted checks if a task is completed (must be called with lock held)
func (s *SwarmState) isTaskCompleted(taskID string) bool {
	for _, completedID := range s.CompletedTasks {
		if completedID == taskID {
			return true
		}
	}
	return false
}

// GetTask returns a task by ID
func (s *SwarmState) GetTask(taskID string) *workflow.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, task := range s.Workflow.Tasks {
		if task.ID == taskID {
			return &task
		}
	}
	return nil
}

// GetAgent returns an agent state by task ID
func (s *SwarmState) GetAgent(taskID string) *workflow.AgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Agents[taskID]
}

// GetActiveAgents returns all agents that are currently running
func (s *SwarmState) GetActiveAgents() []*workflow.AgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	active := []*workflow.AgentState{}
	for _, agent := range s.Agents {
		if agent.Status == workflow.TaskStatusRunning {
			active = append(active, agent)
		}
	}

	return active
}

// GetOutputs returns a map of task ID to output
func (s *SwarmState) GetOutputs() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	outputs := make(map[string]string)
	for taskID, output := range s.outputsCache {
		outputs[taskID] = output
	}

	return outputs
}

// IsComplete checks if all tasks are completed
func (s *SwarmState) IsComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.CompletedTasks) == len(s.Workflow.Tasks)
}

// MarkComplete marks the entire workflow as complete
func (s *SwarmState) MarkComplete() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.CompletedAt = &now
}

// GetProgress returns the completion percentage (0-100)
func (s *SwarmState) GetProgress() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Workflow.Tasks) == 0 {
		return 100.0
	}

	return float64(len(s.CompletedTasks)) / float64(len(s.Workflow.Tasks)) * 100.0
}

// addEvent adds an event to the event log (must be called with lock held)
func (s *SwarmState) addEvent(eventType workflow.EventType, agentID string, filePath string) {
	event := workflow.FileEvent{
		Type:     eventType,
		AgentID:  agentID,
		FilePath: filePath,
		Time:     time.Now(),
	}

	s.Events = append(s.Events, event)
}

// GetRecentEvents returns the N most recent events
func (s *SwarmState) GetRecentEvents(n int) []workflow.FileEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Events) <= n {
		result := make([]workflow.FileEvent, len(s.Events))
		copy(result, s.Events)
		return result
	}

	result := make([]workflow.FileEvent, n)
	copy(result, s.Events[len(s.Events)-n:])
	return result
}
