package tui

import (
	"fmt"
	"regexp"
	"strings"
)

// WorkflowGenerator generates workflow YAML from plan text
type WorkflowGenerator struct{}

// NewWorkflowGenerator creates a new workflow generator
func NewWorkflowGenerator() *WorkflowGenerator {
	return &WorkflowGenerator{}
}

// GenerateFromPlan generates a workflow YAML from plan text
func (g *WorkflowGenerator) GenerateFromPlan(plan string) (string, error) {
	// Extract tasks from plan
	tasks := g.extractTasks(plan)

	if len(tasks) == 0 {
		// No explicit tasks found, create a simple default workflow
		return g.generateDefaultWorkflow(plan), nil
	}

	// Build workflow YAML
	return g.buildWorkflowYAML(tasks), nil
}

// Task represents an extracted task from the plan
type Task struct {
	ID          string
	Description string
	Prompt      string
	AgentType   string
	DependsOn   []string
}

// extractTasks extracts tasks from plan text
func (g *WorkflowGenerator) extractTasks(plan string) []Task {
	tasks := []Task{}

	// Look for task patterns:
	// - Task N: ...
	// - ### Task N: ...
	// - N. ...
	// - Task: ... (named tasks)

	lines := strings.Split(plan, "\n")
	var currentTask *Task

	taskIDCounter := 1

	// Patterns to match
	taskHeaderRe := regexp.MustCompile(`(?i)^[\s\-\#]*(?:task\s*(\d+)|(\d+)\.)\s*:?\s*(.*)$`)
	namedTaskRe := regexp.MustCompile(`(?i)^[\s\-\#]*task\s*:?\s*(.*)$`)

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Check for numbered task
		if matches := taskHeaderRe.FindStringSubmatch(line); matches != nil {
			// Save previous task
			if currentTask != nil {
				tasks = append(tasks, *currentTask)
			}

			// Extract task number or use description
			taskNum := matches[1]
			if taskNum == "" {
				taskNum = matches[2]
			}
			description := strings.TrimSpace(matches[3])

			// Create new task
			taskID := fmt.Sprintf("task%d", taskIDCounter)
			taskIDCounter++

			currentTask = &Task{
				ID:          taskID,
				Description: description,
				Prompt:      g.buildPromptFromContext(lines, i),
				AgentType:   g.inferAgentType(description),
				DependsOn:   g.inferDependencies(taskID, tasks),
			}

			continue
		}

		// Check for named task (e.g., "Task: Analyze codebase")
		if matches := namedTaskRe.FindStringSubmatch(line); matches != nil {
			// Save previous task
			if currentTask != nil {
				tasks = append(tasks, *currentTask)
			}

			description := strings.TrimSpace(matches[1])
			taskID := g.slugify(description)

			currentTask = &Task{
				ID:          taskID,
				Description: description,
				Prompt:      g.buildPromptFromContext(lines, i),
				AgentType:   g.inferAgentType(description),
				DependsOn:   g.inferDependencies(taskID, tasks),
			}

			continue
		}

		// Add content to current task
		if currentTask != nil && line != "" {
			if currentTask.Prompt != "" {
				currentTask.Prompt += "\n"
			}
			currentTask.Prompt += line
		}
	}

	// Save last task
	if currentTask != nil {
		tasks = append(tasks, *currentTask)
	}

	return tasks
}

// buildPromptFromContext builds a prompt from surrounding context
func (g *WorkflowGenerator) buildPromptFromContext(lines []string, startIdx int) string {
	// Collect lines after task header until next task or empty lines
	prompt := strings.Builder{}

	for i := startIdx + 1; i < len(lines) && i < startIdx+10; i++ {
		line := strings.TrimSpace(lines[i])

		// Stop at next task header
		if regexp.MustCompile(`(?i)^[\s\-\#]*(?:task|^\d+\.)`).MatchString(line) {
			break
		}

		// Stop at multiple empty lines
		if line == "" && prompt.Len() > 0 {
			continue
		}

		if line != "" {
			prompt.WriteString(line)
			prompt.WriteString("\n")
		}
	}

	return strings.TrimSpace(prompt.String())
}

// inferAgentType infers agent type from task description
func (g *WorkflowGenerator) inferAgentType(description string) string {
	desc := strings.ToLower(description)

	switch {
	case strings.Contains(desc, "analyze") || strings.Contains(desc, "find") || strings.Contains(desc, "search") || strings.Contains(desc, "explore"):
		return "Explore"
	case strings.Contains(desc, "plan") || strings.Contains(desc, "design") || strings.Contains(desc, "architect"):
		return "Plan"
	case strings.Contains(desc, "test"):
		return "general-purpose" // Could be test-runner if we add that
	default:
		return "general-purpose"
	}
}

// inferDependencies infers dependencies based on task order
func (g *WorkflowGenerator) inferDependencies(taskID string, previousTasks []Task) []string {
	// Simple heuristic: each task depends on the previous one
	if len(previousTasks) > 0 {
		return []string{previousTasks[len(previousTasks)-1].ID}
	}
	return []string{}
}

// slugify converts a description to a slug
func (g *WorkflowGenerator) slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

// buildWorkflowYAML builds the workflow YAML from tasks
func (g *WorkflowGenerator) buildWorkflowYAML(tasks []Task) string {
	var yaml strings.Builder

	yaml.WriteString("name: \"Generated Workflow\"\n")
	yaml.WriteString("description: \"Auto-generated from planning discussion\"\n\n")
	yaml.WriteString("tasks:\n")

	for _, task := range tasks {
		yaml.WriteString(fmt.Sprintf("  - id: \"%s\"\n", task.ID))
		yaml.WriteString(fmt.Sprintf("    agent_type: \"%s\"\n", task.AgentType))
		yaml.WriteString(fmt.Sprintf("    description: \"%s\"\n", task.Description))
		yaml.WriteString("    prompt: |\n")

		// Indent prompt
		promptLines := strings.Split(task.Prompt, "\n")
		for _, line := range promptLines {
			yaml.WriteString(fmt.Sprintf("      %s\n", line))
		}

		// Dependencies
		yaml.WriteString("    depends_on: [")
		if len(task.DependsOn) > 0 {
			for i, dep := range task.DependsOn {
				yaml.WriteString(fmt.Sprintf("\"%s\"", dep))
				if i < len(task.DependsOn)-1 {
					yaml.WriteString(", ")
				}
			}
		}
		yaml.WriteString("]\n\n")
	}

	return yaml.String()
}

// generateDefaultWorkflow generates a simple workflow when no tasks are detected
func (g *WorkflowGenerator) generateDefaultWorkflow(plan string) string {
	return fmt.Sprintf(`name: "Simple Workflow"
description: "Single task workflow"

tasks:
  - id: "main"
    agent_type: "general-purpose"
    description: "Execute the plan"
    prompt: |
      %s
    depends_on: []
`, strings.TrimSpace(plan))
}
