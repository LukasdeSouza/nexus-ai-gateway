export type Preset = 'auto' | 'explore' | 'build' | 'reason' | 'review' | string;
export type ExecutionPolicy = 'explain' | 'plan' | 'approve' | 'safe-auto' | 'autopilot';

export interface ProviderKeys {
  gemini?: string;
  openai?: string;
  anthropic?: string;
  deepseek?: string;
}

export interface SwitchyardOptions {
  apiKey?: string;
  baseURL?: string;
  defaultPreset?: Preset;
  defaultPolicy?: ExecutionPolicy;
  caveman?: boolean;
  providerKeys?: ProviderKeys;
  timeoutMs?: number;
}

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant';
  content: string;
}

export interface ChatCompletionRequest {
  model?: Preset;
  messages: ChatMessage[];
  stream?: boolean;
  maxTokens?: number;
  temperature?: number;
  topP?: number;
  policy?: ExecutionPolicy;
  caveman?: boolean;
  budget?: number;
  providerKeys?: ProviderKeys;
}

export interface RoutingDecision {
  requestedModel: string;
  selectedModel: string;
  provider: string;
  preset: string;
  tier: string;
  complexityScore: number;
  confidencePercent: number;
  intent: string;
  rationale: string;
  estimatedCost: number;
  baselineCost: number;
}

export interface ChatCompletionChoice {
  index: number;
  message: ChatMessage;
  finishReason: string;
}

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
}

export interface ChatCompletionResponse {
  id: string;
  model: string;
  providerId: string;
  choices: ChatCompletionChoice[];
  usage: TokenUsage;
  routing?: RoutingDecision;
}

export interface RemoteTask {
  id: string;
  projectId: string;
  prompt: string;
  preset: string;
  policy: string;
  budget: number;
  status: 'pending' | 'claimed' | 'completed' | 'failed';
  workerId?: string;
  summary?: string;
  diff?: string;
  createdAt: string;
}

export interface CreateTaskParams {
  projectId: string;
  prompt: string;
  preset?: Preset;
  policy?: ExecutionPolicy;
  budget?: number;
}

export interface UsageMetrics {
  projectId: string;
  totalRequests: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalTokens: number;
  totalEstimatedCost: number;
}
