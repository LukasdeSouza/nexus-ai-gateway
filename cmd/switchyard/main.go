// Command switchyard provides the routing, cost, and safety layer for terminal-based AI coding agents.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
)

// ANSI color helpers
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorMagenta = "\033[35m"
)

func bold(s string) string    { return colorBold + s + colorReset }
func green(s string) string   { return colorGreen + s + colorReset }
func cyan(s string) string    { return colorCyan + s + colorReset }
func yellow(s string) string  { return colorYellow + s + colorReset }
func dim(s string) string     { return colorDim + s + colorReset }
func red(s string) string     { return colorRed + s + colorReset }
func magenta(s string) string { return colorMagenta + s + colorReset }

// Model pricing table (per 1M tokens, USD)
type ModelPricing struct {
	Display     string
	Provider    string
	InputPer1M  float64
	OutputPer1M float64
}

var modelPricingTable = map[string]ModelPricing{
	"auto":                       {"dynamic", "Switchyard Router", 0.075, 0.30},
	"explore":                    {"gemini-3.6-flash", "Google", 0.075, 0.30},
	"build":                      {"claude-3-5-sonnet", "Anthropic", 3.00, 15.00},
	"reason":                     {"claude-3-5-sonnet", "Anthropic", 3.00, 15.00},
	"review":                     {"claude-3-5-sonnet", "Anthropic", 3.00, 15.00},
	"smart":                      {"gpt-4o", "OpenAI", 2.50, 10.00},
	"gpt-4o":                     {"gpt-4o", "OpenAI", 2.50, 10.00},
	"cheap":                      {"gpt-4o-mini", "OpenAI", 0.15, 0.60},
	"gpt-4o-mini":                {"gpt-4o-mini", "OpenAI", 0.15, 0.60},
	"quality":                    {"claude-3-5-sonnet", "Anthropic", 3.00, 15.00},
	"claude-3-5-sonnet-20241022": {"claude-3-5-sonnet", "Anthropic", 3.00, 15.00},
	"claude-3-haiku-20240307":    {"claude-3-haiku", "Anthropic", 0.25, 1.25},
	"fast":                       {"gemini-3.6-flash", "Google", 0.075, 0.30},
	"gemini-3.6-flash":           {"gemini-3.6-flash", "Google", 0.075, 0.30},
	"gemini-2.0-flash":           {"gemini-2.0-flash", "Google", 0.075, 0.30},
	"gemini-1.5-flash":           {"gemini-1.5-flash", "Google", 0.075, 0.30},
	"gemini-3.7-flash":           {"gemini-3.7-flash", "Google", 0.15, 0.60},
	"gemini-2.5-pro":             {"gemini-2.5-pro", "Google", 1.25, 5.00},
	"gemini-1.5-pro":             {"gemini-1.5-pro", "Google", 1.25, 5.00},
}

const baselineInputPer1M = 3.00
const baselineOutputPer1M = 15.00

func getPricing(alias string) ModelPricing {
	if p, ok := modelPricingTable[alias]; ok {
		return p
	}
	if strings.HasPrefix(alias, "claude") {
		return ModelPricing{alias, "Anthropic", 3.00, 15.00}
	}
	if strings.HasPrefix(alias, "gemini") {
		return ModelPricing{alias, "Google", 0.075, 0.30}
	}
	return ModelPricing{alias, "OpenAI", 2.50, 10.00}
}

func estimateCost(alias string, inputTokens, outputTokens int) float64 {
	p := getPricing(alias)
	inCost := float64(inputTokens) / 1_000_000.0 * p.InputPer1M
	outCost := float64(outputTokens) / 1_000_000.0 * p.OutputPer1M
	return inCost + outCost
}

func estimateBaselineCost(inputTokens, outputTokens int) float64 {
	inCost := float64(inputTokens) / 1_000_000.0 * baselineInputPer1M
	outCost := float64(outputTokens) / 1_000_000.0 * baselineOutputPer1M
	return inCost + outCost
}

// SessionStats tracks comprehensive economics across the agent session
type SessionStats struct {
	mu           sync.Mutex
	Requests     int
	InputTokens  int
	OutputTokens int
	TotalCost    float64
	BaselineCost float64
	FilesChanged int
	ModelCalls   map[string]int
}

func newSessionStats() *SessionStats {
	return &SessionStats{
		ModelCalls: make(map[string]int),
	}
}

func (s *SessionStats) Record(modelUsed string, inTokens, outTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests++
	s.InputTokens += inTokens
	s.OutputTokens += outTokens
	s.TotalCost += estimateCost(modelUsed, inTokens, outTokens)
	s.BaselineCost += estimateBaselineCost(inTokens, outTokens)
	s.ModelCalls[modelUsed]++
}

