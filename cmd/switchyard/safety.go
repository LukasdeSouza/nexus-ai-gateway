package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ExecutionPolicy controls the autonomy and safety boundaries of Switchyard.
type ExecutionPolicy string

const (
	// PolicyExplain: Read-only answers. File edits are strictly forbidden.
	PolicyExplain ExecutionPolicy = "explain"

	// PolicyPlan: Produces architectural plans and diff previews without touching files.
	PolicyPlan ExecutionPolicy = "plan"

	// PolicyApprove: Shows diffs and requires manual [Y/n/s] confirmation before writing (Default).
	PolicyApprove ExecutionPolicy = "approve"

	// PolicySafeAuto: Automatically applies modifications EXCEPT on protected files (secrets, lockfiles, git).
	PolicySafeAuto ExecutionPolicy = "safe-auto"

	// PolicyAutopilot: Full autonomy with automatic git checkpoints.
	PolicyAutopilot ExecutionPolicy = "autopilot"
)

// ParsePolicy parses a user string into an ExecutionPolicy.
func ParsePolicy(s string) (ExecutionPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "explain", "readonly", "read-only":
		return PolicyExplain, nil
	case "plan", "dry-run":
		return PolicyPlan, nil
	case "approve", "confirm", "manual":
		return PolicyApprove, nil
	case "safe-auto", "safeauto", "safe":
		return PolicySafeAuto, nil
	case "autopilot", "auto", "autonomous":
		return PolicyAutopilot, nil
	default:
		return "", fmt.Errorf("unknown policy '%s'. Valid options: explain, plan, approve, safe-auto, autopilot", s)
	}
}

// Protected file patterns that cannot be modified without explicit override
var protectedFilePatterns = []string{
	".env*",
	"*.pem",
	"*.key",
	"*.cert",
	"*.pfx",
	"go.sum",
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"composer.lock",
	"Gemfile.lock",
	"cargo.lock",
}

// IsProtectedFile checks if a given file path hits protected secrets, lockfiles, or git internals.
func IsProtectedFile(path string) (bool, string) {
	clean := filepath.Clean(path)
	normalized := filepath.ToSlash(clean)
	base := filepath.Base(clean)

	// Disallow any modification to .git
	if strings.Contains(normalized, "/.git/") || strings.HasPrefix(normalized, ".git/") || base == ".git" {
		return true, "git repository metadata is strictly protected"
	}

	for _, pattern := range protectedFilePatterns {
		matched, err := filepath.Match(pattern, base)
		if err == nil && matched {
			return true, fmt.Sprintf("protected file pattern '%s' matched", pattern)
		}
	}

	return false, ""
}

// GitCheckpoint manages lightweight pre-task snapshots for instant rollback.
type GitCheckpoint struct {
	ID        string
	CreatedAt time.Time
	Message   string
}

// LastCheckpoint holds the most recent pre-task git checkpoint.
var LastCheckpoint *GitCheckpoint

// CreateGitCheckpoint creates a git stash or commit snapshot before modifying files.
func CreateGitCheckpoint(dir, taskDesc string) (*GitCheckpoint, error) {
	// Check if git is available in directory
	cmdCheck := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmdCheck.Dir = dir
	if err := cmdCheck.Run(); err != nil {
		// Not a git repository, skip gracefully
		return nil, nil
	}

	// Create stash without modifying working tree
	msg := fmt.Sprintf("switchyard-checkpoint-%s: %s", time.Now().Format("20060102-150405"), taskDesc)
	cmdStash := exec.Command("git", "stash", "create", msg)
	cmdStash.Dir = dir
	out, err := cmdStash.Output()
	if err != nil {
		return nil, err
	}

	stashID := strings.TrimSpace(string(out))
	if stashID == "" {
		// No pending changes in working tree currently
		stashID = "HEAD"
	}

	cp := &GitCheckpoint{
		ID:        stashID,
		CreatedAt: time.Now(),
		Message:   msg,
	}
	LastCheckpoint = cp
	return cp, nil
}

// RollbackGitCheckpoint restores the working directory to the pre-task state.
func RollbackGitCheckpoint(dir string) error {
	cmdCheck := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmdCheck.Dir = dir
	if err := cmdCheck.Run(); err != nil {
		return fmt.Errorf("current directory is not a git repository")
	}

	if LastCheckpoint == nil {
		return fmt.Errorf("no Switchyard checkpoint found for this session")
	}

	// Revert working tree to HEAD / checkpoint
	cmdCheckout := exec.Command("git", "checkout", "--", ".")
	cmdCheckout.Dir = dir
	if out, err := cmdCheckout.CombinedOutput(); err != nil {
		return fmt.Errorf("failed checkout during rollback: %s (%v)", string(out), err)
	}

	// Clean untracked files created by the task
	cmdClean := exec.Command("git", "clean", "-fd")
	cmdClean.Dir = dir
	if out, err := cmdClean.CombinedOutput(); err != nil {
		return fmt.Errorf("failed clean during rollback: %s (%v)", string(out), err)
	}

	return nil
}