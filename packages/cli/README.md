# switchyard-ai

> **Switchyard: The Routing, Cost & Safety Layer for AI Coding Agents**

Switchyard eliminates AI model routing inefficiencies, prevents destructive code modifications with pre-task Git Checkpoints, and enables Bring Your Own Key (BYOK) for zero server costs.

- Official Documentation: https://switchyard-docs--lukasdesouza.replit.app/
- GitHub Repository: https://github.com/LukasdeSouza/nexus-ai-gateway

---

## Installation & Quickstart

Run instantly with `npx` (no prerequisites, runs on Windows, macOS, and Linux):

```bash
npx switchyard-ai
```

Or install globally to use the `switchyard` and `sy` commands from any terminal:

```bash
npm install -g switchyard-ai
```

Verify installation:

```bash
switchyard --help
# or use the fast alias:
sy --help
```

---

## Authentication & API Keys (BYOK)

### 1. Web Authentication (GitHub OAuth)
```bash
switchyard login
```
Opens your browser to authenticate and links your terminal session.

### 2. Bring Your Own Key (Zero Server Costs)
Configure your personal API keys so Switchyard routes directly through your quotas:

```bash
# Configure Gemini
switchyard keys set gemini AIzaSyYourKeyHere

# Configure OpenAI
switchyard keys set openai sk-proj-YourKeyHere

# Configure Anthropic
switchyard keys set anthropic sk-ant-YourKeyHere

# Configure DeepSeek
switchyard keys set deepseek sk-YourKeyHere

# Inspect active keys status
switchyard keys
```

---

## Interactive Agent Chat (REPL)

Start an interactive coding session in any project repository:

```bash
switchyard chat
# or simply:
sy
```

### Context Injection (@ Mentions)
Attach local codebase context into your prompt without manual copying:
- `@src/server.ts`: Injects the file with syntax-indexed line numbers.
- `@internal/storage/`: Injects directory structure and file sizes (skips `.git`, `node_modules`, `vendor`).

### Slash Commands
Control the agent live inside the chat prompt:

| Command | Short | Description |
|---|---|---|
| `/help` | | Show interactive command cheat sheet |
| `/policy <mode>` | `/pol` | Switch policy: `explain`, `plan`, `approve`, `safe-auto`, `autopilot` |
| `/preset <name>` | | Switch preset: `auto`, `explore`, `build`, `reason`, `review` |
| `/model <alias>` | | Pin to a specific model (e.g. `gpt-4o`, `claude-3-5-sonnet`) |
| `/budget <usd>` | | Set per-task hard spending limit in USD (e.g. `/budget 0.25`) |
| `/caveman on\|off` | | Toggle token-saver mode (reduces output tokens by 35-65%) |
| `/rollback` | | Instantly restore workspace to pre-task Git Checkpoint |
| `/stats` | | View session token spend, model calls, and savings vs baseline |
| `/whoami` | | Display current project ID and active API key |
| `/clear` | | Clear current conversation history |
| `/exit` | `/quit` | Exit the REPL |

### Keyboard Shortcuts
- `[Tab]` Key: Context-sensitive autocompletion for slash commands, policies, presets, and `@` file paths.
- `Up / Down Arrows`: Browse command history.
- `Ctrl + C`: Cancel current generation or prompt.
- `Ctrl + D`: Exit cleanly.

---

## One-Shot Task Runner

Execute tasks directly from your terminal or CI/CD scripts without entering the interactive shell:

```bash
# Preview diffs before applying:
switchyard run --policy approve "Add input validation to @src/routes/auth.ts"

# Autonomous execution on feature branch:
switchyard run --policy autopilot "Generate unit tests for @pkg/calculator.go"
```

---

## Remote Task Daemon (Cloud Dispatch)

Connect your local workspace to the Switchyard Web Dashboard. When you dispatch prompts from the web, your local daemon picks them up, edits code locally, and syncs diffs back:

```bash
switchyard daemon
```

---

## Execution Policies

- **`explain`**: Read-only mode. Answers technical questions and generates snippets without modifying disk.
- **`plan`**: Dry-run mode. Generates architecture plans and preview diffs without writing files.
- **`approve`** *(Default)*: Displays unified diffs and prompts `[Y/n/s/a]` before writing to disk.
- **`safe-auto`**: Automatically applies edits, strictly blocking protected files (`.env*`, lockfiles, `.git/`).
- **`autopilot`**: Full multi-file autonomy with automatic Git Checkpoints for instant `/rollback`.

---

## Links & Community

- Documentation: https://switchyard-docs--lukasdesouza.replit.app/
- GitHub: https://github.com/LukasdeSouza/nexus-ai-gateway
- Node.js SDK: `npm install switchyard-sdk`

---
License: MIT
