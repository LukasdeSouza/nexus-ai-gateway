package main

import (
	"os"
	"strings"
	"unicode"
)

// SwitchyardCompleter implements readline.AutoCompleter for interactive command, preset, policy, and @file completion.
type SwitchyardCompleter struct {
	commands []string
	policies []string
	presets  []string
	models   []string
}

// NewSwitchyardCompleter creates a completer initialized with Switchyard commands, policies, and presets.
func NewSwitchyardCompleter() *SwitchyardCompleter {
	return &SwitchyardCompleter{
		commands: []string{
			"/help",
			"/policy",
			"/preset",
			"/model",
			"/rollback",
			"/stats",
			"/caveman",
			"/models",
			"/usage",
			"/whoami",
			"/clear",
			"/exit",
			"/quit",
		},
		policies: []string{
			"explain",
			"plan",
			"approve",
			"safe-auto",
			"autopilot",
		},
		presets: []string{
			"auto",
			"explore",
			"build",
			"reason",
			"review",
		},
		models: []string{
			"auto",
			"explore",
			"build",
			"reason",
			"review",
			"gemini-3.7-flash",
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
func (c *SwitchyardCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	if pos < 0 || pos > len(line) {
		return nil, 0
	}

	lineStr := string(line[:pos])

	// Case 1: Slash commands
	if strings.HasPrefix(strings.TrimLeft(lineStr, " \t"), "/") {
		trimmedLead := strings.TrimLeft(lineStr, " \t")
		parts := strings.Split(trimmedLead, " ")

		// Typing command name
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

		// Typing arguments for /policy
		if (parts[0] == "/policy" || parts[0] == "/pol") && len(parts) >= 2 {
			argPrefix := parts[len(parts)-1]
			var candidates [][]rune
			for _, pol := range c.policies {
				if strings.HasPrefix(pol, argPrefix) {
					suffix := pol[len(argPrefix):]
					candidates = append(candidates, []rune(suffix))
				}
			}
			return candidates, len([]rune(argPrefix))
		}

		// Typing arguments for /preset
		if parts[0] == "/preset" && len(parts) >= 2 {
			argPrefix := parts[len(parts)-1]
			var candidates [][]rune
			for _, pre := range c.presets {
				if strings.HasPrefix(pre, argPrefix) {
					suffix := pre[len(argPrefix):]
					candidates = append(candidates, []rune(suffix))
				}
			}
			return candidates, len([]rune(argPrefix))
		}

		// Typing arguments for /model
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

		// Typing arguments for /caveman
		if parts[0] == "/caveman" && len(parts) >= 2 {
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

	// Case 2: File/Directory autocomplete with '@'
	lastAtIdx := -1
	for i := pos - 1; i >= 0; i-- {
		r := line[i]
		if r == '@' {
			if i == 0 || unicode.IsSpace(line[i-1]) {
				lastAtIdx = i
				break
			}
		} else if unicode.IsSpace(r) {
			break
		}
	}

	if lastAtIdx != -1 {
		typed := string(line[lastAtIdx+1 : pos])
		return completeFilePath(typed)
	}

	return nil, 0
}

var ignoredDirs = map[string]bool{
	".git":         true,
	".gemini":      true,
	".nexus":       true,
	".switchyard":  true,
	"node_modules": true,
	"bin":          true,
	"tmp":          true,
	"vendor":       true,
	".idea":        true,
	".vscode":      true,
}

func completeFilePath(partial string) (candidates [][]rune, matchLen int) {
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
			suffix := candidateName[len(filePrefix):]
			results = append(results, []rune(suffix))
		}
	}

	return results, len([]rune(filePrefix))
}