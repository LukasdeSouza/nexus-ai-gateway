# SWITCHYARD: The Routing, Cost & Safety Layer for AI Coding Agents
## Complete System Architecture, Knowledge Base & Developer Reference Manual

> **Purpose of this Document**:  
> This file is an exhaustive technical knowledge base describing the entire Switchyard platform. It details all CLI commands, slash commands, shortcuts, flags, execution policies, safety guardrails, routing presets, Bring Your Own Key (BYOK) protocols, configuration schemas, and Gateway API endpoints. It is engineered to be ingested directly by AI models, technical writers, or documentation generators to produce user documentation, tutorials, and developer portals.

---

# Table of Contents
1. [Platform Overview & Architecture](#1-platform-overview--architecture)
2. [CLI Installation, Binaries & Quickstart](#2-cli-installation-binaries--quickstart)
3. [Authentication & Web Loopback Flow](#3-authentication--web-loopback-flow)
4. [CLI Command Reference (Subcommands & Flags)](#4-cli-command-reference-subcommands--flags)
5. [Execution Policies & Safety Guardrails (Autonomy Control)](#5-execution-policies--safety-guardrails-autonomy-control)
6. [Interactive REPL (Chat Mode) & Slash Commands](#6-interactive-repl-chat-mode--slash-commands)
7. [Context Injection Engine (@ Mentions & Autocomplete)](#7-context-injection-engine--mentions--autocomplete)
8. [Task Presets & Explainable Routing Engine](#8-task-presets--explainable-routing-engine)
9. [Token Economics, Baseline Tracking & Caveman Mode](#9-token-economics-baseline-tracking--caveman-mode)
10. [Bring Your Own Key (BYOK) System](#10-bring-your-own-key-byok-system)
11. [Repository-Level Configuration (.switchyard.json)](#11-repository-level-configuration-switchyardjson)
12. [Web-to-CLI Remote Task Relay (Daemon Worker)](#12-web-to-cli-remote-task-relay-daemon-worker)
13. [Gateway REST API Reference](#13-gateway-rest-api-reference)
14. [CI/CD Pipeline & Code Coverage Bot](#14-cicd-pipeline--code-coverage-bot)

---

# 1. Platform Overview & Architecture

Switchyard is an enterprise-grade AI coding agent platform that sits between developer workflows (terminals, web dashboards, IDEs) and frontier LLMs (Anthropic Claude, OpenAI GPT, Google Gemini, DeepSeek). It addresses the three critical pain points of autonomous AI coding: **Model Routing / Cost Inefficiencies**, **Safety / Workspace Corruption**, and **Seamless Multi-Interface Dispatch**.

### High-Level Architecture Diagram
```
+-----------------------------------------------------------------------------------+
|                                 DEVELOPER INTERFACES                              |
|   +----------------------------+                     +------------------------+   |
|   |   Switchyard Terminal CLI  |                     |  Switchyard Web UI     |   |
|   |  (switchyard / sy binary)  |                     |  (Vercel SPA Dashboard)|   |
|   +--------------+-------------+                     +-----------+------------+   |
+------------------|-----------------------------------------------|----------------+
                   |                                               |
                   | (CLI Requests & BYOK Headers)                 | (OAuth & Task Dispatch)
                   v                                               v
+-----------------------------------------------------------------------------------+
|                        SWITCHYARD AI GATEWAY (Go HTTP Engine)                     |
|                                                                                   |
|  - Rate Limiter (Token Bucket per Project/Key)                                    |
|  - Tenant Resolver & API Key Validator (SHA-256 Hashes)                          |
|  - Routing Engine (Intent Classifier, Model Cost & Complexity Scoring)            |
|  - Multi-Tier Fallback Chain (Graceful Auto-Failover on 429/500/503)               |
|  - Session Cost Tracker (Claude Sonnet baseline comparison)                       |
|  - Remote Task Queue (Postgres Backlog for CLI Daemons)                           |
|  - Prometheus Metrics & OpenTelemetry Tracing                                     |
+-----------------------------------------------------------------------------------+
                   |                                               |
                   | (Upstream Inference)                          | (Persistence)
                   v                                               v
+------------------------------------+           +----------------------------------+
|          LLM PROVIDERS             |           |       STORAGE INFRASTRUCTURE     |
|  - Anthropic (Claude 3.5 Sonnet)   |           |  - Supabase PostgreSQL           |
|  - Google (Gemini 2.5/3.6/3.7)     |           |  - Redis (Token Bucket Caching)  |
|  - OpenAI (GPT-4o, GPT-4o-mini)    |           |  - Kafka (Usage Event Streaming) |
|  - DeepSeek (V3 / R1)              |           |                                  |
+------------------------------------+           +----------------------------------+
```

### Key Components
1. **Switchyard CLI (`switchyard` / `sy`)**: Interactive REPL, one-shot task runner, local repo configuration scanner, Git checkpoint safety engine, and daemon worker.
2. **Switchyard AI Gateway (`cmd/gateway-api`)**: Production Go HTTP reverse proxy handling auth, token-bucket rate limits, model fallback chains, and metrics.
3. **Switchyard Frontend (`switchyard-frontend`)**: React + TypeScript + Tailwind SPA providing usage analytics, real-time cost charts, interactive task composer, and OAuth loopback authentication.
4. **PostgreSQL Database (`migrations/000001_...` through `000011_...`)**: Stores organizations, projects, API keys, provider connections, routing policies, request audit logs, and remote tasks.

---

# 2. CLI Installation, Binaries & Quickstart

### Binary Distribution
Switchyard compiles to a single, statically linked binary. Two binary aliases are created:
- `switchyard`: The full command name.
- `sy`: The ergonomic shortcut alias for fast terminal typing.

### Compiling from Source
```bash
# Clone the repository
git clone https://github.com/LukasdeSouza/nexus-ai-gateway.git
cd nexus-ai-gateway

# Build both switchyard and sy binaries
go build -o bin/switchyard.exe ./cmd/switchyard/...
cp bin/switchyard.exe bin/sy.exe

# Add to system PATH (Linux / macOS)
export PATH="$PWD/bin:$PATH"

# Add to system PATH (Windows PowerShell)
$env:Path += ";$PWD\bin"
```

### Quickstart Workflow
```bash
# 1. Login to your Switchyard account via Web Browser
switchyard login

# 2. (Optional) Set your own LLM API keys for zero-server-cost (BYOK)
switchyard keys set gemini AIzaSyYourGeminiKeyHere
switchyard keys set openai sk-proj-YourOpenAIKeyHere

# 3. Start interactive REPL chat in any repository
switchyard chat

# Or run a one-shot terminal command
switchyard run "Refactor the database queries in @internal/storage to use pgx pool"
```

---

# 3. Authentication & Web Loopback Flow

Switchyard features an ultra-secure OAuth loopback authentication mechanism similar to GitHub CLI (`gh auth login`) and Supabase CLI.

### Browser Authentication Flow
1. User enters:
   ```bash
   switchyard login
   ```
2. The CLI binds a temporary HTTP listener on localhost (defaulting to loopback port `45454`, with dynamic fallback to available ephemeral ports).
3. A cryptographically random `state` nonce is generated.
4. The CLI automatically launches the default web browser to the Switchyard Web Frontend:
   ```
   https://switchyard-frontend.vercel.app/?port=45454&state=<NONCE>&gateway=http://localhost:8080#/cli-auth
   ```
5. The user authenticates via GitHub OAuth on the web interface.
6. The web interface generates an authorized API key (`ngk_live_...`) for the user's project and redirects top-level window navigation to:
   ```
   http://127.0.0.1:45454/callback?key=ngk_live_...&project=prj_...&state=<NONCE>
   ```
   *(Note: Top-level redirection is used rather than background `fetch()` to comply with modern browser Private Network Access / CORS restrictions).*
7. The local CLI server receives the credentials, validates the state nonce, outputs `[SUCCESS] Authenticated successfully!`, returns a clean confirmation HTML page, and shuts down the local listener.
8. Credentials are stored securely on the local filesystem at:
   - **Linux / macOS**: `~/.switchyard/credentials.json`
   - **Windows**: `C:\Users\<User>\.switchyard\credentials.json`

### Manual API Key Authentication
For headless environments, Docker containers, or remote SSH boxes where no browser is available:
```bash
switchyard login -key ngk_live_xxxxxxxxxxxxxxxxxxxxxxxx -project prj_xxxxxxxxxxxx
```

### Inspecting Active Session
```bash
switchyard whoami
```
Outputs:
```text
Switchyard Session:
  API Key:   ngk_live...jrQ0
  Gateway:   https://api.switchyard.dev
  Project:   prj_23470fe6-e38f-4799-8c08-19f5e73395b4
```

---

# 4. CLI Command Reference (Subcommands & Flags)

### Syntax
```text
switchyard [command] [flags] [arguments]
sy [command] [flags] [arguments]
```
If no subcommand is passed, Switchyard automatically launches `switchyard chat` using default settings.

---

### Command: `switchyard chat`
Starts an interactive terminal session with syntax-highlighted REPL, autocomplete, slash commands, and background task listeners.

```bash
switchyard chat [--preset <preset>] [--policy <policy>] [--budget <usd>] [--model <alias>]
```
- `--preset <name>`: Model preset: `auto`, `explore`, `build`, `reason`, `review` (default: `auto`).
- `--model <name>`: Specific model alias or pinned provider (e.g. `claude-3-5-sonnet`, `gemini-3.7-flash`, `gpt-4o-mini`).
- `--policy <mode>`: Execution policy: `explain`, `plan`, `approve`, `safe-auto`, `autopilot` (default: `approve`).
- `--budget <usd>`: Per-task spending limit in USD (e.g. `0.25`). Aborts prompt before calling models if estimation exceeds cap. Default: `0` (unlimited).

---

### Command: `switchyard run`
Executes a single autonomous task non-interactively and exits. Ideal for scripting, CI/CD pipelines, or quick code generation.

```bash
switchyard run [flags] "<prompt>"
```
- `--preset <name>`: Task preset (default: `auto`).
- `--policy <mode>`: Execution policy (default: `safe-auto`).
- `--budget <usd>`: Hard task budget limit in USD (default: `0.0`).
- `--caveman`: Enable ultra-concise token-saver responses (default: `true`).

**Example**:
```bash
switchyard run --policy approve "Add a new unit test for ParsePolicy in @cmd/switchyard/safety.go"
```

---

### Command: `switchyard daemon` (or `switchyard worker`)
Starts a persistent background worker that polls the Switchyard Gateway for remote tasks dispatched from the Web Dashboard, executes them locally against the current repository, and reports file diffs and results back to the cloud.

```bash
switchyard daemon [-project <project_id>] [-url <gateway_url>]
```
- `-project <id>`: Switchyard Project ID to poll (defaults to saved credentials).
- `-url <url>`: Gateway URL (defaults to `http://localhost:8080` or credentials).

---

### Command: `switchyard keys`
Manages Bring Your Own Key (BYOK) configurations.

```bash
# List all provider keys status
switchyard keys
switchyard keys list

# Set a provider key
switchyard keys set <gemini|openai|anthropic|deepseek> <API_KEY>

# Remove a provider key
switchyard keys remove <gemini|openai|anthropic|deepseek>
```

---

### Command: `switchyard login`
Authenticates the CLI with the Gateway.
- `-key <string>`: Manual Switchyard API key (`ngk_live_...`).
- `-project <string>`: Project ID.
- `-url <string>`: Gateway base URL.
- `-frontend <string>`: Web frontend URL.

---

### Command: `switchyard whoami`
Displays the active logged-in user, masked API key, project ID, and gateway endpoint.

---

### Command: `switchyard help`
Prints the global CLI manual, supported presets, and policy explanations.

---

# 5. Execution Policies & Safety Guardrails (Autonomy Control)

Switchyard introduces strict autonomy boundaries to eliminate the risk of destructive code changes or leaking sensitive tokens.

### Available Execution Policies

| Policy | Level | Terminal Tag | Description |
|---|---|---|---|
| **`explain`** | Read-Only | `[explain]` | Strictly informational. Explains code, answers architecture questions, and generates snippets. Never writes or modifies files on disk. |
| **`plan`** | Dry-Run | `[plan]` | Architectural planning mode. Model returns step-by-step plans and preview diffs, but Switchyard completely suppresses file writing. |
| **`approve`** | Interactive *(Default)* | `[approve]` | Displays a manifest of intended file actions and interactive unified diffs. Prompts the user `[y]es / [n]o / [s]kip / [a]ll` before touching disk. |
| **`safe-auto`** | Autonomous | `[safe-auto]` | Automatically writes code changes directly to disk, but strictly blocks any modification to protected files (secrets, lockfiles, git). |
| **`autopilot`** | Full Autonomy | `[autopilot]` | Full multi-file autonomous execution with automatic Git Checkpoints created before edits for instant single-command rollback. |

---

### Protected File Deny-Lists (`safety.go`)
Under `safe-auto` and `autopilot`, Switchyard intercepts file modification blocks before they touch disk. The following patterns are **strictly protected** and will be immediately blocked:
- **Secrets & Certificates**: `.env*`, `*.pem`, `*.key`, `*.cert`, `*.pfx`
- **Package Lockfiles**: `go.sum`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `composer.lock`, `Gemfile.lock`, `cargo.lock`
- **Version Control Metadata**: `.git`, `.git/*`, `*/.git/*`

---

### Git Checkpoints & Instant Rollback
Whenever Switchyard modifies code in a git repository:
1. `CreateGitCheckpoint()` creates an isolated, non-destructive stash snapshot before applying changes:
   ```text
   switchyard-checkpoint-YYYYMMDD-HHMMSS: <task description>
   ```
2. If the assistant produces unwanted changes, bugs, or broken tests, the user can instantly restore their workspace:
   ```bash
   # Inside interactive chat:
   /rollback
   ```
3. `RollbackGitCheckpoint()` executes `git checkout -- .` and `git clean -fd`, restoring the workspace to its exact pre-task condition in milliseconds.

---

### Code Modification Protocol (Search/Replace & Full Write)
Switchyard models are instructed to output file edits using deterministic blocks that the CLI parses cleanly:

#### Surgical Search & Replace:
````markdown
```edit:path/to/file.go
<<<<<<< SEARCH
func OldLogic() {
    // original code to replace
}
=======
func NewLogic() {
    // replacement code
}
>>>>>>> REPLACE
```
````

#### Full File Creation / Overwrite:
````markdown
```write:path/to/new_file.go
package main

func NewFile() {}
```
````

---

# 6. Interactive REPL (Chat Mode) & Slash Commands

When you launch `switchyard chat` (or simply `switchyard`), you enter the interactive agent shell.

### Prompt Anatomy
```text
switchyard [caveman] [approve] [routed] > 
```
- `[caveman]`: Green indicator showing concise token-saver mode is active.
- `[policy]`: Current execution boundary (`[explain]`, `[plan]`, `[approve]`, `[safe-auto]`, `[autopilot]`).
- `[preset]`: Active routing preset (`[routed]` for auto, or `[explore]`, `[build]`, `[reason]`, `[review]`).

---

### Slash Command Reference

| Slash Command | Short Alias | Arguments | Description |
|---|---|---|---|
| **`/help`** | | | Displays the interactive command cheat sheet and tips. |
| **`/policy`** | `/pol` | `<mode>` | Switch active policy: `explain`, `plan`, `approve`, `safe-auto`, `autopilot`. |
| **`/preset`** | | `<name>` | Switch model preset: `auto`, `explore`, `build`, `reason`, `review`. |
| **`/model`** | | `<name>` | Pin to a specific model alias (e.g. `gpt-4o`, `claude-3-5-sonnet`). |
| **`/budget`** | | `<amount>` | Set per-task spending cap in USD (e.g. `/budget 0.25`). Set `/budget 0` to disable. |
| **`/caveman`** | | `on` \| `off` | Toggle hyper-concise output mode for maximum token and cost savings. |
| **`/rollback`** | | | Instantly restore workspace to the pre-task Git Checkpoint. |
| **`/stats`** | | | Render the comprehensive Session Economics & Routing Report. |
| **`/whoami`** | | | Show current active session, API key, and project ID. |
| **`/clear`** | | | Clear the active multi-turn conversation memory. |
| **`/exit`** | `/quit` | | Exit the Switchyard REPL cleanly. |

---

### Keyboard Shortcuts
- **`[Tab]` Key**: Context-sensitive autocompletion:
  - If typing `/`: Autocompletes slash commands (`/policy`, `/preset`, `/rollback`, etc.).
  - If typing `/policy <arg>`: Autocompletes policy names (`explain`, `plan`, `approve`...).
  - If typing `/preset <arg>`: Autocompletes preset names (`explore`, `build`, `reason`...).
  - If typing `@`: Autocompletes local repository file paths and directories.
- **`Up / Down Arrows`**: Cycle through persistent command history stored at `~/.switchyard/chat_history`.
- **`Ctrl + C`**: Interrupt current task or clear current line.
- **`Ctrl + D`**: Exit Switchyard.

---

# 7. Context Injection Engine (@ Mentions & Autocomplete)

Switchyard allows you to attach codebase context directly into your prompts using `@` syntax, without manually copying and pasting files.

### Referencing Files: `@path/to/file`
When you type `@path/to/file.ext` in your prompt:
1. Switchyard reads the file from disk.
2. Formats the contents with **syntax-indexed line numbers**:
   ```text
   === File: internal/http/server.go (3420 bytes) ===
      1 | package http
      2 | 
      3 | import (
      4 |     "net/http"
   ...
   ```
3. Appends it cleanly to the prompt's `[Referenced Local Code Context]` block.

### Referencing Directories: `@path/to/dir`
When you type `@path/to/directory/`:
1. Switchyard scans the directory tree (up to 60 items).
2. Generates an indented file and subdirectory manifest with byte sizes.
3. Automatically skips standard ignore directories:
   - `.git`, `node_modules`, `vendor`, `bin`, `tmp`, `.idea`, `.vscode`, `.switchyard`.

---

# 8. Task Presets & Explainable Routing Engine

Switchyard replaces static model selection with **Semantic Intent Routing**. Instead of forcing users to guess which model is best, tasks are classified into Presets:

### Presets Matrix

| Preset | Target Model | Provider | Strengths | Cost (In / Out per 1M) |
|---|---|---|---|---|
| **`auto`** | *Dynamic Router* | Switchyard | Classifies complexity and selects optimal provider | Dynamic |
| **`explore`** | Gemini 3.6 Flash | Google | Lightning-fast file search, code comprehension, reading | $0.075 / $0.30 |
| **`build`** | Claude 3.5 Sonnet | Anthropic | Standard development: feature implementation, refactoring, tests | $3.00 / $15.00 |
| **`reason`** | Claude 3.5 Sonnet / o1 | Anthropic / OpenAI | Architecture, concurrent logic, high-frontier debugging | $3.00 / $15.00 |
| **`review`** | Claude 3.5 Sonnet | Anthropic | Deep quality audits, security scanning, vulnerability detection | $3.00 / $15.00 |

---

### Explainable Routing Decision Trace
After every prompt routed through `auto`, Switchyard prints an **Explainable Routing Trace** in the terminal:
```text
  [Switchyard Route Decision]
  |- Preset:     build (confidence: 94%)
  |- Intent:     code_modification (score: 0.82)
  |- Rationale:  Prompt requests multi-file logic changes; routed to frontier coding model
  |- Candidate:  claude-3-5-sonnet (Anthropic)
  |- Economics:  Est. $0.00310 vs $0.01250 baseline (saved ~$0.00940)
```
If an upstream provider experiences rate limiting or an outage, the trace transparently shows failover events:
```text
  |- Fallback #1: gpt-4o (OpenAI) failed: HTTP 429 rate limit exceeded -> redirected
```

---

# 9. Token Economics, Baseline Tracking & Caveman Mode

### Baseline Comparison
Switchyard benchmarks every request against an unrouted industry baseline (standard Claude 3.5 Sonnet rates: **$3.00 / 1M input tokens** and **$15.00 / 1M output tokens**).

Run `/stats` at any time to generate the session economics report:
```text
  +====================================================+
  |         SWITCHYARD SESSION ECONOMICS REPORT        |
  +====================================================+
  Total Requests:      14
  Tokens Consumed:     42,850 (in: 34,200, out: 8,650)
  Files Modified:      6
  Actual Total Spend:  $0.018240
  Unrouted Baseline:   $0.232350 (Claude Sonnet baseline)
  Estimated Savings:   $0.214110 (92.1% saved through routing)
  ----------------------------------------------------
  Models Dispatched:
    gemini-3.6-flash          11 call(s) (78.6%) - Google
    claude-3-5-sonnet          3 call(s) (21.4%) - Anthropic
  +====================================================+
```

---

### Caveman Mode (`/caveman on|off`)
Enabled by default. Caveman mode appends a strict system directive:
> *"Respond directly and concisely. No fluff, no filler, no pleasantries. Optimize for brevity and token savings."*

- **Result**: Cuts conversational filler ("Certainly! Here is the code you requested...").
- **Savings**: Reduces output token consumption by **35% to 65%**, accelerating response latency.

---

### Hard Task Budget Cap (`--budget <usd>`)
To prevent runaway costs on huge prompts or complex multi-turn chats:
```bash
switchyard chat --budget 0.25
# or inside chat:
/budget 0.25
```
Switchyard estimates total prompt tokens before sending the HTTP request. If `estimated_cost > budget`, execution is immediately halted **before any upstream API charges occur**:
```text
BUDGET EXCEEDED: Estimated prompt cost ($0.31200) exceeds task budget limit ($0.25000). Execution aborted before calling models.
```

---

# 10. Bring Your Own Key (BYOK) System

Switchyard allows individual developers and open-source contributors to use their own personal API keys. This ensures zero API costs for server hosts when sharing or open-sourcing the project.

### Priority Resolution Order
When making inference calls, Switchyard resolves keys in the following order:
1. **Locally Configured BYOK Key** (saved in `~/.switchyard/credentials.json` via `switchyard keys set`)
2. **Environment Variable** (`GEMINI_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `DEEPSEEK_API_KEY`)
3. **Gateway Server Default** (The hosted gateway's shared secret, if configured)

### Configuring Provider Keys
```bash
# Configure Gemini
switchyard keys set gemini AIzaSyYourGoogleKey

# Configure OpenAI
switchyard keys set openai sk-proj-YourOpenAIKey

# Configure Anthropic
switchyard keys set anthropic sk-ant-YourAnthropicKey

# Configure DeepSeek
switchyard keys set deepseek sk-YourDeepSeekKey

# View configured status
switchyard keys
```
Output:
```text
  ========================================================
    SWITCHYARD PROVIDER KEYS  -  Bring Your Own Key (BYOK)
  ========================================================
  gemini:      [CONFIGURED]  (AIzaSy...7xQ)
  openai:      [CONFIGURED]  (sk-pro...9mK)
  anthropic:   [NOT SET]     (uses Gateway server default)
  deepseek:    [ENV SET]     (sk-dee...2aA env)
```

### Protocol Header Forwarding
The CLI passes your local keys to the Gateway using standard HTTP headers:
- `X-Gemini-Api-Key`
- `X-OpenAI-Api-Key`
- `X-Anthropic-Api-Key`
- `X-Deepseek-Api-Key`

The Gateway extracts these headers and injects them directly into the respective provider adapter (`gemini/adapter.go`, `openai/adapter.go`, `anthropic/adapter.go`). Your keys are never logged or stored in the Gateway database.

---

# 11. Repository-Level Configuration (`.switchyard.json`)

Teams can enforce consistent policies, models, and spending caps across a repository by committing a `.switchyard.json` file in their repository root.

### Discovery Mechanism
When `switchyard` launches, `loadLocalRepoConfig()` scans upward from the current working directory to parent directories until it finds `.switchyard.json` or reaches the `.git` root.

### Schema (`.switchyard.json`)
```json
{
  "$schema": "https://switchyard.dev/schema/config.json",
  "preset": "build",
  "policy": "approve",
  "budget": 0.50,
  "caveman": true
}
```

### Field Definitions
- `preset` *(string, optional)*: Default model preset for this repo (`auto`, `explore`, `build`, `reason`, `review`).
- `policy` *(string, optional)*: Default autonomy boundary (`explain`, `plan`, `approve`, `safe-auto`, `autopilot`).
- `budget` *(float, optional)*: Per-task spending limit in USD (e.g. `0.25`).
- `caveman` *(boolean, optional)*: Enable (`true`) or disable (`false`) token-saver output.

---

# 12. Web-to-CLI Remote Task Relay (Daemon Worker)

Switchyard enables **Remote Cloud Dispatch**: a developer can submit a prompt on the web interface, and it will execute locally on their machine where their code, git state, and environment reside.

### Architectural Workflow
```
[Web Dashboard] 
       |
       | 1. User enters prompt in Task Composer & clicks "Dispatch"
       v
[Gateway API] ---> Saves task to `remote_tasks` table (status: "pending")
       ^
       | 2. CLI polls GET /v1/tasks/pending?project_id=...
       | 3. CLI claims task: POST /v1/tasks/:id/claim
[Local CLI Daemon]
       |
       | 4. Runs task locally under active Policy & creates Git Checkpoint
       | 5. Generates file diffs & summary
       v
[Gateway API] <--- 6. CLI reports results: POST /v1/tasks/:id/complete
       |
       v
[Web Dashboard] -> Live card updates displaying execution summary & file diffs
```

### Running the Daemon
You can run the daemon in two ways:
1. **Dedicated Worker Mode**:
   ```bash
   switchyard daemon
   ```
2. **Integrated Chat Background Polling**:
   While you are inside an active `switchyard chat` session, a background goroutine automatically checks for incoming web tasks every 3 seconds while you are idle at the prompt!

---

# 13. Gateway REST API Reference

The Switchyard Gateway provides an OpenAI-compatible REST API along with administrative and telemetry endpoints.

### Authentication
All requests must provide a Switchyard Bearer token in the `Authorization` header:
```http
Authorization: Bearer ngk_live_xxxxxxxxxxxxxxxxxxxxxxxx
```

---

### `POST /v1/chat/completions`
OpenAI-compatible inference endpoint with intelligent routing, caveman mode, and failover.

#### Custom Headers
- `X-Switchyard-Policy`: `explain` | `plan` | `approve` | `safe-auto` | `autopilot`
- `X-Nexus-Caveman`: `true` | `false`
- `X-Gemini-Api-Key`: Optional BYOK key
- `X-OpenAI-Api-Key`: Optional BYOK key
- `X-Anthropic-Api-Key`: Optional BYOK key
- `X-Deepseek-Api-Key`: Optional BYOK key

#### Request Body (JSON)
```json
{
  "model": "auto",
  "messages": [
    {"role": "user", "content": "How do I optimize postgres queries?"}
  ],
  "metadata": {
    "caveman": "true",
    "policy": "approve"
  }
}
```

---

### `GET /v1/usage?project_id=<id>`
Fetches aggregated project consumption metrics from PostgreSQL.

#### Response (JSON)
```json
{
  "project_id": "prj_23470fe6-e38f-4799-8c08-19f5e73395b4",
  "total_requests": 142,
  "total_input_tokens": 1285000,
  "total_output_tokens": 340000,
  "total_tokens": 1625000,
  "total_estimated_cost": 1.48200
}
```

---

### `POST /v1/tasks`
Dispatches a new task to the queue for a local CLI daemon.

#### Request Body (JSON)
```json
{
  "project_id": "prj_23470fe6-e38f-4799-8c08-19f5e73395b4",
  "prompt": "Fix unit tests in router_test.go",
  "preset": "build",
  "policy": "safe-auto",
  "budget": 0.25
}
```

---

### `GET /v1/tasks/pending?project_id=<id>`
Polled by CLI daemons to retrieve unfulfilled tasks.

---

### `POST /v1/tasks/:id/claim`
Atomically transitions a task from `pending` to `claimed` by `worker_id`.

---

### `POST /v1/tasks/:id/complete`
Submits execution results, summary, and unified diffs.

```json
{
  "summary": "Applied 2 file modifications cleanly.",
  "diff": "--- a/router.go\n+++ b/router.go\n...",
  "failed": false
}
```

---

# 14. CI/CD Pipeline & Code Coverage Bot

Switchyard includes a full GitHub Actions Continuous Integration pipeline defined in [`.github/workflows/ci.yml`](file:///c:/Users/Lucas/Codetech/switchyard-ai-gateway/.github/workflows/ci.yml).

### Features
1. **Automated Atomic Testing**: Runs `go test ./... -coverprofile=coverage.out -covermode=atomic` across all packages.
2. **PR Code Coverage Bot**: Automatically comments on Pull Requests with an expandable Markdown breakdown:
   - Overall project test status (`PASSED` / `FAILED`).
   - Total statement coverage percentage.
   - Package-by-package coverage table with status indicators (`[High]`, `[Med]`, `[Low]`).
   - Full raw coverage output.
3. **Job Summary & Artifacts**: Embeds the test summary into GitHub Actions workflow runs and retains coverage artifacts for 30 days.
4. **Cross-Platform Line Ending Safety ([`.gitattributes`](file:///c:/Users/Lucas/Codetech/switchyard-ai-gateway/.gitattributes))**: Enforces LF line endings across all Go files, preventing BOM and syntax errors when collaborating between Windows and Linux runners.

---
*Document Version: 1.0.0 — Generated for Switchyard AI Gateway & CLI Platform.*
