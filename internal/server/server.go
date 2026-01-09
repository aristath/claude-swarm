package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aristath/claude-swarm/internal/state"
	"github.com/aristath/claude-swarm/internal/workflow"
)

// Server is the HTTP API server for agent communication
type Server struct {
	state      *state.SwarmState
	swarmDir   string
	httpServer *http.Server
}

// NewServer creates a new API server
func NewServer(swarmState *state.SwarmState, swarmDir string, port int) *Server {
	s := &Server{
		state:    swarmState,
		swarmDir: swarmDir,
	}

	mux := http.NewServeMux()

	// File operation endpoints
	mux.HandleFunc("/api/file/read", s.handleFileRead)
	mux.HandleFunc("/api/file/write", s.handleFileWrite)
	mux.HandleFunc("/api/file/edit", s.handleFileEdit)
	mux.HandleFunc("/api/bash", s.handleBash)
	mux.HandleFunc("/api/glob", s.handleGlob)
	mux.HandleFunc("/api/grep", s.handleGrep)

	// Agent communication endpoints
	mux.HandleFunc("/api/question", s.handleQuestion)
	mux.HandleFunc("/api/complete", s.handleComplete)

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return s
}

// Start starts the HTTP server
func (s *Server) Start() error {
	fmt.Printf("Starting API server on %s\n", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop() error {
	return s.httpServer.Close()
}

// Request/Response types

type FileReadRequest struct {
	Path string `json:"path"`
}

type FileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FileEditRequest struct {
	Path      string          `json:"path"`
	Edits     []workflow.Edit `json:"edits"`
	OldString string          `json:"old_string,omitempty"` // Single edit support
	NewString string          `json:"new_string,omitempty"`
}

type BashRequest struct {
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir,omitempty"`
}

type GlobRequest struct {
	Pattern string `json:"pattern"`
}

type GrepRequest struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Recursive  bool   `json:"recursive,omitempty"`
	IgnoreCase bool   `json:"ignore_case,omitempty"`
}

type QuestionRequest struct {
	AgentID  string `json:"agent_id"`
	Question string `json:"question"`
}

type CompleteRequest struct {
	AgentID string `json:"agent_id"`
	Output  string `json:"output"`
}

type APIResponse struct {
	Success bool   `json:"success"`
	Data    string `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Handlers

func (s *Server) handleFileRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FileReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(req.Path)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to read file: %v", err), http.StatusInternalServerError)
		return
	}

	s.jsonSuccess(w, string(content))
}

func (s *Server) handleFileWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FileWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(req.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to create directory: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(req.Path, []byte(req.Content), 0644); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to write file: %v", err), http.StatusInternalServerError)
		return
	}

	s.jsonSuccess(w, fmt.Sprintf("Wrote %d bytes to %s", len(req.Content), req.Path))
}

func (s *Server) handleFileEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FileEditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Support both single edit and multiple edits
	edits := req.Edits
	if len(edits) == 0 && req.OldString != "" {
		edits = []workflow.Edit{
			{
				OldString: req.OldString,
				NewString: req.NewString,
			},
		}
	}

	if len(edits) == 0 {
		s.jsonError(w, "No edits provided", http.StatusBadRequest)
		return
	}

	// Read file
	content, err := os.ReadFile(req.Path)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to read file: %v", err), http.StatusInternalServerError)
		return
	}

	result := string(content)

	// Apply each edit
	for i, edit := range edits {
		if !strings.Contains(result, edit.OldString) {
			s.jsonError(w, fmt.Sprintf("Edit %d: old_string not found in file", i+1), http.StatusBadRequest)
			return
		}
		result = strings.Replace(result, edit.OldString, edit.NewString, 1)
	}

	// Write back
	if err := os.WriteFile(req.Path, []byte(result), 0644); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to write file: %v", err), http.StatusInternalServerError)
		return
	}

	s.jsonSuccess(w, fmt.Sprintf("Applied %d edit(s) to %s", len(edits), req.Path))
}

func (s *Server) handleBash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	cmd := exec.Command("bash", "-c", req.Command)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Include output even on error
		s.jsonResponse(w, APIResponse{
			Success: false,
			Data:    string(output),
			Error:   err.Error(),
		})
		return
	}

	s.jsonSuccess(w, string(output))
}

func (s *Server) handleGlob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GlobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	matches, err := filepath.Glob(req.Pattern)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Glob failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.jsonSuccess(w, strings.Join(matches, "\n"))
}

func (s *Server) handleGrep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GrepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Build grep command
	args := []string{}
	if req.Recursive {
		args = append(args, "-r")
	}
	if req.IgnoreCase {
		args = append(args, "-i")
	}
	args = append(args, req.Pattern)
	if req.Path != "" {
		args = append(args, req.Path)
	} else {
		args = append(args, ".")
	}

	cmd := exec.Command("grep", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// grep returns error if no matches, but that's not really an error
		if len(output) == 0 {
			s.jsonSuccess(w, "")
			return
		}
	}

	s.jsonSuccess(w, string(output))
}

func (s *Server) handleQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req QuestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Add question to state
	qNum, err := s.state.AddQuestion(req.AgentID, req.Question)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to add question: %v", err), http.StatusInternalServerError)
		return
	}

	// For now, return a placeholder answer
	// In a real implementation, this would trigger orchestrator to formulate answer
	answer := fmt.Sprintf("Question %d received from agent %s. Orchestrator will process and answer.", qNum, req.AgentID)

	s.jsonSuccess(w, answer)
}

func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Mark task as complete
	if err := s.state.CompleteTask(req.AgentID, req.Output); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to complete task: %v", err), http.StatusInternalServerError)
		return
	}

	s.jsonSuccess(w, fmt.Sprintf("Task %s marked as complete", req.AgentID))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.jsonSuccess(w, "OK")
}

// Helper methods

func (s *Server) jsonSuccess(w http.ResponseWriter, data string) {
	s.jsonResponse(w, APIResponse{
		Success: true,
		Data:    data,
	})
}

func (s *Server) jsonError(w http.ResponseWriter, error string, statusCode int) {
	w.WriteHeader(statusCode)
	s.jsonResponse(w, APIResponse{
		Success: false,
		Error:   error,
	})
}

func (s *Server) jsonResponse(w http.ResponseWriter, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
