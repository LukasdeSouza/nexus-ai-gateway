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

// PromptAndApplyEdits renders diffs and applies them automatically (if autoApply=true) or prompts the user
func PromptAndApplyEdits(edits []ProposedEdit, autoApply bool) (appliedCount int, errs []error) {
	if len(edits) == 0 {
		return 0, nil
	}

	reader := bufio.NewReader(os.Stdin)

	for i, edit := range edits {
		cleanPath := filepath.Clean(edit.FilePath)
		fmt.Printf("\n%s %s (%d/%d):\n", bold(cyan("Proposed Code Change on")), bold(cleanPath), i+1, len(edits))

		if edit.Action == "write" {
			fmt.Printf("  %s %s (%d bytes)\n", green("[CREATE / OVERWRITE]"), cleanPath, len(edit.NewText))
			previewLines := strings.Split(edit.NewText, "\n")
			maxLines := 10
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
			// Search & Replace diff view
			fmt.Println("  --- Target chunk to replace:")
			for _, l := range strings.Split(edit.OldText, "\n") {
				fmt.Printf("  %s %s\n", red("-"), l)
			}
			fmt.Println("  +++ Replacement chunk:")
			for _, l := range strings.Split(edit.NewText, "\n") {
				fmt.Printf("  %s %s\n", green("+"), l)
			}
		}

		shouldApply := autoApply
		if !autoApply {
			fmt.Printf("\n  %s %s [Y/n/s (skip)]: ", bold(yellow("Apply this change to disk?")), cleanPath)
			choice, _ := reader.ReadString('\n')
			choice = strings.ToLower(strings.TrimSpace(choice))
			if choice == "" || choice == "y" || choice == "yes" {
				shouldApply = true
			}
		} else {
			fmt.Printf("\n  %s Auto-applying change to %s...\n", green("->"), cleanPath)
		}

		if shouldApply {
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
				// Read existing file and perform exact replacement
				data, err := os.ReadFile(cleanPath)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed reading %s: %v", cleanPath, err))
					continue
				}
				content := string(data)
				if !strings.Contains(content, edit.OldText) {
					errs = append(errs, fmt.Errorf("target snippet not found in %s (file may have changed)", cleanPath))
					fmt.Printf("  %s Target snippet not found in %s\n", red("FAILED"), cleanPath)
					continue
				}
				newContent := strings.Replace(content, edit.OldText, edit.NewText, 1)
				if err := os.WriteFile(cleanPath, []byte(newContent), 0644); err != nil {
					errs = append(errs, fmt.Errorf("failed updating %s: %v", cleanPath, err))
					continue
				}
				fmt.Printf("  %s Successfully applied patch to %s\n", green("OK"), cleanPath)
				appliedCount++
			}
		} else {
			fmt.Printf("  %s Skipped change for %s\n", dim("->"), cleanPath)
		}
	}

	return appliedCount, errs
}