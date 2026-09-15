// Command nexus-cli provides terminal administration, diagnostics, and an interactive chat REPL for Nexus AI Gateway.
package main

import (
	"bufio"
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
)

func bold(s string) string   { return colorBold + s + colorReset }
func green(s string) string  { return colorGreen + s + colorReset }
func cyan(s string) string   { return colorCyan + s + colorReset }
func yellow(s string) string { return colorYellow + s + colorReset }
func dim(s string) string    { return colorDim + s + colorReset }
func red(s string) string    { return colorRed + s + colorReset }

// Model pricing table (per 1M tokens, USD)

type ModelPricing struct {
	Display     string
	Provider    string
	InputPer1M  float64
	OutputPer1M float64
}

var modelPricingTable = map[string]ModelPricing{
	"auto":                       {"dynamic", "Nexus Router", 0.075, 0.30},
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
	"gemini-2.5-pro":             {"gemini-2.5-pro", "Google", 3.50, 10.50},
	"gemini-1.5-pro":             {"gemini-1.5-pro", "Google", 3.50, 10.50},
}

const gpt4oInputPer1M = 2.50
const gpt4oOutputPer1M = 10.00

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
	return (float64(inputTokens)/1_000_000)*p.InputPer1M +
		(float64(outputTokens)/1_000_000)*p.OutputPer1M
}

func estimateGPT4oCost(inputTokens, outputTokens int) float64 {
	return (float64(inputTokens)/1_000_000)*gpt4oInputPer1M +
		(float64(outputTokens)/1_000_000)*gpt4oOutputPer1M
}

// Spinner

type Spinner struct {
	frames []string
	label  string
	done   chan struct{}
	wg     sync.WaitGroup
}

func newSpinner(label string) *Spinner {
	return &Spinner{
		frames: []string{"|", "/", "-", "\\"},
		label:  label,
		done:   make(chan struct{}),
	}
}

func (s *Spinner) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		i := 0
		for {
			select {
			case <-s.done:
				fmt.Printf("\r%s\r", strings.Repeat(" ", len(s.label)+12))
				return
			default:
				fmt.Printf("\r  %s %s", cyan(s.frames[i%len(s.frames)]), dim(s.label))
				time.Sleep(80 * time.Millisecond)
				i++
			}
		}
	}()
}

func (s *Spinner) Stop() {
	close(s.done)
	s.wg.Wait()
}

// Session stats

type SessionStats struct {
	Requests     int
	InputTokens  int
	OutputTokens int
	ActualCost   float64
	BaselineCost float64
	ModelCounts  map[string]int
}

func newSessionStats() *SessionStats {
	return &SessionStats{ModelCounts: make(map[string]int)}
}

func (s *SessionStats) Record(modelUsed string, inputTok, outputTok int) {
	s.Requests++
	s.InputTokens += inputTok
	s.OutputTokens += outputTok
	s.ActualCost += estimateCost(modelUsed, inputTok, outputTok)
	s.BaselineCost += estimateGPT4oCost(inputTok, outputTok)
	s.ModelCounts[modelUsed]++
}

// Credentials

type Credentials struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	ProjectID string `json:"project_id,omitempty"`
}

// FallbackAttemptTrace records a model failure before redirect
type FallbackAttemptTrace struct {
	Model      string `json:"model"`
	ProviderID string `json:"provider_id"`
	Tier       string `json:"tier"`
	Error      string `json:"error"`
	Reason     string `json:"reason"`
}

// RoutingMeta holds gateway decision metadata and fallback trace
type RoutingMeta struct {
	RequestedModel  string                 `json:"requested_model"`
	SelectedModel   string                 `json:"selected_model"`
	Provider        string                 `json:"provider"`
	Tier            string                 `json:"tier"`
	ComplexityScore float64                `json:"complexity_score"`
	Intent          string                 `json:"intent"`
	Rationale       string                 `json:"rationale"`
	ContextChars    int                    `json:"context_chars"`
	FallbackTrace   []FallbackAttemptTrace `json:"fallback_trace,omitempty"`
	RedirectSummary string                 `json:"redirect_summary,omitempty"`
}

// ChatResult

