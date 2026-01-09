package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PlanningMode represents the planning phase state
type PlanningMode int

const (
	ModeDiscussion PlanningMode = iota
	ModeReviewPlan
	ModeGeneratingWorkflow
	ModeReady
)

// Message represents a conversation message
type Message struct {
	Author  string
	Content string
	Time    time.Time
}

// PlanningModel handles the interactive planning phase
type PlanningModel struct {
	sessionID   string
	swarmDir    string
	mode        PlanningMode
	messages    []Message
	plan        strings.Builder
	viewport    viewport.Model
	textarea    textarea.Model
	width       int
	height      int
	ready       bool
	workflowGen *WorkflowGenerator
}

// NewPlanningModel creates a new planning model
func NewPlanningModel(sessionID, swarmDir string) PlanningModel {
	ta := textarea.New()
	ta.Placeholder = "Type your message here..."
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.CharLimit = 5000

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return PlanningModel{
		sessionID:   sessionID,
		swarmDir:    swarmDir,
		mode:        ModeDiscussion,
		messages:    []Message{},
		textarea:    ta,
		viewport:    vp,
		workflowGen: NewWorkflowGenerator(),
	}
}

func (m PlanningModel) Init() tea.Cmd {
	// Show welcome message
	welcome := Message{
		Author:  "System",
		Content: "Welcome to Claude Swarm Planning Mode!\n\nLet's discuss your plan together. I'll help you break down the work into tasks.\n\nTell me: What would you like to accomplish?",
		Time:    time.Now(),
	}
	m.messages = append(m.messages, welcome)
	m.updateViewport()

	return textarea.Blink
}

func (m PlanningModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "ctrl+d":
			// Save and exit to next phase
			if m.mode == ModeDiscussion {
				m.savePlan()
				m.mode = ModeReviewPlan
				m.addSystemMessage("Plan saved! Press [G] to generate workflow, [E] to continue editing, [Q] to quit.")
			}
			return m, nil

		case "g", "G":
			// Generate workflow
			if m.mode == ModeReviewPlan {
				m.mode = ModeGeneratingWorkflow
				m.addSystemMessage("Generating workflow from plan...")
				return m, m.generateWorkflow()
			}
			return m, nil

		case "e", "E":
			// Return to editing
			if m.mode == ModeReviewPlan {
				m.mode = ModeDiscussion
				m.addSystemMessage("Continuing discussion. Press Ctrl+D when ready to review.")
			}
			return m, nil

		case "s", "S":
			// Start orchestration
			if m.mode == ModeReady {
				return m, func() tea.Msg {
					return StartOrchestrationMsg{}
				}
			}
			return m, nil

		case "q", "Q":
			return m, tea.Quit

		case "enter":
			// Send message
			if m.mode == ModeDiscussion {
				userMsg := strings.TrimSpace(m.textarea.Value())
				if userMsg != "" {
					m.addUserMessage(userMsg)
					m.textarea.Reset()

					// Add to plan
					m.plan.WriteString(userMsg)
					m.plan.WriteString("\n\n")

					// Prompt for Claude's response
					m.addSystemMessage("[CLAUDE A] Please respond to the user's message, helping them plan their workflow.")
				}
			}
			return m, nil

		case "tab":
			// Add tab spacing for better formatting
			if m.mode == ModeDiscussion {
				m.textarea.InsertString("    ")
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 8
		m.textarea.SetWidth(msg.Width - 4)
		m.updateViewport()
		return m, nil

	case WorkflowGeneratedMsg:
		m.mode = ModeReady
		m.addSystemMessage(fmt.Sprintf("Workflow generated successfully!\n\nWorkflow: %s\nTasks: %d\n\nPress [S] to start orchestration, [Q] to quit.", msg.Path, msg.TaskCount))
		return m, nil
	}

	// Update sub-models
	if m.mode == ModeDiscussion {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m PlanningModel) View() string {
	var s strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(1, 2).
		Render(fmt.Sprintf("Claude Swarm - %s", m.getModeString()))

	s.WriteString(header)
	s.WriteString("\n")

	// Session info
	info := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(fmt.Sprintf("Session: %s | Directory: %s", m.sessionID, m.swarmDir))
	s.WriteString(info)
	s.WriteString("\n\n")

	// Conversation viewport
	viewportStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 2)

	s.WriteString(viewportStyle.Render(m.viewport.View()))
	s.WriteString("\n\n")

	// Input area (only in discussion mode)
	if m.mode == ModeDiscussion {
		inputLabel := lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Render("Your message (Enter to send, Ctrl+D to finish planning):")
		s.WriteString(inputLabel)
		s.WriteString("\n")

		textareaStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))

		s.WriteString(textareaStyle.Render(m.textarea.View()))
	}

	// Help
	s.WriteString("\n\n")
	help := m.getHelpText()
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(help)
	s.WriteString(helpStyle)

	return s.String()
}