func (s *SessionStats) AddFilesChanged(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FilesChanged += count
}

func (s *SessionStats) Print() {
	s.mu.Lock()
	defer s.mu.Unlock()

	savings := s.BaselineCost - s.TotalCost
	savingsPct := 0.0
	if s.BaselineCost > 0 {
		savingsPct = (savings / s.BaselineCost) * 100.0
	}

	fmt.Println()
	fmt.Println(bold(cyan("  +====================================================+")))
	fmt.Println(bold(cyan("  |         SWITCHYARD SESSION ECONOMICS REPORT        |")))
	fmt.Println(bold(cyan("  +====================================================+")))
	fmt.Printf("  Total Requests:      %s\n", bold(fmt.Sprintf("%d", s.Requests)))
	fmt.Printf("  Tokens Consumed:     %s (in: %d, out: %d)\n",
		cyan(fmt.Sprintf("%d", s.InputTokens+s.OutputTokens)), s.InputTokens, s.OutputTokens)
	fmt.Printf("  Files Modified:      %s\n", bold(fmt.Sprintf("%d", s.FilesChanged)))
	fmt.Printf("  Actual Total Spend:  %s\n", bold(green(fmt.Sprintf("$%.6f", s.TotalCost))))
	fmt.Printf("  Unrouted Baseline:   %s (Claude Sonnet baseline)\n", dim(fmt.Sprintf("$%.6f", s.BaselineCost)))
	if savings > 0 {
		fmt.Printf("  Estimated Savings:   %s %s\n",
			bold(green(fmt.Sprintf("$%.6f", savings))),
			green(fmt.Sprintf("(%.1f%% saved through routing)", savingsPct)))
	}
	if len(s.ModelCalls) > 0 {
		fmt.Println(dim("  ----------------------------------------------------"))
		fmt.Println("  Models Dispatched:")
		for m, count := range s.ModelCalls {
			p := getPricing(m)
			pct := float64(count) / float64(s.Requests) * 100.0
			fmt.Printf("    %-24s %2d call(s) (%4.1f%%) - %s\n", cyan(m), count, pct, p.Provider)
		}
	}
	fmt.Println(bold(cyan("  +====================================================+")))
	fmt.Println()
}

// Credentials management
type Credentials struct {
	APIKey    string `json:"api_key"`
	ProjectID string `json:"project_id"`
	BaseURL   string `json:"base_url"`
}

func credentialsFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".switchyard", "credentials.json")
}

