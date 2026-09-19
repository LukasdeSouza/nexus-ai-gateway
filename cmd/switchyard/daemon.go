package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type remoteTaskResponse struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"project_id"`
	Prompt    string  `json:"prompt"`
	Preset    string  `json:"preset"`
	Policy    string  `json:"policy"`
	Budget    float64 `json:"budget"`
	Status    string  `json:"status"`
}

// StartDaemonWorker starts the background daemon worker that polls for web-dispatched remote tasks.
func StartDaemonWorker(projectID, gatewayURL string) error {
	creds, _ := loadCredentials()
	if gatewayURL == "" && creds != nil && creds.BaseURL != "" {
		gatewayURL = creds.BaseURL
	}
	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080"
	}
	gatewayURL = strings.TrimRight(gatewayURL, "/")

	if projectID == "" && creds != nil {
		projectID = creds.ProjectID
	}

	if projectID == "" {
		return fmt.Errorf("project_id is required for daemon worker. Run 'switchyard login' first or pass -project <id>")
	}

	hostname, _ := os.Hostname()
	workerID := "cli-daemon-" + hostname

	fmt.Println()
	fmt.Println(bold(cyan("  ========================================================")))
	fmt.Println(bold(cyan("    SWITCHYARD DAEMON  -  Remote Task Relay Active       ")))
	fmt.Println(bold(cyan("  ========================================================")))
	fmt.Printf("  Gateway:   %s\n", cyan(gatewayURL))
	fmt.Printf("  Project:   %s\n", cyan(projectID))
	fmt.Printf("  Worker:    %s\n", dim(workerID))
	fmt.Println(dim("  Status:    Listening for tasks dispatched from Web UI..."))
	fmt.Println(dim("  --------------------------------------------------------"))
	fmt.Println(dim("  Press Ctrl+C to stop daemon worker"))
	fmt.Println(dim("  --------------------------------------------------------"))
	fmt.Println()

	client := &http.Client{Timeout: 10 * time.Second}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println()
			fmt.Println(yellow("  Daemon worker shutting down cleanly."))
			return nil

		case <-ticker.C:
			_, _ = PollAndExecuteSingleTask(client, gatewayURL, projectID, workerID)
		}
	}
}

// PollAndExecuteSingleTask checks for pending remote tasks and executes one if available.
func PollAndExecuteSingleTask(client *http.Client, gatewayURL, projectID, workerID string) (bool, error) {
	pendingTasks, err := fetchPendingTasks(client, gatewayURL, projectID)
	if err != nil || len(pendingTasks) == 0 {
		return false, err
	}

	task := pendingTasks[0]
	if err := claimTask(client, gatewayURL, task.ID, workerID); err != nil {
		return false, err
	}

	fmt.Println(bold(yellow(fmt.Sprintf("\n  [Cloud Task Received] \"%s\"", task.Prompt))))
	fmt.Printf("  Task ID: %s - Preset: %s - Policy: %s\n", dim(task.ID), cyan(task.Preset), cyan(task.Policy))

	policy, err := ParsePolicy(task.Policy)
	if err != nil {
		policy = PolicyApprove
	}

	summary, diff, failed := executeRemoteTaskLocally(task.Preset, policy, task.Budget, task.Prompt)

	if completeErr := reportTaskCompletion(client, gatewayURL, task.ID, summary, diff, failed); completeErr != nil {
		fmt.Printf("  %s Failed to sync task completion: %v\n", red("[Error]"), completeErr)
		return true, completeErr
	}
	fmt.Println(bold(green("  [Cloud Task Completed] Results & diff synced to Web Dashboard.")))
	return true, nil
}

func fetchPendingTasks(client *http.Client, gatewayURL, projectID string) ([]remoteTaskResponse, error) {
	reqURL := fmt.Sprintf("%s/v1/tasks/pending?project_id=%s", gatewayURL, url.QueryEscape(projectID))
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}

	var payload struct {
		Tasks []remoteTaskResponse `json:"tasks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Tasks, nil
}

func claimTask(client *http.Client, gatewayURL, taskID, workerID string) error {
	reqURL := fmt.Sprintf("%s/v1/tasks/%s/claim", gatewayURL, taskID)
	body, _ := json.Marshal(map[string]string{"worker_id": workerID})
	resp, err := client.Post(reqURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("claim failed status %d", resp.StatusCode)
	}
	return nil
}

func reportTaskCompletion(client *http.Client, gatewayURL, taskID, summary, diff string, failed bool) error {
	reqURL := fmt.Sprintf("%s/v1/tasks/%s/complete", gatewayURL, taskID)
	body, _ := json.Marshal(map[string]interface{}{
		"summary": summary,
		"diff":    diff,
		"failed":  failed,
	})
	resp, err := client.Post(reqURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("complete failed status %d", resp.StatusCode)
	}
	return nil
}

func executeRemoteTaskLocally(preset string, policy ExecutionPolicy, budget float64, rawPrompt string) (summary string, diff string, failed bool) {
	expandedPrompt, items, _ := ExpandPromptContext(rawPrompt)

	history := []map[string]string{
		{"role": "user", "content": expandedPrompt},
	}

	creds, _ := loadCredentials()
	apiKey := os.Getenv("SWITCHYARD_API_KEY")
	if apiKey == "" && creds != nil {
		apiKey = creds.APIKey
	}
	baseURL := os.Getenv("SWITCHYARD_BASE_URL")
	if baseURL == "" && creds != nil && creds.BaseURL != "" {
		baseURL = creds.BaseURL
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	res, err := sendChatConversation(baseURL, apiKey, preset, history, true, policy, budget)
	if err != nil {
		return fmt.Sprintf("Execution error: %v", err), "", true
	}

	edits := ParseProposedEdits(res.Content)
	if len(edits) > 0 {
		var diffBuilder strings.Builder
		for _, ed := range edits {
			diffBuilder.WriteString(fmt.Sprintf("--- %s\n+++ %s\n%s\n\n", ed.FilePath, ed.FilePath, ed.NewText))
		}

		appliedCount, blockedCount, errs := ApplyEditsWithPolicy(edits, policy, rawPrompt)
		if len(errs) > 0 {
			return fmt.Sprintf("Applied %d edits, %d blocked, error: %v", appliedCount, blockedCount, errs[0]), diffBuilder.String(), true
		}

		_ = items
		return fmt.Sprintf("Applied %d file modifications cleanly (%d blocked).", appliedCount, blockedCount), diffBuilder.String(), false
	}

	return res.Content, "", false
}
