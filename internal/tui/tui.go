package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aristath/claude-swarm/internal/orchestrator"
	"github.com/aristath/claude-swarm/internal/server"
	"github.com/aristath/claude-swarm/internal/state"
	"github.com/aristath/claude-swarm/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

// AppMode represents the current application mode
type AppMode int

const (
	ModePlanning AppMode = iota
	ModeOrchestration
)

// MainModel is the top-level model that coordinates between phases
type MainModel struct {
	mode            AppMode
	sessionID       string
	swarmDir        string
	planningModel   PlanningModel
	orchestration   OrchestrationModel
	orchestratorSvc *orchestrator.Orchestrator
	apiServer       *server.Server
	ready           bool
}

// NewMainModel creates a new main TUI model
func NewMainModel(sessionID, swarmDir string) MainModel {
	return MainModel{
		mode:          ModePlanning,
		sessionID:     sessionID,
		swarmDir:      swarmDir,
		planningModel: NewPlanningModel(sessionID, swarmDir),
		ready:         false,
	}
}

func (m MainModel) Init() tea.Cmd {
	return m.planningModel.Init()
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.orchestratorSvc != nil {
				m.orchestratorSvc.Stop()
			}
			if m.apiServer != nil {
				m.apiServer.Stop()
			}
			return m, tea.Quit
		}

	case StartOrchestrationMsg:
		// Transition from planning to orchestration
		return m.startOrchestration()

	case OrchestratorReadyMsg:
		// Orchestrator is ready, show orchestration UI
		m.mode = ModeOrchestration
		m.orchestration = NewOrchestrationModel(m.sessionID, m.swarmDir, msg.State)
		return m, m.orchestration.Init()

	case OrchestratorEventMsg:
		// Forward to orchestration model
		if m.mode == ModeOrchestration {
			updated, cmd := m.orchestration.Update(msg)
			m.orchestration = updated.(OrchestrationModel)
			return m, cmd
		}
	}

	// Delegate to active model
	switch m.mode {
	case ModePlanning:
		updated, cmd := m.planningModel.Update(msg)
		m.planningModel = updated.(PlanningModel)
		return m, cmd

	case ModeOrchestration:
		updated, cmd := m.orchestration.Update(msg)
		m.orchestration = updated.(OrchestrationModel)
		return m, cmd
	}

	return m, nil
}

func (m MainModel) View() string {
	switch m.mode {
	case ModePlanning:
		return m.planningModel.View()
	case ModeOrchestration:
		return m.orchestration.View()
	default:
		return "Unknown mode"
	}
}

func (m MainModel) startOrchestration() (tea.Model, tea.Cmd) {
	// Load workflow
	workflowPath := filepath.Join(m.swarmDir, "workflow.yaml")
	parser := workflow.NewParser()
	wf, err := parser.ParseFile(workflowPath)
	if err != nil {
		return m, func() tea.Msg {
			return ErrorMsg{Err: fmt.Errorf("failed to load workflow: %w", err)}
		}
	}

	// Load plan
	planPath := filepath.Join(m.swarmDir, "plan.md")
	planData, err := os.ReadFile(planPath)
	if err != nil {
		return m, func() tea.Msg {
			return ErrorMsg{Err: fmt.Errorf("failed to load plan: %w", err)}
		}
	}

	// Create state
	swarmState := state.NewSwarmState(m.sessionID, string(planData), wf)

	// Create orchestrator
	orch, err := orchestrator.NewOrchestrator(m.swarmDir, swarmState)
	if err != nil {
		return m, func() tea.Msg {
			return ErrorMsg{Err: fmt.Errorf("failed to create orchestrator: %w", err)}
		}
	}

	m.orchestratorSvc = orch

	// Create API server on port 8080
	apiServer := server.NewServer(swarmState, m.swarmDir, 8080)
	m.apiServer = apiServer

	// Start API server in background
	go func() {
		if err := apiServer.Start(); err != nil {
			fmt.Printf("API server error: %v\n", err)
		}
	}()

	// Start orchestrator in background
	go func() {
		if err := orch.Run(); err != nil {
			fmt.Printf("Orchestrator error: %v\n", err)
		}
	}()

	// Notify that orchestrator is ready
	return m, func() tea.Msg {
		return OrchestratorReadyMsg{State: swarmState}
	}
}

// Custom messages
type OrchestratorReadyMsg struct {
	State *state.SwarmState
}

// Run starts the TUI application
func Run(sessionID, swarmDir string) error {
	model := NewMainModel(sessionID, swarmDir)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err := p.Run()
	return err
}
