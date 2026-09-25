package main

import (
	"os"
	"strings"
	"unicode"
)

// NexusCompleter implements readline.AutoCompleter for interactive command and @file completion.
type NexusCompleter struct {
	commands []string
	models   []string
}

// NewNexusCompleter creates a completer initialized with standard gateway commands and models.
func NewNexusCompleter() *NexusCompleter {
	return &NexusCompleter{
		commands: []string{
			"/help",
			"/model",
			"/auto",
			"/auto-apply",
			"/caveman",
			"/models",
			"/stats",
			"/usage",
			"/whoami",
			"/clear",
			"/exit",
			"/quit",
		},
		models: []string{
			"auto",
			"fast",
			"cheap",
			"smart",
			"quality",
			"gemini-3.6-flash",
			"gemini-2.5-pro",
			"gpt-4o",
			"gpt-4o-mini",
			"claude-3-5-sonnet-20241022",
			"claude-3-haiku-20240307",
		},
	}
}

// Do implements readline.AutoCompleter.
// It returns the completion candidate suffixes and the length of the matching prefix.
func (c *NexusCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	if pos < 0 || pos > len(line) {
		return nil, 0
	}

	lineStr := string(line[:pos])

	// Case 1: Slash command at the start of input
	if strings.HasPrefix(strings.TrimLeft(lineStr, " \t"), "/") {
		trimmedLead := strings.TrimLeft(lineStr, " \t")
		parts := strings.Split(trimmedLead, " ")

		// If typing the command name itself (e.g. "/m" or "/")
		if len(parts) == 1 {
			prefix := parts[0]
			var candidates [][]rune
			for _, cmd := range c.commands {
				if strings.HasPrefix(cmd, prefix) {
					suffix := cmd[len(prefix):]
					candidates = append(candidates, []rune(suffix))
				}
			}
			return candidates, len([]rune(prefix))
		}

		// If typing arguments for /model
		if parts[0] == "/model" && len(parts) >= 2 {
			argPrefix := parts[len(parts)-1]
			var candidates [][]rune
			for _, m := range c.models {
				if strings.HasPrefix(m, argPrefix) {
					suffix := m[len(argPrefix):]
					candidates = append(candidates, []rune(suffix))
				}
			}
			return candidates, len([]rune(argPrefix))
		}

		// If typing arguments for /auto, /auto-apply, or /caveman
		if (parts[0] == "/auto" || parts[0] == "/auto-apply" || parts[0] == "/caveman") && len(parts) >= 2 {
			argPrefix := parts[len(parts)-1]
			var candidates [][]rune
			for _, opt := range []string{"on", "off"} {
				if strings.HasPrefix(opt, argPrefix) {
					suffix := opt[len(argPrefix):]
					candidates = append(candidates, []rune(suffix))
				}
			}
			return candidates, len([]rune(argPrefix))
		}
	}

	// Case 2: File/Folder context completion with '@' anywhere in the line
	lastAtIdx := -1
	for i := pos - 1; i >= 0; i-- {
		r := line[i]
		if r == '@' {
			// Check that '@' is either at the beginning or preceded by whitespace
			if i == 0 || unicode.IsSpace(line[i-1]) {
				lastAtIdx = i
				break
			}
		} else if unicode.IsSpace(r) {
			// Stopped at previous word boundary without finding '@'
			break
		}
	}

	if lastAtIdx != -1 {
		// Extract what was typed after '@' up to pos
		typed := string(line[lastAtIdx+1 : pos])
		candidates, matchLen := completeFilePath(typed)
		return candidates, matchLen
	}

	return nil, 0
}

var ignoredDirs = map[string]bool{
	".git":         true,
	".gemini":      true,
	".nexus":       true,
	"node_modules": true,
	"bin":          true,
	"tmp":          true,
	"vendor":       true,
	".idea":        true,
	".vscode":      true,
}

// completeFilePath resolves file and directory suggestions given the partial path typed after '@'.
func completeFilePath(partial string) (candidates [][]rune, matchLen int) {
	// Normalize slashes for cross-platform matching
	normalized := strings.ReplaceAll(partial, "\\", "/")

	var searchDir string
	var filePrefix string

	lastSlash := strings.LastIndex(normalized, "/")
	if lastSlash == -1 {
		searchDir = "."
		filePrefix = normalized
	} else {
		searchDir = normalized[:lastSlash]
		if searchDir == "" {
			searchDir = "/"
		}
		filePrefix = normalized[lastSlash+1:]
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil, 0
	}

	var results [][]rune
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files unless user explicitly typed a dot
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(filePrefix, ".") {
			continue
		}
		if entry.IsDir() && ignoredDirs[name] {
			continue
		}

		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(filePrefix)) {
			candidateName := name
			if entry.IsDir() {
				candidateName += "/"
			}

			// Suffix to append after the prefix that was matched
			suffix := candidateName[len(filePrefix):]
			results = append(results, []rune(suffix))
		}
	}

	return results, len([]rune(filePrefix))
}