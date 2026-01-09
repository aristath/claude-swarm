package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aristath/claude-swarm/internal/state"
	"github.com/aristath/claude-swarm/internal/workflow"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// OrchestrationModel handles the orchestration phase with split-screen layout
type OrchestrationModel struct {
	sessionID        string
	swarmDir         string
	state            *state.SwarmState
	mainViewport     viewport.Model
	sidebarViewport  viewport.Model
	width            int
	height           int
	focusedPane      PaneType
	lastUpdate       time.Time
}

// PaneType represents which pane is focused
type PaneType int

const (
	OrchestratorPane PaneType = iota
	AgentSidebarPane
)

// NewOrchestrationModel creates a new orchestration model
func NewOrchestrationModel(sessionID, swarmDir string, swarmState *state.SwarmState) OrchestrationModel {
	mainVP := viewport.New(80, 30)
	sideVP := viewport.New(30, 30)

	return OrchestrationModel{
		sessionID:       sessionID,
		swarmDir:        swarmDir,
		state:           swarmState,
		mainViewport:    mainVP,
		sidebarViewport: sideVP,
		focusedPane:     OrchestratorPane,
		lastUpdate:      time.Now(),
	}
}

func (m OrchestrationModel) Init() tea.Cmd {
	return tea.Batch(
		m.tick(),
	)
}

func (m OrchestrationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "Q":
			return m, tea.Quit

		case "tab":
			// Switch focused pane
			if m.focusedPane == OrchestratorPane {
				m.focusedPane = AgentSidebarPane
			} else {
				m.focusedPane = OrchestratorPane
			}
			return m, nil

		case "r", "R":
			// Refresh view
			m.updateViewports()
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Split 70/30
		mainWidth := int(float64(msg.Width) * 0.70)
		sideWidth := msg.Width - mainWidth - 6 // Account for borders and padding

		m.mainViewport.Width = mainWidth - 4
		m.mainViewport.Height = msg.Height - 8

		m.sidebarViewport.Width = sideWidth - 4
		m.sidebarViewport.Height = msg.Height - 8

		m.updateViewports()
		return m, nil

	case TickMsg:
		// Periodic update
		m.lastUpdate = time.Time(msg)
		m.updateViewports()
		return m, m.tick()

	case OrchestratorEventMsg:
		// Handle orchestrator events
		m.updateViewports()
		return m, nil
	}

	// Update viewports based on focused pane
	if m.focusedPane == OrchestratorPane {
		m.mainViewport, cmd = m.mainViewport.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		m.sidebarViewport, cmd = m.sidebarViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m OrchestrationModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Calculate split widths (70/30)
	mainWidth := int(float64(m.width) * 0.70)
	sideWidth := m.width - mainWidth - 2

	// Header
	header := m.renderHeader()

	// Main orchestrator view
	mainContent := m.renderOrchestratorView(mainWidth)

	// Agent sidebar
	sideContent := m.renderAgentSidebar(sideWidth)

	// Style the panes
	mainBorder := lipgloss.RoundedBorder()
	sideBorder := lipgloss.RoundedBorder()

	mainColor := lipgloss.Color("63")
	sideColor := lipgloss.Color("205")

	// Highlight focused pane
	if m.focusedPane == OrchestratorPane {
		mainColor = lipgloss.Color("cyan")
	} else {
		sideColor = lipgloss.Color("cyan")
	}

	mainStyle := lipgloss.NewStyle().
		Width(mainWidth).
		Height(m.height - 4).
		Border(mainBorder).
		BorderForeground(mainColor).
		Padding(1, 2)

	sideStyle := lipgloss.NewStyle().
		Width(sideWidth).
		Height(m.height - 4).
		Border(sideBorder).
		BorderForeground(sideColor).
		Padding(1, 2)

	// Render panes
	main := mainStyle.Render(mainContent)
	side := sideStyle.Render(sideContent)

	// Combine horizontally
	body := lipgloss.JoinHorizontal(lipgloss.Top, main, side)

	// Help footer
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *OrchestrationModel) renderHeader() string {
	title := fmt.Sprintf("Claude Swarm - %s", m.state.Workflow.Name)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(0, 2)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	progress := m.state.GetProgress()
	info := fmt.Sprintf("Session: %s | Progress: %.0f%% | Updated: %s",
		m.sessionID,
		progress,
		m.lastUpdate.Format("15:04:05"))

	return lipgloss.JoinVertical(lipgloss.Left,
		headerStyle.Render(title),
		infoStyle.Render(info),
	)
}

func (m *OrchestrationModel) renderOrchestratorView(width int) string {
	var content strings.Builder

	// Progress bar
	progress := m.state.GetProgress()
	progressBar := m.renderProgressBar(progress, width-8)
	content.WriteString(progressBar)
	content.WriteString("\n\n")

	// Task list
	content.WriteString(lipgloss.NewStyle().Bold(true).Render("Tasks:"))
	content.WriteString("\n")
	content.WriteString(m.renderTaskList())
	content.WriteString("\n\n")

	// Recent events
	content.WriteString(lipgloss.NewStyle().Bold(true).Render("Recent Events:"))
	content.WriteString("\n")
	content.WriteString(m.renderEventLog(8))

	m.mainViewport.SetContent(content.String())
	return m.mainViewport.View()
}

