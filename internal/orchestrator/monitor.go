package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aristath/claude-swarm/internal/workflow"
	"github.com/fsnotify/fsnotify"
)

// FileMonitor watches agent directories for file changes
type FileMonitor struct {
	swarmDir string
	watcher  *fsnotify.Watcher
	events   chan workflow.FileEvent
	errors   chan error
	done     chan bool
}

// NewFileMonitor creates a new file monitor
func NewFileMonitor(swarmDir string) (*FileMonitor, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	return &FileMonitor{
		swarmDir: swarmDir,
		watcher:  watcher,
		events:   make(chan workflow.FileEvent, 100),
		errors:   make(chan error, 10),
		done:     make(chan bool),
	}, nil
}

// Start begins monitoring for file changes
func (m *FileMonitor) Start() error {
	// Watch all existing agent directories
	agentsDir := filepath.Join(m.swarmDir, "agents")
	if err := m.watchDirectory(agentsDir); err != nil {
		return fmt.Errorf("failed to watch agents directory: %w", err)
	}

	// Start the watch loop in a goroutine
	go m.watch()

	return nil
}

// Stop stops the file monitor
func (m *FileMonitor) Stop() {
	close(m.done)
	m.watcher.Close()
}

// Events returns the channel for file events
func (m *FileMonitor) Events() <-chan workflow.FileEvent {
	return m.events
}

// Errors returns the channel for errors
func (m *FileMonitor) Errors() <-chan error {
	return m.errors
}

// WatchAgentDir adds an agent directory to the watch list
func (m *FileMonitor) WatchAgentDir(agentDir string) error {
	return m.watchDirectory(agentDir)
}

// watchDirectory recursively watches a directory and all subdirectories
func (m *FileMonitor) watchDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Directory might not exist yet, ignore
			return nil
		}

		if info.IsDir() {
			if err := m.watcher.Add(path); err != nil {
				return fmt.Errorf("failed to watch %s: %w", path, err)
			}
		}

		return nil
	})
}

// watch is the main event loop
func (m *FileMonitor) watch() {
	for {
		select {
		case <-m.done:
			return

		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}

			// Only handle file creations
			if event.Op&fsnotify.Create == fsnotify.Create {
				m.handleCreate(event.Name)
			}

		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			m.errors <- err
		}
	}
}

// handleCreate handles a file creation event
func (m *FileMonitor) handleCreate(path string) {
	// Check if it's a directory - if so, watch it
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		m.watcher.Add(path)
		return
	}

	// Extract agent ID from path
	agentID := m.extractAgentID(path)
	if agentID == "" {
		return
	}

	// Determine event type based on file pattern
	eventType := m.detectEventType(path)
	if eventType == "" {
		return
	}

	// Send event
	m.events <- workflow.FileEvent{
		Type:     workflow.EventType(eventType),
		AgentID:  agentID,
		FilePath: path,
	}
}

// extractAgentID extracts the agent ID from a file path
func (m *FileMonitor) extractAgentID(path string) string {
	// Path format: .../agents/agent-<task-id>/...
	re := regexp.MustCompile(`/agents/agent-([^/]+)/`)
	matches := re.FindStringSubmatch(path)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// detectEventType determines the event type from the file path
func (m *FileMonitor) detectEventType(path string) string {
	filename := filepath.Base(path)

	switch {
	case strings.HasPrefix(filename, "q-") && strings.HasSuffix(filename, ".txt"):
		// questions/q-N.txt
		if strings.Contains(path, "/questions/") {
			return string(workflow.EventQuestionAsked)
		}
		// followup/q-N.txt
		if strings.Contains(path, "/followup/") {
			return string(workflow.EventFollowUpAsked)
		}

	case strings.HasPrefix(filename, "a-") && strings.HasSuffix(filename, ".txt"):
		// questions/a-N.txt
		if strings.Contains(path, "/questions/") {
			return string(workflow.EventQuestionAnswered)
		}
		// followup/a-N.txt
		if strings.Contains(path, "/followup/") {
			return string(workflow.EventFollowUpAnswered)
		}

	case filename == "COMPLETE":
		return string(workflow.EventTaskCompleted)

	case filename == "status.txt":
		return string(workflow.EventAgentStatusUpdate)

	case strings.HasPrefix(filename, "msg-") && strings.HasSuffix(filename, ".json"):
		// messages/msg-N.json
		if strings.Contains(path, "/messages/") {
			return string(workflow.EventFileOperationRequest)
		}
	}

	return ""
}
