package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// TaskStatus represents the lifecycle state of a remote task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusClaimed   TaskStatus = "claimed"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// RemoteTask represents a web-dispatched remote task in the task queue.
type RemoteTask struct {
	ID            string     `json:"id"`
	ProjectID     string     `json:"project_id"`
	Prompt        string     `json:"prompt"`
	Preset        string     `json:"preset"`
	Policy        string     `json:"policy"`
	Budget        float64    `json:"budget"`
	Status        TaskStatus `json:"status"`
	ResultSummary string     `json:"result_summary,omitempty"`
	Diff          string     `json:"diff,omitempty"`
	ClaimedBy     string     `json:"claimed_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// NewRemoteTask creates a new RemoteTask domain instance.
func NewRemoteTask(projectID, prompt, preset, policy string, budget float64) *RemoteTask {
	now := time.Now().UTC()
	if preset == "" {
		preset = "auto"
	}
	if policy == "" {
		policy = "approve"
	}
	return &RemoteTask{
		ID:        "task_" + uuid.New().String(),
		ProjectID: projectID,
		Prompt:    strings.TrimSpace(prompt),
		Preset:    preset,
		Policy:    policy,
		Budget:    budget,
		Status:    TaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Validate checks task invariants.
func (t *RemoteTask) Validate() error {
	if strings.TrimSpace(t.ProjectID) == "" {
		return New(CodeBadRequest, "project_id is required")
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return New(CodeBadRequest, "task prompt cannot be empty")
	}
	return nil
}
