package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ProposedEdit represents a file modification requested by the assistant
type ProposedEdit struct {
	FilePath string
	OldText  string
	NewText  string
	Action   string // "edit" | "write"
}

var editBlockRegex = regexp.MustCompile("(?s)```edit:([^\n\r]+)\n<<<<<<< SEARCH\n(.*?)\n=======\n(.*?)\n>>>>>>> REPLACE\n```")
var writeBlockRegex = regexp.MustCompile("(?s)```write:([^\n\r]+)\n(.*?)```")

// ParseProposedEdits extracts code modifications from model responses
func ParseProposedEdits(response string) []ProposedEdit {
	var edits []ProposedEdit

	// 1. Search/Replace edits
	editMatches := editBlockRegex.FindAllStringSubmatch(response, -1)
	for _, match := range editMatches {
		path := strings.TrimSpace(match[1])
		oldText := match[2]
		newText := match[3]
		edits = append(edits, ProposedEdit{
			FilePath: path,
			OldText:  oldText,
			NewText:  newText,
			Action:   "edit",
		})
	}

	// 2. Full file creations / overwrites
	writeMatches := writeBlockRegex.FindAllStringSubmatch(response, -1)
	for _, match := range writeMatches {
		path := strings.TrimSpace(match[1])
		content := match[2]
		edits = append(edits, ProposedEdit{
			FilePath: path,
			NewText:  content,
			Action:   "write",
		})
	}

	return edits
}

// ApplyEditsWithPolicy renders diffs and applies them subject to the active execution policy & safety guardrails.
func ApplyEditsWithPolicy(edits []ProposedEdit, policy ExecutionPolicy, taskDesc string) (appliedCount int, blockedCount int, errs []error) {
	if len(edits) == 0 {
		return 0, 0, nil
	}

	if policy == PolicyExplain {
		fmt.Printf("\n  %s %s: %d proposed file edit(s) were not applied.\n\n",
			yellow("[Policy: Explain]"), dim("Read-only mode active"), len(edits))
		return 0, len(edits), nil
	}

	if policy == PolicyPlan {
		fmt.Printf("\n  %s %s: Previewing %d planned modification(s) (no files touched):\n",
			cyan("[Policy: Plan]"), dim("Plan inspection mode"), len(edits))
		for i, edit := range edits {
			cleanPath := filepath.Clean(edit.FilePath)
			fmt.Printf("    %d. [%s] %s\n", i+1, strings.ToUpper(edit.Action), cleanPath)
		}
		fmt.Println()
		return 0, len(edits), nil
	}

	reader := bufio.NewReader(os.Stdin)
	var checkpointCreated bool

	for i, edit := range edits {
		cleanPath := filepath.Clean(edit.FilePath)
		isProtected, reason := IsProtectedFile(cleanPath)

		// Guardrail: in safe-auto, protected files are strictly blocked
		if isProtected && policy == PolicySafeAuto {
			fmt.Printf("\n  %s Skipped %s (%s). Use '/policy approve' to override.\n",
				red("[SAFETY BLOCK]"), bold(cleanPath), reason)
			blockedCount++
			continue
		}

		fmt.Printf("\n%s %s (%d/%d):\n", bold(cyan("Proposed Code Change on")), bold(cleanPath), i+1, len(edits))
		if isProtected {
			fmt.Printf("  %s %s (%s)\n", red("[WARNING: PROTECTED FILE]"), cleanPath, reason)
		}

		if edit.Action == "write" {
			fmt.Printf("  %s %s (%d bytes)\n", green("[CREATE / OVERWRITE]"), cleanPath, len(edit.NewText))
			previewLines := strings.Split(edit.NewText, "\n")
			maxLines := 8
			if len(previewLines) < maxLines {
				maxLines = len(previewLines)
			}
			for j := 0; j < maxLines; j++ {
				fmt.Printf("  %s %s\n", green("+"), previewLines[j])
			}
			if len(previewLines) > maxLines {
				fmt.Printf("  %s ... (%d more lines)\n", dim("+"), len(previewLines)-maxLines)
			}
		} else {
			fmt.Println("  --- Target chunk to replace:")
			for _, l := range strings.Split(edit.OldText, "\n") {
				fmt.Printf("  %s %s\n", red("-"), l)
			}
			fmt.Println("  +++ Replacement chunk:")
			for _, l := range strings.Split(edit.NewText, "\n") {
				fmt.Printf("  %s %s\n", green("+"), l)
			}
		}

		shouldApply := false
		switch policy {
		case PolicySafeAuto, PolicyAutopilot:
			shouldApply = true
			fmt.Printf("\n  %s Auto-applying change to %s...\n", green("->"), cleanPath)

		case PolicyApprove:
			promptText := fmt.Sprintf("Apply this change to %s? [Y/n/s (skip)]: ", cleanPath)
			if isProtected {
				promptText = fmt.Sprintf("CONFIRM modifying PROTECTED file %s? [y/N]: ", cleanPath)
			}
			fmt.Printf("\n  %s ", bold(yellow(promptText)))

			choice, _ := reader.ReadString('\n')
			choice = strings.ToLower(strings.TrimSpace(choice))
			if isProtected {
				if choice == "y" || choice == "yes" {
					shouldApply = true
				}
			} else {
				if choice == "" || choice == "y" || choice == "yes" {
					shouldApply = true
				}
			}
		}

		if shouldApply {
			// Auto Git Checkpoint before the very first modification in the batch
			if !checkpointCreated {
				if cp, cpErr := CreateGitCheckpoint(".", taskDesc); cpErr == nil && cp != nil {
					fmt.Printf("  %s Created checkpoint %s (use '/rollback' to revert)\n",
						dim("[Git Checkpoint]"), dim(cp.ID[:min(7, len(cp.ID))]))
					checkpointCreated = true
				}
			}

			if edit.Action == "write" {
				if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
					errs = append(errs, fmt.Errorf("failed creating parent dirs for %s: %v", cleanPath, err))
					continue
				}
				if err := os.WriteFile(cleanPath, []byte(edit.NewText), 0644); err != nil {
					errs = append(errs, fmt.Errorf("failed writing %s: %v", cleanPath, err))
					continue
				}
				fmt.Printf("  %s Successfully wrote %s\n", green("OK"), cleanPath)
				appliedCount++
			} else {
				data, err := os.ReadFile(cleanPath)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed reading target file %s: %v", cleanPath, err))
					continue
				}
				origContent := string(data)
				if !strings.Contains(origContent, edit.OldText) {
					errs = append(errs, fmt.Errorf("target chunk not found in %s; file may have changed", cleanPath))
					continue
				}
				patched := strings.Replace(origContent, edit.OldText, edit.NewText, 1)
				if err := os.WriteFile(cleanPath, []byte(patched), 0644); err != nil {
					errs = append(errs, fmt.Errorf("failed applying patch to %s: %v", cleanPath, err))
					continue
				}
				fmt.Printf("  %s Successfully patched %s\n", green("OK"), cleanPath)
				appliedCount++
			}
		} else {
			fmt.Printf("  %s Skipped %s\n", yellow("!"), cleanPath)
		}
	}

	return appliedCount, blockedCount, errs
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}