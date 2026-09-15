package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var atMentionRegex = regexp.MustCompile(`@([a-zA-Z0-9_\-\.\/\\]+)`)

// ContextItem represents an expanded file or directory
type ContextItem struct {
	Path     string
	IsDir    bool
	Content  string
	LineCount int
	SizeBytes int64
}

// ExpandPromptContext scans user input for @filepath or @dirpath mentions and embeds the file contents
func ExpandPromptContext(input string) (expandedPrompt string, items []ContextItem, errs []error) {
	matches := atMentionRegex.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input, nil, nil
	}

	seen := make(map[string]bool)
	var loadedContexts []string

	for _, match := range matches {
		rawPath := match[1]
		cleanPath := filepath.Clean(rawPath)

		if seen[cleanPath] {
			continue
		}
		seen[cleanPath] = true

		info, err := os.Stat(cleanPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not read '@%s': %v", rawPath, err))
			continue
		}

		if info.IsDir() {
			// List directory overview
			dirListing, err := formatDirContext(cleanPath)
			if err != nil {
				errs = append(errs, fmt.Errorf("error reading directory '@%s': %v", rawPath, err))
				continue
			}
			loadedContexts = append(loadedContexts, dirListing)
			items = append(items, ContextItem{
				Path:      cleanPath,
				IsDir:     true,
				Content:   dirListing,
				SizeBytes: info.Size(),
			})
		} else {
			// Read file content
			// Skip binary or huge files (> 500KB)
			if info.Size() > 500*1024 {
				errs = append(errs, fmt.Errorf("file '@%s' is too large (>500KB) for inline context", rawPath))
				continue
			}

			data, err := os.ReadFile(cleanPath)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed reading '@%s': %v", rawPath, err))
				continue
			}

			lines := strings.Split(string(data), "\n")
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("--- File: %s (%d lines, %d bytes) ---\n", filepath.ToSlash(cleanPath), len(lines), info.Size()))
			for i, line := range lines {
				sb.WriteString(fmt.Sprintf("%4d | %s\n", i+1, line))
			}
			sb.WriteString(fmt.Sprintf("--- End of File: %s ---\n", filepath.ToSlash(cleanPath)))

			loadedContexts = append(loadedContexts, sb.String())
			items = append(items, ContextItem{
				Path:      cleanPath,
				IsDir:     false,
				Content:   sb.String(),
				LineCount: len(lines),
				SizeBytes: info.Size(),
			})
		}
	}

	if len(loadedContexts) == 0 {
		return input, items, errs
	}

	var finalPrompt strings.Builder
	finalPrompt.WriteString("[Referenced Local Code Context]:\n\n")
	for _, ctxStr := range loadedContexts {
		finalPrompt.WriteString(ctxStr)
		finalPrompt.WriteString("\n\n")
	}
	finalPrompt.WriteString("[User Instruction]:\n")
	finalPrompt.WriteString(input)

	return finalPrompt.String(), items, errs
}

func formatDirContext(dirPath string) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- Directory Listing: %s ---\n", filepath.ToSlash(dirPath)))

	count := 0
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip .git and hidden folders
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules" || info.Name() == ".gemini") {
			return filepath.SkipDir
		}
		if path == dirPath {
			return nil
		}
		rel, _ := filepath.Rel(dirPath, path)
		if info.IsDir() {
			sb.WriteString(fmt.Sprintf("  [dir]  %s/\n", filepath.ToSlash(rel)))
		} else {
			sb.WriteString(fmt.Sprintf("  [file] %s (%d bytes)\n", filepath.ToSlash(rel), info.Size()))
		}
		count++
		if count > 80 {
			sb.WriteString("  ... (truncated remaining files)\n")
			return filepath.SkipDir
		}
		return nil
	})

	sb.WriteString(fmt.Sprintf("--- End of Directory: %s ---\n", filepath.ToSlash(dirPath)))
	return sb.String(), err
}