func loadCredentials() (*Credentials, error) {
	path := credentialsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		// Fallback to .nexus credentials if present
		legacyPath := filepath.Join(filepath.Dir(path), "..", ".nexus", "credentials.json")
		if legacyData, lErr := os.ReadFile(legacyPath); lErr == nil {
			var creds Credentials
			if err := json.Unmarshal(legacyData, &creds); err == nil {
				return &creds, nil
			}
		}
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

func saveCredentials(creds *Credentials) error {
	path := credentialsFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func main() {
	if len(os.Args) < 2 {
		startInteractiveChat("auto", PolicyApprove, 0.0)
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		presetFlag := fs.String("preset", "auto", "Model preset: auto, explore, build, reason, review")
		modelFlag := fs.String("model", "", "Model alias or pinned provider:model")
		policyFlag := fs.String("policy", "safe-auto", "Execution policy: explain, plan, approve, safe-auto, autopilot")
		budgetFlag := fs.Float64("budget", 0.0, "Hard task budget limit in USD (e.g. 0.25)")
		cavemanFlag := fs.Bool("caveman", true, "Enable concise token-saver responses")
		fs.Parse(os.Args[2:])

		selectedPreset := *presetFlag
		if *modelFlag != "" {
			selectedPreset = *modelFlag
		}
		policy, err := ParsePolicy(*policyFlag)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		prompt := strings.Join(fs.Args(), " ")
		if strings.TrimSpace(prompt) == "" {
			fmt.Println(red("Error: Task prompt is required. Usage: switchyard run [flags] \"<prompt>\""))
			os.Exit(1)
		}
		runOneShotTask(selectedPreset, policy, *budgetFlag, *cavemanFlag, prompt)

	case "chat":
		fs := flag.NewFlagSet("chat", flag.ExitOnError)
		presetFlag := fs.String("preset", "auto", "Model preset: auto, explore, build, reason, review")
		modelFlag := fs.String("model", "", "Model alias or pinned provider:model")
		policyFlag := fs.String("policy", "approve", "Execution policy: explain, plan, approve, safe-auto, autopilot")
		budgetFlag := fs.Float64("budget", 0.0, "Hard task budget limit in USD (e.g. 0.25)")
		fs.Parse(os.Args[2:])

		selectedPreset := *presetFlag
		if *modelFlag != "" {
			selectedPreset = *modelFlag
		}
		policy, err := ParsePolicy(*policyFlag)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		startInteractiveChat(selectedPreset, policy, *budgetFlag)

	case "login":
		fs := flag.NewFlagSet("login", flag.ExitOnError)
		keyFlag := fs.String("key", "", "Switchyard API Key (ngk_...) for manual authentication")
		urlFlag := fs.String("url", DefaultGatewayURL, "Switchyard Gateway Base URL")
		frontendFlag := fs.String("frontend", DefaultFrontendURL, "Switchyard Web Frontend URL")
		projectFlag := fs.String("project", "", "Project ID")
		fs.Parse(os.Args[2:])

		if *keyFlag != "" {
			creds := &Credentials{
				APIKey:    strings.TrimSpace(*keyFlag),
				ProjectID: strings.TrimSpace(*projectFlag),
				BaseURL:   strings.TrimSpace(*urlFlag),
			}
			if err := saveCredentials(creds); err != nil {
				fmt.Println(red(fmt.Sprintf("Failed to save credentials: %v", err)))
				os.Exit(1)
			}
			fmt.Println(green("Logged in successfully. Credentials saved to ~/.switchyard/credentials.json"))
		} else {
			_, err := StartWebAuthFlow(*frontendFlag, *urlFlag)
			if err != nil {
				fmt.Println(red(fmt.Sprintf("Authentication failed: %v", err)))
				os.Exit(1)
			}
		}

	case "whoami":
		creds, err := loadCredentials()
		if err != nil || creds.APIKey == "" {
			fmt.Println(yellow("Not logged in. Run 'switchyard login' to authenticate."))
			return
		}
		fmt.Println(bold("\nSwitchyard Session:"))
		fmt.Printf("  API Key:   %s...%s\n", creds.APIKey[:min(8, len(creds.APIKey))], creds.APIKey[max(0, len(creds.APIKey)-4):])
		fmt.Printf("  Gateway:   %s\n", cyan(creds.BaseURL))
		if creds.ProjectID != "" {
			fmt.Printf("  Project:   %s\n", cyan(creds.ProjectID))
		}
		fmt.Println()

	case "help", "--help", "-h":
		printRootHelp()

	default:
		// Default to running interactive chat with the first argument as preset
		policy := PolicyApprove
		startInteractiveChat(cmd, policy, 0.0)
	}
}

func printRootHelp() {
	fmt.Println()
	fmt.Println(bold(cyan("  SWITCHYARD - The Routing, Cost & Safety Layer for AI Coding Agents")))
	fmt.Println()
	fmt.Println(bold("  Usage:"))
	fmt.Printf("    %s %s\n", cyan("switchyard run"), dim("\"<prompt>\" [--preset <name>] [--policy <mode>] [--budget <usd>]"))
	fmt.Printf("    %s %s\n", cyan("switchyard chat"), dim("[--preset <name>] [--policy <mode>] [--budget <usd>]"))
	fmt.Printf("    %s %s\n", cyan("switchyard login"), dim("-key <key> [-project <id>]"))
	fmt.Printf("    %s\n", cyan("switchyard whoami"))
	fmt.Println()
	fmt.Println(bold("  Task Presets:"))
	fmt.Printf("    %-14s %s\n", yellow("auto"), "Semantic routing: classifies task into explore, build, reason, or review")
	fmt.Printf("    %-14s %s\n", yellow("explore"), "Fast & low-cost: file search, exploration, and reading code")
	fmt.Printf("    %-14s %s\n", yellow("build"), "Balanced: feature implementation, refactoring, tests")
	fmt.Printf("    %-14s %s\n", yellow("reason"), "High frontier: architecture, concurrency, subtle logic bugs")
	fmt.Printf("    %-14s %s\n", yellow("review"), "Quality audit: code reviews, security vulnerability scanning")
	fmt.Println()
	fmt.Println(bold("  Execution Policies:"))
	fmt.Printf("    %-14s %s\n", yellow("explain"), "Read-only: answer questions without modifying any code")
	fmt.Printf("    %-14s %s\n", yellow("plan"), "Planning mode: preview plans without writing files to disk")
	fmt.Printf("    %-14s %s\n", yellow("approve"), "Default: interactive diff review with [Y/n] confirmation")
	fmt.Printf("    %-14s %s\n", yellow("safe-auto"), "Autonomous: edits code automatically, strictly blocks protected files")
	fmt.Printf("    %-14s %s\n", yellow("autopilot"), "Autonomous multi-file execution with instant git rollback")
	fmt.Println()
}

func buildPrompt(caveman bool, policy ExecutionPolicy, preset string, budget float64) string {
	tags := ""
	if caveman {
		tags += green("[caveman]") + " "
	}
	switch policy {
	case PolicyExplain:
		tags += dim("[explain]") + " "
	case PolicyPlan:
		tags += cyan("[plan]") + " "
	case PolicyApprove:
		tags += green("[approve]") + " "
	case PolicySafeAuto:
		tags += yellow("[safe-auto]") + " "
	case PolicyAutopilot:
		tags += magenta("[autopilot]") + " "
	}
	if budget > 0 {
		tags += yellow(fmt.Sprintf("[$%.2f cap]", budget)) + " "
	}

	presetDisplay := preset
	if preset == "auto" {
		presetDisplay = "routed"
	}
	return fmt.Sprintf("%s %s[%s] > ", cyan("switchyard"), tags, yellow(presetDisplay))
}

func printBanner(preset, baseURL, projectID string, caveman bool, policy ExecutionPolicy, budget float64) {
	p := getPricing(preset)
	cavemanStr := green("ON (token-saver)")
	if !caveman {
		cavemanStr = dim("OFF")
	}

	budgetStr := dim("UNLIMITED (no cap)")
	if budget > 0 {
		budgetStr = yellow(fmt.Sprintf("$%.4f per task", budget))
	}

	fmt.Println()
	fmt.Println(bold(cyan("  ========================================================")))
	fmt.Println(bold(cyan("    SWITCHYARD  -  Routing, Cost & Safety for AI Agents   ")))
	fmt.Println(bold(cyan("  ========================================================")))
	fmt.Printf("  Preset:      %s -> %s (%s)\n", yellow(preset), bold(p.Display), p.Provider)
	fmt.Printf("  Policy:      %s\n", formatPolicyLabel(policy))
	fmt.Printf("  Budget:      %s\n", budgetStr)
	fmt.Printf("  Caveman:     %s\n", cavemanStr)
	fmt.Printf("  Safety:      Protected file deny-lists + pre-task Git Checkpoints\n")
	fmt.Printf("  Suggest:     Press %s for commands, presets, policies & @files\n", bold("[Tab]"))
	fmt.Printf("  Gateway:     %s\n", cyan(baseURL))
	if projectID != "" {
		fmt.Printf("  Project:     %s\n", cyan(projectID))
	}
	fmt.Println(dim("  --------------------------------------------------------"))
	fmt.Println(dim("  /policy <mode>  /preset <name>  /budget <usd>  /rollback"))
	fmt.Println(dim("  --------------------------------------------------------"))
	fmt.Println()
}

func formatPolicyLabel(policy ExecutionPolicy) string {
	switch policy {
	case PolicyExplain:
		return dim("EXPLAIN (read-only, no modifications)")
	case PolicyPlan:
		return cyan("PLAN (previews diffs without touching files)")
	case PolicyApprove:
		return green("APPROVE (interactive [Y/n] diff confirmation)")
	case PolicySafeAuto:
		return yellow("SAFE-AUTO (auto-applies edits, blocks protected files)")
	case PolicyAutopilot:
		return magenta("AUTOPILOT (autonomous execution + auto-checkpoint)")
	default:
		return string(policy)
	}
}

func printChatHelp() {
	fmt.Println()
	fmt.Println(bold("  Switchyard Controls:"))
	fmt.Printf("  %-24s %s\n", cyan("/policy <mode>"), "Set execution policy: explain, plan, approve, safe-auto, autopilot")
	fmt.Printf("  %-24s %s\n", cyan("/preset <name>"), "Set model preset: auto, explore, build, reason, review, or model name")
	fmt.Printf("  %-24s %s\n", cyan("/budget <usd>"), "Set per-task hard spending cap (e.g. /budget 0.25, 0 to disable)")
	fmt.Printf("  %-24s %s\n", cyan("/rollback"), "Instantly rollback workspace to pre-task Git Checkpoint")
	fmt.Printf("  %-24s %s\n", cyan("/stats"), "View session economics, model distribution, and savings vs baseline")
	fmt.Printf("  %-24s %s\n", cyan("/caveman on|off"), "Toggle concise direct output for maximum token savings")
	fmt.Printf("  %-24s %s\n", cyan("/models"), "List available models and provider connections")
	fmt.Printf("  %-24s %s\n", cyan("/whoami"), "Show current credentials and active project")
	fmt.Printf("  %-24s %s\n", cyan("/clear"), "Clear conversation history")
	fmt.Printf("  %-24s %s\n", cyan("/exit  /quit"), "Exit Switchyard")
	fmt.Println()
	fmt.Println(bold("  Interactive Tips:"))
	fmt.Printf("  %-24s %s\n", cyan("[Tab] key"), "Autocompletes slash commands, policies, presets, and @files")
	fmt.Printf("  %-24s %s\n", cyan("@file / @dir"), "Embeds workspace files with line numbers into prompt context")
	fmt.Printf("  %-24s %s\n", cyan("Safety Guardrails"), ".env*, lockfiles, certificates, and .git are strictly protected")
	fmt.Println()
}

func startInteractiveChat(initialPreset string, initialPolicy ExecutionPolicy, initialBudget float64) {
	creds, _ := loadCredentials()
	apiKey := os.Getenv("SWITCHYARD_API_KEY")
	baseURL := os.Getenv("SWITCHYARD_BASE_URL")

	if apiKey == "" && creds != nil {
		apiKey = creds.APIKey
	}
	if apiKey == "" {
		apiKey = os.Getenv("NEXUS_API_KEY")
	}
	if baseURL == "" && creds != nil && creds.BaseURL != "" {
		baseURL = creds.BaseURL
	}
	if baseURL == "" {
		baseURL = os.Getenv("NEXUS_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	if apiKey == "" {
		fmt.Println(yellow("No credentials found. Launching Switchyard web authentication..."))
		newCreds, err := StartWebAuthFlow(DefaultFrontendURL, baseURL)
		if err != nil {
			fmt.Println(red(fmt.Sprintf("Authentication failed: %v", err)))
			fmt.Println(dim("You can log in manually using: switchyard login -key <ngk_...>"))
			return
		}
		creds = newCreds
		apiKey = creds.APIKey
	}

	preset := initialPreset
	if preset == "" {
		preset = "auto"
	}
	policy := initialPolicy
	budget := initialBudget
	caveman := true
	stats := newSessionStats()
	var history []map[string]string

	projectID := ""
	if creds != nil {
		projectID = creds.ProjectID
	}

	printBanner(preset, baseURL, projectID, caveman, policy, budget)

	home, _ := os.UserHomeDir()
	historyFile := filepath.Join(home, ".switchyard", "chat_history")
	_ = os.MkdirAll(filepath.Dir(historyFile), 0700)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          buildPrompt(caveman, policy, preset, budget),
		HistoryFile:     historyFile,
		AutoComplete:    NewSwitchyardCompleter(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Printf("Terminal error: %v\n", err)
		return
	}
	defer rl.Close()

	for {
		rl.SetPrompt(buildPrompt(caveman, policy, preset, budget))
		line, err := rl.Readline()
		if err != nil {
			break
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			cmd := parts[0]
			switch cmd {
			case "/help":
				printChatHelp()

			case "/policy", "/pol":
				if len(parts) < 2 {
					fmt.Printf("  Active Policy: %s\n  Options: explain, plan, approve, safe-auto, autopilot\n\n",
						bold(formatPolicyLabel(policy)))
				} else {
					newPol, pErr := ParsePolicy(parts[1])
					if pErr != nil {
						fmt.Printf("  %s %v\n\n", red("Error:"), pErr)
					} else {
						policy = newPol
						fmt.Printf("  %s Switched execution policy to: %s\n\n", green("OK"), bold(formatPolicyLabel(policy)))
					}
				}

			case "/preset", "/model":
				if len(parts) < 2 {
					p := getPricing(preset)
					fmt.Printf("  Active Preset: %s -> %s (%s)\n\n", yellow(preset), bold(p.Display), p.Provider)
				} else {
					newPreset := parts[1]
					p := getPricing(newPreset)
					preset = newPreset
					fmt.Printf("  %s Switched preset to: %s -> %s (%s)\n\n", green("OK"), yellow(preset), bold(p.Display), p.Provider)
				}

			case "/rollback":
				fmt.Println(bold("  Executing workspace rollback to pre-task Git Checkpoint..."))
				if err := RollbackGitCheckpoint("."); err != nil {
					fmt.Printf("  %s %v\n\n", red("Rollback Failed:"), err)
				} else {
					fmt.Printf("  %s Workspace cleanly restored to pre-task state.\n\n", green("SUCCESS:"))
				}

			case "/caveman":
				if len(parts) < 2 {
					fmt.Printf("  Caveman mode is currently: %v\n\n", caveman)
				} else {
					arg := strings.ToLower(parts[1])
					caveman = (arg == "on" || arg == "true" || arg == "1")
					fmt.Printf("  %s Caveman mode set to: %v\n\n", green("OK"), caveman)
				}

			case "/budget":
				if len(parts) < 2 {
					if budget <= 0 {
						fmt.Println("  Budget limit: UNLIMITED (no cap)")
					} else {
						fmt.Printf("  Budget limit: $%.4f per task\n", budget)
					}
					fmt.Println("  Use: /budget <amount> (e.g. /budget 0.25) or /budget 0 to disable")
				} else {
					var b float64
					_, err := fmt.Sscanf(parts[1], "%f", &b)
					if err != nil || b < 0 {
						fmt.Println(red("  Invalid budget amount. Example: /budget 0.25"))
					} else {
						budget = b
						if budget == 0 {
							fmt.Println(green("  OK Budget limit disabled (unlimited spend)."))
						} else {
							fmt.Printf("  %s Task budget limit set to $%.4f\n\n", green("OK"), budget)
						}
					}
				}

			case "/stats":
				stats.Print()

			case "/whoami":
				if creds != nil {
					fmt.Printf("  Switchyard Session: Project %s | Gateway %s\n\n", cyan(creds.ProjectID), cyan(baseURL))
				}

			case "/clear":
				history = nil
				fmt.Printf("  %s Conversation context cleared.\n\n", green("OK"))

			case "/exit", "/quit":
				return

			default:
				fmt.Printf("  %s Unknown command '%s'. Type /help for controls.\n\n", yellow("!"), cmd)
			}
			continue
		}

		// 1. Expand @mentions into context
		expandedInput, items, ctxErrs := ExpandPromptContext(input)
		for _, err := range ctxErrs {
			fmt.Printf("  %s %v\n", yellow("[Context Warning]"), err)
		}
		if len(items) > 0 {
			var names []string
			for _, it := range items {
				if it.IsDir {
					names = append(names, fmt.Sprintf("%s/ (%d items)", it.Path, it.Children))
				} else {
					names = append(names, fmt.Sprintf("%s (%d lines)", it.Path, it.Lines))
				}
			}
			fmt.Printf("  %s %s\n", dim("[Attached Context]"), dim(strings.Join(names, ", ")))
		}

		history = append(history, map[string]string{"role": "user", "content": expandedInput})

		spinner := newSpinner("Switchyard is analyzing and routing...")
		spinner.Start()

		result, err := sendChatConversation(baseURL, apiKey, preset, history, caveman, policy, budget)

		spinner.Stop()

		if err != nil {
			fmt.Printf("\n  %s %v\n\n", red("Error:"), err)
			history = history[:len(history)-1]
			continue
		}

		stats.Record(result.ModelUsed, result.InputTokens, result.OutputTokens)
		history = append(history, map[string]string{"role": "assistant", "content": result.Content})

		// Print Explainable Routing Decision Trace
		if result.Routing != nil {
			printRoutingDecision(result.Routing)
		}

		fmt.Printf("\n%s %s\n", bold(cyan("Switchyard:")), result.Content)
		printResponseMeta(result, preset)

		// 2. Parse and apply proposed code modifications under safety policy
		proposedEdits := ParseProposedEdits(result.Content)
		if len(proposedEdits) > 0 {
			taskDesc := input
			if len(taskDesc) > 50 {
				taskDesc = taskDesc[:50] + "..."
			}
			applied, blocked, editErrs := ApplyEditsWithPolicy(proposedEdits, policy, taskDesc)
			for _, err := range editErrs {
				fmt.Printf("  %s %v\n", red("[Edit Error]"), err)
			}
			stats.AddFilesChanged(applied)
			if applied > 0 {
				fmt.Printf("\n  %s Applied %d code modification(s) under policy '%s'.\n",
					green("SUCCESS:"), applied, bold(string(policy)))
			}
			if blocked > 0 {
				fmt.Printf("  %s Blocked %d protected file modification(s).\n",
					yellow("GUARDRAIL:"), blocked)
			}
			fmt.Println()
		}
	}
}

type ChatResult struct {
	Content      string
	ModelUsed    string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
	Cost         float64
	Routing      *RoutingMeta
}

type RoutingMeta struct {
	RequestedModel string            `json:"requested_model"`
	SelectedModel  string            `json:"selected_model"`
	Provider       string            `json:"provider"`
	Preset         string            `json:"preset"`
	Tier           string            `json:"tier"`
	Complexity     float64           `json:"complexity_score"`
	Confidence     int               `json:"confidence_percent"`
	Intent         string            `json:"intent"`
	Rationale      string            `json:"rationale"`
	ContextChars   int               `json:"context_chars"`
	EstimatedCost  float64           `json:"estimated_cost"`
	BaselineCost   float64           `json:"baseline_cost"`
	FallbackTrace  []FallbackTrace   `json:"fallback_trace,omitempty"`
	RedirectSum    string            `json:"redirect_summary,omitempty"`
}

type FallbackTrace struct {
	Model      string `json:"model"`
	ProviderID string `json:"provider_id"`
	Tier       string `json:"tier"`
	Error      string `json:"error"`
	Reason     string `json:"reason"`
}

func printRoutingDecision(r *RoutingMeta) {
	fmt.Println()
	fmt.Printf("  %s\n", bold("[Switchyard Route Decision]"))
	presetLabel := r.Preset
	if presetLabel == "" {
		presetLabel = r.Tier
	}
	confidenceStr := ""
	if r.Confidence > 0 {
		confidenceStr = fmt.Sprintf("(confidence: %d%%)", r.Confidence)
	}
	fmt.Printf("  |- Preset:     %s %s\n", yellow(presetLabel), dim(confidenceStr))
	fmt.Printf("  |- Intent:     %s %s\n", cyan(r.Intent), dim(fmt.Sprintf("(score: %.2f)", r.Complexity)))
	fmt.Printf("  |- Rationale:  %s\n", dim(r.Rationale))
	fmt.Printf("  |- Candidate:  %s (%s)\n", bold(r.SelectedModel), r.Provider)

	if r.BaselineCost > 0 && r.EstimatedCost > 0 {
		savings := r.BaselineCost - r.EstimatedCost
		if savings > 0 {
			fmt.Printf("  |- Economics:  Est. $%.5f vs $%.5f baseline %s\n",
				r.EstimatedCost, r.BaselineCost, green(fmt.Sprintf("(saved ~$%.5f)", savings)))
		}
	}

	if len(r.FallbackTrace) > 0 {
		for i, fb := range r.FallbackTrace {
			fmt.Printf("  |- %s %s (%s) failed: %s -> redirected\n",
				yellow(fmt.Sprintf("Fallback #%d:", i+1)),
				fb.Model, fb.ProviderID, dim(truncateStr(fb.Error, 60)),
			)
		}
	}
}

func printResponseMeta(r ChatResult, preset string) {
	latencyStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
	costStr := fmt.Sprintf("$%.6f", r.Cost)
	modelLabel := r.ModelUsed
	if modelLabel == "" {
		modelLabel = preset
	}
	p := getPricing(modelLabel)

	fmt.Printf("  %s %s (%s)  %s  in:%d  out:%d  %s\n\n",
		dim("~"),
		yellow(modelLabel),
		p.Provider,
		dim(latencyStr),
		r.InputTokens,
		r.OutputTokens,
		green(costStr),
	)
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func sendChatConversation(baseURL, apiKey, preset string, history []map[string]string, caveman bool, policy ExecutionPolicy, budget float64) (ChatResult, error) {
	// 1. Pre-flight budget estimation
	totalChars := 0
	for _, m := range history {
		totalChars += len(m["content"])
	}
	estTokens := totalChars / 4
	if estTokens < 50 {
		estTokens = 50
	}
	preCost := estimateCost(preset, estTokens, 400)
	if budget > 0 && preCost > budget {
		return ChatResult{}, fmt.Errorf("BUDGET EXCEEDED: Estimated prompt cost ($%.5f) exceeds task budget limit ($%.5f). Execution aborted before calling models.", preCost, budget)
	}

	start := time.Now()

	payload := map[string]interface{}{
		"model":    preset,
		"messages": history,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, err
	}

	url := strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return ChatResult{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-Switchyard-Policy", string(policy))
	if caveman {
		req.Header.Set("X-Nexus-Caveman", "true")
	}

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("connection failed to Switchyard Gateway (%s): %w", baseURL, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResult{}, err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(respBytes, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			return ChatResult{}, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
		}
		return ChatResult{}, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		NexusRouting *RoutingMeta `json:"nexus_routing"`
	}

	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return ChatResult{}, fmt.Errorf("failed parsing gateway response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("no response choices returned from gateway")
	}

	latency := time.Since(start)
	cost := estimateCost(chatResp.Model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)

	if budget > 0 && cost > budget {
		fmt.Printf("\n  %s Actual task spend ($%.5f) exceeded budget limit ($%.5f) by $%.5f.\n",
			red("[BUDGET ALERT]"), cost, budget, cost-budget)
	}

	return ChatResult{
		Content:      chatResp.Choices[0].Message.Content,
		ModelUsed:    chatResp.Model,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Latency:      latency,
		Cost:         cost,
		Routing:      chatResp.NexusRouting,
	}, nil
}

// runOneShotTask executes a single terminal task non-interactively and exits.
func runOneShotTask(preset string, policy ExecutionPolicy, budget float64, caveman bool, rawPrompt string) {
	creds, _ := loadCredentials()
	apiKey := os.Getenv("SWITCHYARD_API_KEY")
	baseURL := os.Getenv("SWITCHYARD_BASE_URL")

	if apiKey == "" && creds != nil {
		apiKey = creds.APIKey
	}
	if apiKey == "" {
		apiKey = os.Getenv("NEXUS_API_KEY")
	}
	if baseURL == "" && creds != nil && creds.BaseURL != "" {
		baseURL = creds.BaseURL
	}
	if baseURL == "" {
		baseURL = os.Getenv("NEXUS_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	if apiKey == "" {
		fmt.Println(yellow("No credentials found. Launching Switchyard web authentication..."))
		newCreds, err := StartWebAuthFlow(DefaultFrontendURL, baseURL)
		if err != nil {
			fmt.Println(red(fmt.Sprintf("Authentication failed: %v", err)))
			fmt.Println(dim("You can log in manually using: switchyard login -key <ngk_...>"))
			os.Exit(1)
		}
		creds = newCreds
		apiKey = creds.APIKey
	}

	// 1. Expand @mentions into context
	expandedPrompt, items, ctxErrs := ExpandPromptContext(rawPrompt)
	for _, err := range ctxErrs {
		fmt.Printf("  %s %v\n", yellow("[Context Warning]"), err)
	}
	if len(items) > 0 {
		var names []string
		for _, it := range items {
			if it.IsDir {
				names = append(names, fmt.Sprintf("%s/ (%d items)", it.Path, it.Children))
			} else {
				names = append(names, fmt.Sprintf("%s (%d lines)", it.Path, it.Lines))
			}
		}
		fmt.Printf("  %s %s\n", dim("[Attached Context]"), dim(strings.Join(names, ", ")))
	}

	history := []map[string]string{
		{"role": "user", "content": expandedPrompt},
	}

	spinner := newSpinner("Switchyard is analyzing task and routing...")
	spinner.Start()

	result, err := sendChatConversation(baseURL, apiKey, preset, history, caveman, policy, budget)
	spinner.Stop()

	if err != nil {
		fmt.Printf("\n  %s %v\n\n", red("Error:"), err)
		if strings.Contains(err.Error(), "BUDGET EXCEEDED") {
			os.Exit(2)
		}
		os.Exit(1)
	}

	// Print Explainable Routing Decision Trace
	if result.Routing != nil {
		printRoutingDecision(result.Routing)
	}

	fmt.Printf("\n%s\n%s\n", bold(cyan("Switchyard:")), result.Content)
	printResponseMeta(result, preset)

	// Apply edits under policy
	proposedEdits := ParseProposedEdits(result.Content)
	if len(proposedEdits) > 0 {
		taskDesc := rawPrompt
		if len(taskDesc) > 50 {
			taskDesc = taskDesc[:50] + "..."
		}
		applied, blocked, editErrs := ApplyEditsWithPolicy(proposedEdits, policy, taskDesc)
		for _, err := range editErrs {
			fmt.Printf("  %s %v\n", red("[Edit Error]"), err)
		}
		if applied > 0 {
			fmt.Printf("\n  %s Applied %d code modification(s) under policy '%s'.\n",
				green("SUCCESS:"), applied, bold(string(policy)))
		}
		if blocked > 0 {
			fmt.Printf("  %s Blocked %d protected file modification(s).\n",
				yellow("GUARDRAIL:"), blocked)
		}
	}

	// Task budget report
	if budget > 0 {
		rem := budget - result.Cost
		if rem >= 0 {
			fmt.Printf("  %s Spent $%.5f of $%.5f budget limit ($%.5f remaining)\n\n",
				dim("[Budget]"), result.Cost, budget, rem)
		}
	}
}

type spinner struct {
	msg     string
	stopCh  chan struct{}
	stopped bool
	mu      sync.Mutex
}

func newSpinner(msg string) *spinner {
	return &spinner{
		msg:    msg,
		stopCh: make(chan struct{}),
	}
}

func (s *spinner) Start() {
	frames := []string{"|", "/", "-", "\\"}
	go func() {
		i := 0
		for {
			select {
			case <-s.stopCh:
				fmt.Print("\r\033[K")
				return
			default:
				fmt.Printf("\r  %s %s", cyan(frames[i%len(frames)]), dim(s.msg))
				time.Sleep(100 * time.Millisecond)
				i++
			}
		}
	}()
}

func (s *spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.stopped = true
		close(s.stopCh)
		time.Sleep(50 * time.Millisecond)
		fmt.Print("\r\033[K")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}