type ChatResult struct {
	Content      string
	ModelUsed    string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
	Cost         float64
	Routing      *RoutingMeta
}

// main

func main() {
	creds := loadCredentials()

	baseURL := os.Getenv("NEXUS_BASE_URL")
	if baseURL == "" {
		baseURL = creds.BaseURL
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	apiKey := os.Getenv("NEXUS_API_KEY")
	if apiKey == "" {
		apiKey = creds.APIKey
	}

	if len(os.Args) < 2 {
		if creds.APIKey != "" {
			startInteractiveChat(baseURL, apiKey, "auto", creds)
			return
		}
		printUsage()
		os.Exit(0)
	}

	subcommand := os.Args[1]

	switch subcommand {
	case "help", "--help", "-h":
		printUsage()

	case "login":
		loginCmd := flag.NewFlagSet("login", flag.ExitOnError)
		keyFlag := loginCmd.String("key", "", "Gateway API Key (ngk_live_...)")
		projectFlag := loginCmd.String("project", "", "Default Project ID (prj_...)")
		urlFlag := loginCmd.String("url", baseURL, "Gateway Base URL")
		_ = loginCmd.Parse(os.Args[2:])

		keyVal := *keyFlag
		if keyVal == "" {
			fmt.Print("Enter your Nexus Gateway API Key (ngk_...): ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			keyVal = strings.TrimSpace(input)
		}
		if keyVal == "" {
			fmt.Println(red("Error: API Key cannot be empty."))
			os.Exit(1)
		}
		credsToSave := Credentials{BaseURL: *urlFlag, APIKey: keyVal, ProjectID: *projectFlag}
		if err := saveCredentials(credsToSave); err != nil {
			fmt.Printf("Error saving credentials: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%s Logged in! Credentials saved to %s\n", green("OK"), getCredentialsPath())

	case "whoami":
		if creds.APIKey == "" {
			fmt.Println(yellow("Not logged in. Run 'nexus-cli login' to authenticate."))
			return
		}
		prefix := creds.APIKey
		if len(prefix) > 12 {
			prefix = prefix[:12] + "..."
		}
		fmt.Println(bold("\nNexus Gateway Session:"))
		fmt.Printf("  Gateway URL: %s\n", cyan(creds.BaseURL))
		fmt.Printf("  API Key:     %s\n", dim(prefix))
		if creds.ProjectID != "" {
			fmt.Printf("  Project ID:  %s\n", cyan(creds.ProjectID))
		}
		fmt.Printf("  Config File: %s\n\n", dim(getCredentialsPath()))

	case "logout":
		if err := removeCredentials(); err != nil {
			fmt.Printf("Error removing credentials: %v\n", err)
			return
		}
		fmt.Println(green("OK") + " Logged out successfully.")

	case "health":
		checkHealth(baseURL)

	case "models":
		listModels(baseURL, apiKey)

	case "usage":
		usageCmd := flag.NewFlagSet("usage", flag.ExitOnError)
		projectFlag := usageCmd.String("project", creds.ProjectID, "Project ID")
		limitFlag := usageCmd.Int("limit", 50, "Limit records")
		_ = usageCmd.Parse(os.Args[2:])
		if *projectFlag == "" {
			fmt.Println(red("Error: -project is required (or specify when logging in)"))
			os.Exit(1)
		}
		showUsage(baseURL, apiKey, *projectFlag, *limitFlag)

	case "chat":
		chatCmd := flag.NewFlagSet("chat", flag.ExitOnError)
		model := chatCmd.String("model", "auto", "Model alias (auto, smart, cheap, fast, quality...)")
		prompt := chatCmd.String("prompt", "", "Single prompt (omit to start REPL)")
		key := chatCmd.String("key", apiKey, "Gateway API key")
		_ = chatCmd.Parse(os.Args[2:])
		if *prompt == "" {
			startInteractiveChat(baseURL, *key, *model, creds)
			return
		}
		sendSingleChat(baseURL, *key, *model, *prompt, true)

	case "providers":
		if len(os.Args) < 3 || os.Args[2] != "add" {
			fmt.Println("Usage: nexus-cli providers add -provider <openai|anthropic|gemini> -key <key>")
			os.Exit(1)
		}
		pCmd := flag.NewFlagSet("providers add", flag.ExitOnError)
		providerName := pCmd.String("provider", "", "Provider name")
		secret := pCmd.String("key", "", "Provider API Key")
		projID := pCmd.String("project", creds.ProjectID, "Project ID")
		_ = pCmd.Parse(os.Args[3:])
		if *providerName == "" || *secret == "" || *projID == "" {
			fmt.Println(red("Error: -provider, -key, and -project are required"))
			os.Exit(1)
		}
		addProvider(baseURL, *projID, *providerName, *secret)

	case "create-org":
		orgCmd := flag.NewFlagSet("create-org", flag.ExitOnError)
		name := orgCmd.String("name", "", "Organization name")
		_ = orgCmd.Parse(os.Args[2:])
		if *name == "" {
			fmt.Println(red("Error: -name is required"))
			os.Exit(1)
		}
		createOrg(baseURL, *name)

	case "create-project":
		projCmd := flag.NewFlagSet("create-project", flag.ExitOnError)
		orgID := projCmd.String("org", "", "Org ID")
		name := projCmd.String("name", "", "Project name")
		env := projCmd.String("env", "production", "Environment")
		_ = projCmd.Parse(os.Args[2:])
		if *orgID == "" || *name == "" {
			fmt.Println(red("Error: -org and -name are required"))
			os.Exit(1)
		}
		createProject(baseURL, *orgID, *name, *env)

	case "create-key":
		keyCmd := flag.NewFlagSet("create-key", flag.ExitOnError)
		projID := keyCmd.String("project", creds.ProjectID, "Project ID")
		env := keyCmd.String("env", "live", "Key environment prefix")
		_ = keyCmd.Parse(os.Args[2:])
		if *projID == "" {
			fmt.Println(red("Error: -project is required"))
			os.Exit(1)
		}
		createKey(baseURL, *projID, *env)

	default:
		fmt.Printf("Unknown command: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

// startInteractiveChat launches an interactive terminal chat REPL.

func startInteractiveChat(baseURL, apiKey, initialModel string, creds Credentials) {
	if apiKey == "" {
		fmt.Println(red("Error: Not logged in. Run 'nexus-cli login' first or pass -key."))
		return
	}

	model := initialModel
	if model == "" {
		model = "auto"
	}

	caveman := true
	autoApply := false
	stats := newSessionStats()
	var history []map[string]string

	printBanner(model, baseURL, creds.ProjectID, caveman, autoApply)

	home, _ := os.UserHomeDir()
	historyFile := filepath.Join(home, ".nexus", "chat_history")
	_ = os.MkdirAll(filepath.Dir(historyFile), 0700)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          buildPrompt(caveman, autoApply, model),
		HistoryFile:     historyFile,
		AutoComplete:    NewNexusCompleter(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Printf("Terminal readline error: %v\n", err)
		return
	}
	defer rl.Close()

	for {
		rl.SetPrompt(buildPrompt(caveman, autoApply, model))
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

			case "/model":
				if len(parts) < 2 {
					p := getPricing(model)
					fmt.Printf("  Current model: %s -> %s (%s)\n\n", yellow(model), bold(p.Display), p.Provider)
				} else {
					newModel := parts[1]
					p := getPricing(newModel)
					model = newModel
					fmt.Printf("  %s Switched to %s -> %s (%s)\n",
						green("OK"), yellow(model), bold(p.Display), p.Provider)
					if model == "auto" {
						fmt.Println("  Routing: Dynamic based on prompt complexity (fast for simple, smart for complex)")
					} else {
						fmt.Printf("  Pricing: $%.3f/Mtok in  $%.2f/Mtok out\n\n", p.InputPer1M, p.OutputPer1M)
					}
				}

			case "/caveman":
				if len(parts) < 2 {
					status := "ENABLED (concise, direct, max token savings)"
					if !caveman {
						status = "DISABLED (verbose responses)"
					}
					fmt.Printf("  Caveman Mode is currently: %s\n  Use: /caveman on | /caveman off\n\n", bold(status))
				} else {
					arg := strings.ToLower(parts[1])
					if arg == "on" || arg == "true" || arg == "1" {
						caveman = true
						fmt.Printf("  %s Caveman Mode ENABLED: responses will be direct and token-optimized.\n\n", green("OK"))
					} else {
						caveman = false
						fmt.Printf("  %s Caveman Mode DISABLED: responses will use standard verbosity.\n\n", yellow("!"))
					}
				}

			case "/auto-apply", "/autowrite", "/auto":
				if len(parts) < 2 {
					status := "DISABLED (requires manual [Y/n] confirmation for every change)"
					if autoApply {
						status = "ENABLED (AI automatically writes and patches files without prompting)"
					}
					fmt.Printf("  Auto-Apply is currently: %s\n  Use: /auto-apply on | /auto-apply off\n\n", bold(status))
				} else {
					arg := strings.ToLower(parts[1])
					if arg == "on" || arg == "true" || arg == "1" {
						autoApply = true
						fmt.Printf("  %s Auto-Apply Mode ENABLED: proposed code edits will be written automatically.\n\n", yellow("WARNING:"))
					} else {
						autoApply = false
						fmt.Printf("  %s Auto-Apply Mode DISABLED: manual confirmation [Y/n] will be requested for all changes.\n\n", green("OK"))
					}
				}

			case "/models":
				listModels(baseURL, apiKey)

			case "/usage":
				if creds.ProjectID != "" {
					showUsage(baseURL, apiKey, creds.ProjectID, 50)
				} else {
					fmt.Println(yellow("  Project ID not set in current session."))
				}

			case "/stats":
				printSessionStats(stats)

			case "/whoami":
				prefix := apiKey
				if len(prefix) > 12 {
					prefix = prefix[:12] + "..."
				}
				fmt.Printf("  Gateway: %s  Project: %s  Key: %s  Caveman: %v\n\n",
					cyan(baseURL), cyan(creds.ProjectID), dim(prefix), caveman)

			case "/clear":
				history = nil
				fmt.Println(green("  OK") + " Conversation context cleared.\n")

			case "/exit", "/quit":
				if stats.Requests > 0 {
					printSessionStats(stats)
				}
				fmt.Println(cyan("\n  Goodbye!\n"))
				return

			default:
				fmt.Printf("  Unknown command: %s. Type /help for available commands.\n\n", cmd)
			}
			continue
		}

		// 1. Expand any @file or @dir mentions in input
		expandedInput, contextItems, contextErrs := ExpandPromptContext(input)
		for _, err := range contextErrs {
			fmt.Printf("  %s %v\n", yellow("[Context Notice]"), err)
		}
		if len(contextItems) > 0 {
			var loadedNames []string
			for _, item := range contextItems {
				loadedNames = append(loadedNames, filepath.ToSlash(item.Path))
			}
			fmt.Printf("  %s Loaded file context: %s\n", green("->"), cyan(strings.Join(loadedNames, ", ")))
		}

		history = append(history, map[string]string{"role": "user", "content": expandedInput})

		spinner := newSpinner("Nexus is analyzing and routing...")
		spinner.Start()

		result, err := sendChatConversation(baseURL, apiKey, model, history, caveman)

		spinner.Stop()

		if err != nil {
			fmt.Printf("\n  %s %v\n\n", red("Error:"), err)
			history = history[:len(history)-1]
			continue
		}

		stats.Record(result.ModelUsed, result.InputTokens, result.OutputTokens)

		history = append(history, map[string]string{"role": "assistant", "content": result.Content})

		// Print Routing Thought Process & Fallback Redirection Traces
		if result.Routing != nil {
			printRoutingDecision(result.Routing)
		}

		fmt.Printf("\n%s %s\n", bold(cyan("Nexus:")), result.Content)
		printResponseMeta(result, model)

		// 2. Parse any proposed code modifications (Search/Replace or Write blocks)
		proposedEdits := ParseProposedEdits(result.Content)
		if len(proposedEdits) > 0 {
			applied, editErrs := PromptAndApplyEdits(proposedEdits, autoApply)
			for _, err := range editErrs {
				fmt.Printf("  %s %v\n", red("[Edit Error]"), err)
			}
			if applied > 0 {
				fmt.Printf("\n  %s Applied %d code modification(s) to your workspace.\n\n", green("SUCCESS:"), applied)
			}
		}
	}
}

func printRoutingDecision(r *RoutingMeta) {
	fmt.Println()
	fmt.Printf("  %s\n", bold("[Nexus Routing Engine]"))
	fmt.Printf("  |- Intent detected: %s %s\n", cyan(r.Intent), dim(fmt.Sprintf("(score: %.2f)", r.ComplexityScore)))
	fmt.Printf("  |- Rationale:       %s\n", dim(r.Rationale))

	if len(r.FallbackTrace) > 0 {
		for i, fb := range r.FallbackTrace {
			fmt.Printf("  |- %s Model attempt [%s] -> %s (%s) failed\n",
				red(fmt.Sprintf("Failover #%d:", i+1)),
				fb.Tier,
				bold(fb.Model),
				fb.ProviderID,
			)
			fmt.Printf("  |  * Error:  %s\n", red(truncateStr(fb.Error, 85)))
			fmt.Printf("  |  * Action: %s\n", yellow(fb.Reason))
		}
		fmt.Printf("  |- Decision:        %s -> %s (%s) [Resolved via Fallback]\n",
			green("["+r.Tier+"]"),
			bold(green(r.SelectedModel)),
			r.Provider,
		)
	} else {
		fmt.Printf("  |- Decision:        %s -> %s (%s)\n",
			yellow("["+r.Tier+"]"),
			bold(r.SelectedModel),
			r.Provider,
		)
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func buildPrompt(caveman, autoApply bool, model string) string {
	tags := ""
	if caveman {
		tags += green("[caveman]") + " "
	}
	if autoApply {
		tags += yellow("[auto-write]") + " "
	}
	modelDisplay := model
	if model == "auto" {
		modelDisplay = "routed"
	}
	return fmt.Sprintf("%s %s[%s] > ", cyan("nexus"), tags, yellow(modelDisplay))
}

func printBanner(model, baseURL, projectID string, caveman, autoApply bool) {
	p := getPricing(model)
	cavemanStr := green("ON (token-saver)")
	if !caveman {
		cavemanStr = dim("OFF")
	}
	autoApplyStr := dim("OFF (manual [Y/n] confirmation)")
	if autoApply {
		autoApplyStr = yellow("ON (auto-write to disk)")
	}
	fmt.Println()
	fmt.Println(bold(cyan("  +====================================================+")))
	fmt.Println(bold(cyan("  |      NEXUS AI GATEWAY  -  Interactive Coding Agent  |")))
	fmt.Println(bold(cyan("  +====================================================+")))
	fmt.Printf("  Model:       %s -> %s (%s)\n", yellow(model), bold(p.Display), p.Provider)
	fmt.Printf("  Auto-Apply:  %s\n", autoApplyStr)
	fmt.Printf("  Caveman:     %s\n", cavemanStr)
	fmt.Printf("  Suggest:     Press %s anytime to autocomplete %s and %s\n", bold("[Tab]"), cyan("/commands"), cyan("@files/@dirs"))
	fmt.Printf("  Gateway:     %s\n", cyan(baseURL))
	if projectID != "" {
		fmt.Printf("  Project:     %s\n", cyan(projectID))
	}
	fmt.Println(dim("  ----------------------------------------------------"))
	fmt.Println(dim("  /auto on|off  /model <name>  /caveman on|off  /exit"))
	fmt.Println(dim("  ----------------------------------------------------"))
	fmt.Println()
}

func printChatHelp() {
	fmt.Println()
	fmt.Println(bold("  Available Commands:"))
	fmt.Printf("  %-22s %s\n", cyan("/model <name>"), "Switch model: auto, fast, cheap, smart, quality, gpt-4o...")
	fmt.Printf("  %-22s %s\n", cyan("/auto on|off"), "Autonomous file editing (no [Y/n] prompt). Alias for /auto-apply.")
	fmt.Printf("  %-22s %s\n", cyan("/caveman on|off"), "Toggle concise direct output for maximum token savings")
	fmt.Printf("  %-22s %s\n", cyan("/models"), "List available model aliases from gateway")
	fmt.Printf("  %-22s %s\n", cyan("/stats"), "Session stats and savings vs GPT-4o baseline")
	fmt.Printf("  %-22s %s\n", cyan("/usage"), "Gateway usage report for your project")
	fmt.Printf("  %-22s %s\n", cyan("/whoami"), "Show current session info")
	fmt.Printf("  %-22s %s\n", cyan("/clear"), "Clear conversation history")
	fmt.Printf("  %-22s %s\n", cyan("/exit  /quit"), "Quit chat")
	fmt.Println()
	fmt.Println(bold("  Interactive Autocomplete & Context:"))
	fmt.Printf("  %-22s %s\n", cyan("[Tab] key"), "Autocompletes slash commands (/m -> /model) and arguments")
	fmt.Printf("  %-22s %s\n", cyan("@file / @dir"), "Type @ and press [Tab] to list and complete workspace files/folders")
	fmt.Printf("  %-22s %s\n", cyan("edit / create"), "Nexus proposes diffs and prompts [Y/n] to apply (or auto-writes if /auto on)")
	fmt.Println()
	fmt.Println(bold("  Model Aliases & Tiers:"))
	fmt.Printf("  %-14s -> Dynamic routing: fast for simple, smart for complex  (prompt shows [routed])\n", yellow("auto"))
	fmt.Printf("  %-14s -> Google gemini-3.6-flash ($0.075/Mtok in) - ultra fast\n", yellow("fast"))
	fmt.Printf("  %-14s -> OpenAI gpt-4o-mini ($0.15/Mtok in) - cheap & capable\n", yellow("cheap"))
	fmt.Printf("  %-14s -> OpenAI gpt-4o ($2.50/Mtok in) - deep reasoning\n", yellow("smart"))
	fmt.Printf("  %-14s -> Anthropic claude-3-5-sonnet ($3.00/Mtok in) - top quality\n", yellow("quality"))
	fmt.Println()
}

func printResponseMeta(r ChatResult, alias string) {
	latencyStr := fmt.Sprintf("%dms", r.Latency.Milliseconds())
	costStr := fmt.Sprintf("$%.6f", r.Cost)
	modelLabel := r.ModelUsed
	if modelLabel == "" {
		modelLabel = alias
	}
	p := getPricing(modelLabel)
	routeIndicator := ""
	if alias == "auto" {
		routeIndicator = "[auto] "
	}
	fmt.Printf("\n  ~ %s%s  %s  in:%s  out:%s  %s\n\n",
		yellow(routeIndicator),
		cyan(fmt.Sprintf("%s (%s)", modelLabel, p.Provider)),
		dim(latencyStr),
		dim(formatInt(r.InputTokens)),
		dim(formatInt(r.OutputTokens)),
		yellow(costStr),
	)
}

func printSessionStats(stats *SessionStats) {
	savings := stats.BaselineCost - stats.ActualCost
	savingsPct := 0.0
	if stats.BaselineCost > 0 {
		savingsPct = (savings / stats.BaselineCost) * 100
	}

	fmt.Println()
	fmt.Println(bold(cyan("  +============================================+")))
	fmt.Println(bold("  SESSION STATS"))
	fmt.Println(cyan("  +============================================+"))
	fmt.Printf("  %-22s %s\n", dim("Requests:"), bold(fmt.Sprintf("%d", stats.Requests)))
	fmt.Printf("  %-22s %s / %s tokens\n",
		dim("Tokens in / out:"),
		bold(formatInt(stats.InputTokens)),
		bold(formatInt(stats.OutputTokens)),
	)

	if len(stats.ModelCounts) > 0 {
		first := true
		for m, count := range stats.ModelCounts {
			p := getPricing(m)
			label := "Models used:"
			if !first {
				label = ""
			}
			fmt.Printf("  %-22s %s (%s) x%d\n", dim(label), yellow(m), dim(p.Display), count)
			first = false
		}
	}

	fmt.Printf("  %-22s %s\n", dim("Actual cost:"), green(fmt.Sprintf("$%.6f", stats.ActualCost)))
	fmt.Printf("  %-22s %s\n", dim("GPT-4o all-in:"), dim(fmt.Sprintf("$%.6f", stats.BaselineCost)))

	if savings > 0 {
		fmt.Printf("  %-22s %s (%.0f%% cheaper)\n",
			dim("You saved:"),
			bold(green(fmt.Sprintf("$%.6f", savings))),
			savingsPct,
		)
	} else if savings < 0 {
		fmt.Printf("  %-22s %s\n", dim("Extra vs baseline:"), yellow(fmt.Sprintf("+$%.6f", -savings)))
	}

	fmt.Println(cyan("  +============================================+"))
	fmt.Println()
}

func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

// sendChatConversation makes a chat completion request to the gateway.

func sendChatConversation(baseURL, apiKey, model string, messages []map[string]string, caveman bool) (ChatResult, error) {
	cavemanVal := "true"
	if !caveman {
		cavemanVal = "false"
	}

	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
		"metadata": map[string]string{
			"caveman": cavemanVal,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	if caveman {
		req.Header.Set("X-Nexus-Caveman", "true")
	} else {
		req.Header.Set("X-Nexus-Caveman", "false")
	}

	client := &http.Client{Timeout: 120 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResult{}, err
	}

	if resp.StatusCode != http.StatusOK {
		var errEnvelope struct {
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &errEnvelope) == nil && errEnvelope.Error.Message != "" {
			return ChatResult{}, fmt.Errorf("[%s] %s", errEnvelope.Error.Code, errEnvelope.Error.Message)
		}
		return ChatResult{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		NexusRouting *RoutingMeta `json:"nexus_routing"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return ChatResult{}, err
	}
	if len(chatResp.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("no response choices returned")
	}

	inputTok := chatResp.Usage.PromptTokens
	outputTok := chatResp.Usage.CompletionTokens
	cost := estimateCost(chatResp.Model, inputTok, outputTok)

	return ChatResult{
		Content:      chatResp.Choices[0].Message.Content,
		ModelUsed:    chatResp.Model,
		InputTokens:  inputTok,
		OutputTokens: outputTok,
		Latency:      latency,
		Cost:         cost,
		Routing:      chatResp.NexusRouting,
	}, nil
}

// printUsage prints the top-level help.

func printUsage() {
	fmt.Println()
	fmt.Println(bold(cyan("  Nexus AI Gateway CLI")))
	fmt.Println()
	fmt.Println(bold("  Auth:"))
	fmt.Println("    login              Save API key & settings locally")
	fmt.Println("    whoami             Display current session")
	fmt.Println("    logout             Clear local credentials")
	fmt.Println()
	fmt.Println(bold("  Chat & Usage:"))
	fmt.Println("    chat               Start interactive chat REPL (defaults to /auto mode)")
	fmt.Println("    models             List available model aliases")
	fmt.Println("    usage              View token metrics & spend")
	fmt.Println("    health             Check gateway liveness")
	fmt.Println()
	fmt.Println(bold("  Administration:"))
	fmt.Println("    providers add      Register a BYOK provider key")
	fmt.Println("    create-org         Create a tenant organization")
	fmt.Println("    create-project     Create a project")
	fmt.Println("    create-key         Generate a gateway API key")
	fmt.Println()
	fmt.Println(bold("  Quickstart:"))
	fmt.Println(dim("    nexus-cli login -key ngk_live_... -project prj_..."))
	fmt.Println(dim("    nexus-cli chat"))
	fmt.Println(dim("    nexus-cli usage -project prj_..."))
	fmt.Println()
}

// Credentials helpers

func getCredentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".nexus", "credentials.json")
}

func loadCredentials() Credentials {
	path := getCredentialsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{BaseURL: "http://localhost:8080"}
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{BaseURL: "http://localhost:8080"}
	}
	if creds.BaseURL == "" {
		creds.BaseURL = "http://localhost:8080"
	}
	return creds
}

func saveCredentials(creds Credentials) error {
	path := getCredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func removeCredentials() error {
	path := getCredentialsPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// API helpers

func checkHealth(baseURL string) {
	s := newSpinner("Checking gateway health...")
	s.Start()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	s.Stop()
	if err != nil {
		fmt.Printf("  Gateway unavailable at %s: %v\n", baseURL, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		fmt.Printf("  %s Gateway healthy (%d): %s\n", green("OK"), resp.StatusCode, string(body))
	} else {
		fmt.Printf("  %s Status %d: %s\n", yellow("!!"), resp.StatusCode, string(body))
	}
}

func listModels(baseURL, apiKey string) {
	s := newSpinner("Fetching models...")
	s.Start()
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	s.Stop()
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	printPrettyJSON(body)
}

func showUsage(baseURL, apiKey, projectID string, limit int) {
	s := newSpinner("Loading usage report...")
	s.Start()
	url := fmt.Sprintf("%s/v1/usage?project_id=%s&limit=%d", baseURL, projectID, limit)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	s.Stop()
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var usageData struct {
		ProjectID          string  `json:"project_id"`
		TotalRequests      int     `json:"total_requests"`
		TotalInputTokens   int64   `json:"total_input_tokens"`
		TotalOutputTokens  int64   `json:"total_output_tokens"`
		TotalTokens        int64   `json:"total_tokens"`
		TotalEstimatedCost float64 `json:"total_estimated_cost"`
	}

	if err := json.Unmarshal(body, &usageData); err == nil && usageData.ProjectID != "" {
		fmt.Println()
		fmt.Println(bold(cyan("  +====================================================+")))
		fmt.Printf(bold("  NEXUS USAGE REPORT")+" -- Project: %s\n", cyan(usageData.ProjectID))
		fmt.Println(cyan("  +====================================================+"))
		fmt.Printf("  %-26s %s\n", dim("Total Requests:"), bold(fmt.Sprintf("%d", usageData.TotalRequests)))
		fmt.Printf("  %-26s %s\n", dim("Input Tokens:"), bold(formatInt(int(usageData.TotalInputTokens))))
		fmt.Printf("  %-26s %s\n", dim("Output Tokens:"), bold(formatInt(int(usageData.TotalOutputTokens))))
		fmt.Printf("  %-26s %s\n", dim("Total Tokens:"), bold(formatInt(int(usageData.TotalTokens))))
		fmt.Printf("  %-26s %s\n", dim("Estimated Cost (USD):"), green(fmt.Sprintf("$%.6f", usageData.TotalEstimatedCost)))
		fmt.Println(cyan("  +====================================================+"))
		fmt.Println()
	} else {
		printPrettyJSON(body)
	}
}

func sendSingleChat(baseURL, apiKey, model, prompt string, caveman bool) {
	if apiKey == "" {
		fmt.Println(red("Error: Gateway API key is required. Run 'nexus-cli login' or pass -key."))
		return
	}
	s := newSpinner("Sending request...")
	s.Start()
	result, err := sendChatConversation(baseURL, apiKey, model, []map[string]string{
		{"role": "user", "content": prompt},
	}, caveman)
	s.Stop()
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	if result.Routing != nil {
		printRoutingDecision(result.Routing)
	}
	fmt.Printf("\n%s %s\n", bold(cyan("Nexus:")), result.Content)
	printResponseMeta(result, model)
}

func addProvider(baseURL, projectID, providerName, secret string) {
	payload := map[string]string{"project_id": projectID, "provider": providerName, "secret_ref": secret}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(baseURL+"/v1/providers", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	printPrettyJSON(body)
}

func createOrg(baseURL, name string) {
	payload := map[string]string{"name": name}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(baseURL+"/v1/organizations", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	printPrettyJSON(body)
}

func createProject(baseURL, orgID, name, env string) {
	payload := map[string]string{"organization_id": orgID, "name": name, "environment": env}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(baseURL+"/v1/projects", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	printPrettyJSON(body)
}

func createKey(baseURL, projectID, env string) {
	payload := map[string]interface{}{"project_id": projectID, "env": env}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(baseURL+"/v1/api-keys", "application/json", bytes.NewReader(data))
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	printPrettyJSON(body)
}

func printPrettyJSON(data []byte) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "  ", "  "); err == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(data))
	}
}