func (m *OrchestrationModel) renderAgentSidebar(width int) string {
	var content strings.Builder

	// Active agents section
	activeAgents := m.state.GetActiveAgents()
	content.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render(fmt.Sprintf("Active Agents (%d)", len(activeAgents))))
	content.WriteString("\n\n")

	if len(activeAgents) == 0 {
		content.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			Render("No active agents"))
		content.WriteString("\n\n")
	} else {
		for _, agent := range activeAgents {
			content.WriteString(m.renderAgentCard(agent))
			content.WriteString("\n")
		}
	}

	// Recent Q&A section
	content.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		Render("Recent Q&A"))
	content.WriteString("\n\n")
	content.WriteString(m.renderRecentQuestions(5))

	m.sidebarViewport.SetContent(content.String())
	return m.sidebarViewport.View()
}

func (m *OrchestrationModel) renderProgressBar(progress float64, width int) string {
	filled := int((progress / 100.0) * float64(width))
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("green")).
		Render(fmt.Sprintf("Progress: [%s] %.0f%%", bar, progress))
}

func (m *OrchestrationModel) renderTaskList() string {
	var tasks strings.Builder

	for _, task := range m.state.Workflow.Tasks {
		agent := m.state.GetAgent(task.ID)

		var status string
		var icon string
		var color lipgloss.Color

		if agent == nil {
			status = "pending"
			icon = "⋯"
			color = lipgloss.Color("240")
		} else {
			switch agent.Status {
			case workflow.TaskStatusRunning:
				status = "running"
				icon = "⧗"
				color = lipgloss.Color("yellow")
				elapsed := time.Since(agent.StartedAt).Round(time.Second)
				status = fmt.Sprintf("running (%s)", elapsed)
			case workflow.TaskStatusCompleted:
				status = "completed"
				icon = "✓"
				color = lipgloss.Color("green")
			case workflow.TaskStatusFailed:
				status = "failed"
				icon = "✗"
				color = lipgloss.Color("red")
			default:
				status = "unknown"
				icon = "?"
				color = lipgloss.Color("240")
			}
		}

		line := lipgloss.NewStyle().
			Foreground(color).
			Render(fmt.Sprintf("  %s %-15s [%s]", icon, task.ID, status))

		tasks.WriteString(line)
		tasks.WriteString("\n")
	}

	return tasks.String()
}

func (m *OrchestrationModel) renderEventLog(count int) string {
	events := m.state.GetRecentEvents(count)

	var log strings.Builder

	for _, event := range events {
		timestamp := event.Time.Format("15:04:05")
		var icon string
		var color lipgloss.Color

		switch event.Type {
		case workflow.EventTaskStarted:
			icon = "▶"
			color = lipgloss.Color("cyan")
		case workflow.EventTaskCompleted:
			icon = "✓"
			color = lipgloss.Color("green")
		case workflow.EventQuestionAsked:
			icon = "💬"
			color = lipgloss.Color("yellow")
		case workflow.EventQuestionAnswered:
			icon = "💡"
			color = lipgloss.Color("green")
		default:
			icon = "•"
			color = lipgloss.Color("240")
		}

		line := lipgloss.NewStyle().
			Foreground(color).
			Render(fmt.Sprintf("%s [%s] %s: %s", icon, timestamp, event.AgentID, event.Type))

		log.WriteString(line)
		log.WriteString("\n")
	}

	return log.String()
}

func (m *OrchestrationModel) renderAgentCard(agent *workflow.AgentState) string {
	elapsed := time.Since(agent.StartedAt).Round(time.Second)

	var statusIcon string
	var statusColor lipgloss.Color

	switch agent.Status {
	case workflow.TaskStatusRunning:
		statusIcon = "⧗"
		statusColor = lipgloss.Color("yellow")
	case workflow.TaskStatusCompleted:
		statusIcon = "✓"
		statusColor = lipgloss.Color("green")
	case workflow.TaskStatusFailed:
		statusIcon = "✗"
		statusColor = lipgloss.Color("red")
	default:
		statusIcon = "⋯"
		statusColor = lipgloss.Color("240")
	}

	card := fmt.Sprintf("%s %s\n  Started: %s ago\n  Questions: %d",
		statusIcon,
		agent.TaskID,
		elapsed,
		len(agent.Questions))

	return lipgloss.NewStyle().
		Foreground(statusColor).
		Render(card)
}

func (m *OrchestrationModel) renderRecentQuestions(count int) string {
	var questions strings.Builder

	questionCount := 0
	for _, agent := range m.state.Agents {
		for _, q := range agent.Questions {
			if questionCount >= count {
				break
			}

			qText := q.Text
			if len(qText) > 50 {
				qText = qText[:50] + "..."
			}

			aText := q.Answer
			if len(aText) > 50 {
				aText = aText[:50] + "..."
			}

			qa := fmt.Sprintf("%s → orchestrator\nQ: %s\nA: %s\n",
				agent.TaskID,
				qText,
				aText)

			questions.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render(qa))
			questions.WriteString("\n")

			questionCount++
		}
	}

	if questionCount == 0 {
		questions.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			Render("No questions yet"))
	}

	return questions.String()
}

func (m *OrchestrationModel) renderFooter() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(1, 2)

	return helpStyle.Render("[Tab] Switch pane | [R] Refresh | [Q] Quit")
}

func (m *OrchestrationModel) updateViewports() {
	// Trigger re-render
	m.lastUpdate = time.Now()
}

func (m OrchestrationModel) tick() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Custom messages
type TickMsg time.Time
type OrchestratorEventMsg struct {
	Event workflow.FileEvent
}