func (m *PlanningModel) addUserMessage(content string) {
	msg := Message{
		Author:  "You",
		Content: content,
		Time:    time.Now(),
	}
	m.messages = append(m.messages, msg)
	m.updateViewport()
}

func (m *PlanningModel) addSystemMessage(content string) {
	msg := Message{
		Author:  "System",
		Content: content,
		Time:    time.Now(),
	}
	m.messages = append(m.messages, msg)
	m.updateViewport()
}

func (m *PlanningModel) updateViewport() {
	var content strings.Builder

	for _, msg := range m.messages {
		timestamp := msg.Time.Format("15:04:05")

		var style lipgloss.Style
		switch msg.Author {
		case "You":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("cyan")).
				Bold(true)
		case "Claude A":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("green")).
				Bold(true)
		case "System":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("yellow")).
				Italic(true)
		default:
			style = lipgloss.NewStyle()
		}

		header := style.Render(fmt.Sprintf("[%s] %s:", timestamp, msg.Author))
		content.WriteString(header)
		content.WriteString("\n")
		content.WriteString(msg.Content)
		content.WriteString("\n\n")
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

func (m *PlanningModel) savePlan() error {
	planFile := filepath.Join(m.swarmDir, "plan.md")

	planContent := fmt.Sprintf("# Plan\n\n%s\n", m.plan.String())

	if err := os.WriteFile(planFile, []byte(planContent), 0644); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	return nil
}

func (m *PlanningModel) generateWorkflow() tea.Cmd {
	return func() tea.Msg {
		// Generate workflow from plan
		workflow, err := m.workflowGen.GenerateFromPlan(m.plan.String())
		if err != nil {
			return ErrorMsg{Err: err}
		}

		// Save workflow
		workflowFile := filepath.Join(m.swarmDir, "workflow.yaml")
		if err := os.WriteFile(workflowFile, []byte(workflow), 0644); err != nil {
			return ErrorMsg{Err: err}
		}

		// Count tasks (simple count)
		taskCount := strings.Count(workflow, "- id:")

		return WorkflowGeneratedMsg{
			Path:      workflowFile,
			TaskCount: taskCount,
		}
	}
}

func (m PlanningModel) getModeString() string {
	switch m.mode {
	case ModeDiscussion:
		return "Planning Discussion"
	case ModeReviewPlan:
		return "Review Plan"
	case ModeGeneratingWorkflow:
		return "Generating Workflow"
	case ModeReady:
		return "Ready to Orchestrate"
	default:
		return "Unknown"
	}
}

func (m PlanningModel) getHelpText() string {
	switch m.mode {
	case ModeDiscussion:
		return "Ctrl+D: Finish planning | Ctrl+C: Quit"
	case ModeReviewPlan:
		return "[G] Generate workflow | [E] Continue editing | [Q] Quit"
	case ModeReady:
		return "[S] Start orchestration | [Q] Quit"
	default:
		return "[Q] Quit"
	}
}

// Custom messages
type StartOrchestrationMsg struct{}
type WorkflowGeneratedMsg struct {
	Path      string
	TaskCount int
}
type ErrorMsg struct {
	Err error
}
