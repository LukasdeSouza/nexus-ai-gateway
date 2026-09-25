# switchyard-sdk

> **Official TypeScript & JavaScript SDK for the Switchyard AI Gateway**

Integrate intelligent multi-model routing, execution policies, token cost tracking, and remote task dispatch into your Node.js, Next.js, and TypeScript applications.

- Official Documentation: https://switchyard-docs--lukasdesouza.replit.app/
- GitHub Repository: https://github.com/LukasdeSouza/nexus-ai-gateway
- CLI Package: `npm install -g switchyard-ai`

---

## Installation

```bash
npm install switchyard-sdk
```

Or using other package managers:

```bash
# pnpm
pnpm add switchyard-sdk

# yarn
yarn add switchyard-sdk

# bun
bun add switchyard-sdk
```

---

## Quickstart

```typescript
import { Switchyard } from 'switchyard-sdk';

const switchyard = new Switchyard({
  apiKey: process.env.SWITCHYARD_API_KEY, // Optional if running local gateway
  baseURL: 'http://localhost:8080',       // Defaults to http://localhost:8080 or process.env.SWITCHYARD_BASE_URL
  defaultPreset: 'auto',
  defaultPolicy: 'approve',
  caveman: true,                          // Save 35-65% tokens with concise responses
});

async function main() {
  const completion = await switchyard.chat.completions.create({
    model: 'auto',
    messages: [
      { role: 'user', content: 'Design a clean token-bucket rate limiter in TypeScript' }
    ],
  });

  console.log('Response:\n', completion.choices[0].message.content);

  // Inspect the explainable routing trace:
  if (completion.routing) {
    console.log('Selected Candidate:', completion.routing.selectedModel);
    console.log('Provider:', completion.routing.provider);
    console.log('Estimated Cost:', completion.routing.estimatedCost);
    console.log('Routing Rationale:', completion.routing.rationale);
  }
}

main();
```

---

## Bring Your Own Key (BYOK)

Configure custom API keys on the client to route directly through your personal provider quotas:

```typescript
const switchyard = new Switchyard({
  providerKeys: {
    gemini: process.env.GEMINI_API_KEY,
    openai: process.env.OPENAI_API_KEY,
    anthropic: process.env.ANTHROPIC_API_KEY,
    deepseek: process.env.DEEPSEEK_API_KEY,
  },
});
```

---

## Execution Policies

Set safety and autonomy boundaries per request:

```typescript
const completion = await switchyard.chat.completions.create({
  policy: 'explain', // 'explain' | 'plan' | 'approve' | 'safe-auto' | 'autopilot'
  model: 'build',    // 'auto' | 'explore' | 'build' | 'reason' | 'review'
  budget: 0.25,      // Hard spending cap in USD
  caveman: true,
  messages: [{ role: 'user', content: 'Audit this authentication handler for security bugs' }],
});
```

---

## Remote Task Dispatch

Dispatch asynchronous coding tasks to developer CLI daemons running in local repositories:

```typescript
// 1. Create a remote task
const task = await switchyard.tasks.create({
  projectId: 'prj_23470fe6-e38f-4799-8c08-19f5e73395b4',
  prompt: 'Refactor database migration 000011 to add index on status',
  preset: 'build',
  policy: 'safe-auto',
  budget: 0.50,
});

console.log('Task submitted with ID:', task.id);

// 2. Poll for pending tasks (used by CLI workers)
const pendingTasks = await switchyard.tasks.listPending('prj_23470fe6-e38f-4799-8c08-19f5e73395b4');

// 3. Claim and complete tasks
if (pendingTasks.length > 0) {
  const current = pendingTasks[0];
  await switchyard.tasks.claim(current.id, 'worker-ci-01');

  // Submit completion results and file diffs:
  await switchyard.tasks.complete(current.id, {
    summary: 'Successfully added index to remote_tasks table',
    diff: '--- a/schema.sql\n+++ b/schema.sql\n@@ -1,3 +1,4 @@\n+CREATE INDEX idx_status ON remote_tasks(status);',
    failed: false,
  });
}
```

---

## Usage Analytics

Fetch aggregated project consumption and token metrics:

```typescript
const usage = await switchyard.usage.get('prj_23470fe6-e38f-4799-8c08-19f5e73395b4');

console.log('Total Requests:', usage.totalRequests);
console.log('Total Tokens:', usage.totalTokens);
console.log('Total Cost USD:', usage.totalEstimatedCost);
```

---

## Documentation & Links

- Full Documentation: https://switchyard-docs--lukasdesouza.replit.app/
- GitHub: https://github.com/LukasdeSouza/nexus-ai-gateway
- CLI Package: `npm install -g switchyard-ai`

---
License: MIT
