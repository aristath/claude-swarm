package workflow

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parser handles parsing and validation of workflow files
type Parser struct{}

// NewParser creates a new workflow parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseFile reads and parses a workflow YAML file
func (p *Parser) ParseFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	return p.Parse(data)
}

// Parse parses workflow YAML data
func (p *Parser) Parse(data []byte) (*Workflow, error) {
	var workflow Workflow

	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	if err := p.Validate(&workflow); err != nil {
		return nil, fmt.Errorf("workflow validation failed: %w", err)
	}

	return &workflow, nil
}

// Validate validates a workflow definition
func (p *Parser) Validate(workflow *Workflow) error {
	if workflow.Name == "" {
		return fmt.Errorf("workflow name is required")
	}

	if len(workflow.Tasks) == 0 {
		return fmt.Errorf("workflow must have at least one task")
	}

	// Validate task IDs are unique
	taskIDs := make(map[string]bool)
	for _, task := range workflow.Tasks {
		if task.ID == "" {
			return fmt.Errorf("task ID is required")
		}

		if taskIDs[task.ID] {
			return fmt.Errorf("duplicate task ID: %s", task.ID)
		}
		taskIDs[task.ID] = true

		if task.Prompt == "" {
			return fmt.Errorf("task %s: prompt is required", task.ID)
		}

		// Validate dependencies exist
		for _, depID := range task.DependsOn {
			if !taskIDs[depID] && !p.taskExistsInList(depID, workflow.Tasks) {
				return fmt.Errorf("task %s: dependency %s not found", task.ID, depID)
			}
		}
	}

	// Check for circular dependencies
	if err := p.checkCircularDependencies(workflow); err != nil {
		return err
	}

	return nil
}

// taskExistsInList checks if a task ID exists in the task list
func (p *Parser) taskExistsInList(taskID string, tasks []Task) bool {
	for _, task := range tasks {
		if task.ID == taskID {
			return true
		}
	}
	return false
}

// checkCircularDependencies detects circular dependencies in the workflow
func (p *Parser) checkCircularDependencies(workflow *Workflow) error {
	// Build dependency graph
	deps := make(map[string][]string)
	for _, task := range workflow.Tasks {
		deps[task.ID] = task.DependsOn
	}

	// Check each task for circular dependencies
	for taskID := range deps {
		if err := p.hasCircularDep(taskID, deps, make(map[string]bool), []string{}); err != nil {
			return err
		}
	}

	return nil
}

// hasCircularDep performs DFS to detect circular dependencies
func (p *Parser) hasCircularDep(taskID string, deps map[string][]string, visited map[string]bool, path []string) error {
	if visited[taskID] {
		// Found a cycle
		cycle := append(path, taskID)
		return fmt.Errorf("circular dependency detected: %s", strings.Join(cycle, " -> "))
	}

	visited[taskID] = true
	path = append(path, taskID)

	for _, depID := range deps[taskID] {
		if err := p.hasCircularDep(depID, deps, visited, path); err != nil {
			return err
		}
	}

	visited[taskID] = false
	return nil
}

// InterpolatePrompt replaces {task-id.output} variables with actual outputs
func (p *Parser) InterpolatePrompt(prompt string, outputs map[string]string) string {
	result := prompt

	for taskID, output := range outputs {
		placeholder := fmt.Sprintf("{%s.output}", taskID)
		result = strings.ReplaceAll(result, placeholder, output)
	}

	return result
}
