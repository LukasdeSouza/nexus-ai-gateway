package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ContextItem records a file or directory injected into prompt context.
type ContextItem struct {
	Path     string
	IsDir    bool
	Size     int64
	Lines    int
	Content  string
	Children int
}

var atMentionRegex = regexp.MustCompile(`(?:^|\s)@([a-zA-Z0-9_\-\./\\]+)`)

// ExpandPromptContext parses all @mentions in user input and appends file contents / directory listings.
func ExpandPromptContext(input string) (expandedPrompt string, items []ContextItem, errs []error) {
	matches := atMentionRegex.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input, nil, nil
	}

	seen := make(map[string]bool)
	var contextBlocks []string

	for _, match := range matches {
		rawPath := strings.TrimSpace(match[1])
		cleanPath := filepath.Clean(rawPath)

		if seen[cleanPath] {
			continue
		}
		seen[cleanPath] = true

		info, err := os.Stat(cleanPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("@%s not found: %v", rawPath, err))
			continue
		}

		if info.IsDir() {
			item, block, err := readDirectoryContext(cleanPath)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed reading directory @%s: %v", rawPath, err))
				continue
			}
			items = append(items, item)
			contextBlocks = append(contextBlocks, block)
		} else {
			item, block, err := readFileContext(cleanPath, info.Size())
			if err != nil {
				errs = append(errs, fmt.Errorf("failed reading file @%s: %v", rawPath, err))
				continue
			}
			items = append(items, item)
			contextBlocks = append(contextBlocks, block)
		}
	}

	if len(contextBlocks) == 0 {
		return input, items, errs
	}

	var sb strings.Builder
	sb.WriteString(input)
	sb.WriteString("\n\n---\n[Referenced Local Code Context]\n")
	for _, block := range contextBlocks {
		sb.WriteString(block)
		sb.WriteString("\n")
	}

	return sb.String(), items, errs
}

func readFileContext(path string, size int64) (ContextItem, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return ContextItem{}, "", err
	}
	defer file.Close()

	var sb strings.Builder
	normalizedPath := filepath.ToSlash(path)
	sb.WriteString(fmt.Sprintf("=== File: %s (%d bytes) ===\n", normalizedPath, size))

	scanner := bufio.NewScanner(file)
	lineNum := 1
	for scanner.Scan() {
		sb.WriteString(fmt.Sprintf("%4d | %s\n", lineNum, scanner.Text()))
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return ContextItem{}, "", err
	}

	item := ContextItem{
		Path:    normalizedPath,
		IsDir:   false,
		Size:    size,
		Lines:   lineNum - 1,
		Content: sb.String(),
	}

	return item, sb.String(), nil
}

func readDirectoryContext(path string) (ContextItem, string, error) {
	var sb strings.Builder
	normalizedPath := filepath.ToSlash(path)
	sb.WriteString(fmt.Sprintf("=== Directory Overview: %s/ ===\n", normalizedPath))

	childCount := 0
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if p == path {
			return nil
		}

		rel, _ := filepath.Rel(path, p)
		base := info.Name()
		if info.IsDir() && ignoredDirs[base] {
			return filepath.SkipDir
		}

		childCount++
		if info.IsDir() {
			sb.WriteString(fmt.Sprintf("  dir/  %s/\n", filepath.ToSlash(rel)))
		} else {
			sb.WriteString(fmt.Sprintf("  file  %-35s (%d bytes)\n", filepath.ToSlash(rel), info.Size()))
		}

		if childCount >= 60 {
			sb.WriteString("  ... (directory truncated at 60 items)\n")
			return filepath.SkipDir
		}
		return nil
	})

	if err != nil {
		return ContextItem{}, "", err
	}

	item := ContextItem{
		Path:     normalizedPath,
		IsDir:    true,
		Children: childCount,
		Content:  sb.String(),
	}

	return item, sb.String(), nil
}