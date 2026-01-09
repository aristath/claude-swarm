package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aristath/claude-swarm/internal/workflow"
)

// Persistence handles saving and loading swarm state
type Persistence struct {
	stateFile string
}

// NewPersistence creates a new persistence handler
func NewPersistence(swarmDir string) *Persistence {
	return &Persistence{
		stateFile: filepath.Join(swarmDir, "state.json"),
	}
}

// Save saves the swarm state to disk
func (p *Persistence) Save(state *SwarmState) error {
	state.mu.RLock()
	defer state.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write atomically by writing to temp file then renaming
	tmpFile := p.stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if err := os.Rename(tmpFile, p.stateFile); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// Load loads the swarm state from disk
func (p *Persistence) Load() (*SwarmState, error) {
	data, err := os.ReadFile(p.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("state file not found")
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state SwarmState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// Initialize maps if they're nil
	if state.Agents == nil {
		state.Agents = make(map[string]*workflow.AgentState)
	}
	if state.outputsCache == nil {
		state.outputsCache = make(map[string]string)
	}

	return &state, nil
}

// Exists checks if a state file exists
func (p *Persistence) Exists() bool {
	_, err := os.Stat(p.stateFile)
	return err == nil
}
