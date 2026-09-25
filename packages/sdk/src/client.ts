import {
  SwitchyardOptions,
  ChatCompletionRequest,
  ChatCompletionResponse,
  CreateTaskParams,
  RemoteTask,
  UsageMetrics,
} from './types';

export class Switchyard {
  private apiKey: string;
  private baseURL: string;
  private options: SwitchyardOptions;

  constructor(options: SwitchyardOptions = {}) {
    this.options = options;
    this.apiKey =
      options.apiKey ||
      (typeof process !== 'undefined' ? process.env.SWITCHYARD_API_KEY || process.env.NEXUS_API_KEY || '' : '');
    this.baseURL = (
      options.baseURL ||
      (typeof process !== 'undefined'
        ? process.env.SWITCHYARD_BASE_URL || process.env.NEXUS_BASE_URL || 'http://localhost:8080'
        : 'http://localhost:8080')
    ).replace(/\/$/, '');
  }

  private getHeaders(customProviderKeys?: SwitchyardOptions['providerKeys'], policy?: string, caveman?: boolean): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }

    if (policy || this.options.defaultPolicy) {
      headers['X-Switchyard-Policy'] = policy || this.options.defaultPolicy!;
    }

    if (caveman !== undefined ? caveman : this.options.caveman) {
      headers['X-Nexus-Caveman'] = 'true';
    }

    // Merge BYOK provider keys (request override > client default > process.env)
    const keys = { ...this.options.providerKeys, ...customProviderKeys };

    const gemini = keys.gemini || (typeof process !== 'undefined' ? process.env.GEMINI_API_KEY : undefined);
    if (gemini) headers['X-Gemini-Api-Key'] = gemini;

    const openai = keys.openai || (typeof process !== 'undefined' ? process.env.OPENAI_API_KEY : undefined);
    if (openai) headers['X-OpenAI-Api-Key'] = openai;

    const anthropic = keys.anthropic || (typeof process !== 'undefined' ? process.env.ANTHROPIC_API_KEY : undefined);
    if (anthropic) headers['X-Anthropic-Api-Key'] = anthropic;

    const deepseek = keys.deepseek || (typeof process !== 'undefined' ? process.env.DEEPSEEK_API_KEY : undefined);
    if (deepseek) headers['X-Deepseek-Api-Key'] = deepseek;

    return headers;
  }

  readonly chat = {
    completions: {
      create: async (req: ChatCompletionRequest): Promise<ChatCompletionResponse> => {
        const url = `${this.baseURL}/v1/chat/completions`;
        const headers = this.getHeaders(req.providerKeys, req.policy, req.caveman);

        const body: Record<string, any> = {
          model: req.model || this.options.defaultPreset || 'auto',
          messages: req.messages,
          stream: req.stream || false,
        };

        if (req.maxTokens) body.max_tokens = req.maxTokens;
        if (req.temperature !== undefined) body.temperature = req.temperature;
        if (req.topP !== undefined) body.top_p = req.topP;

        const response = await fetch(url, {
          method: 'POST',
          headers,
          body: JSON.stringify(body),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`Switchyard Gateway error [${response.status}]: ${errorText}`);
        }

        const data = await response.json();
        return {
          id: data.id || '',
          model: data.model || body.model,
          providerId: data.provider_id || '',
          choices: (data.choices || []).map((c: any) => ({
            index: c.index,
            message: c.message,
            finishReason: c.finish_reason || 'stop',
          })),
          usage: {
            inputTokens: data.usage?.input_tokens || 0,
            outputTokens: data.usage?.output_tokens || 0,
            totalTokens: data.usage?.total_tokens || 0,
          },
          routing: data.routing
            ? {
                requestedModel: data.routing.requested_model,
                selectedModel: data.routing.selected_model,
                provider: data.routing.provider,
                preset: data.routing.preset,
                tier: data.routing.tier,
                complexityScore: data.routing.complexity_score,
                confidencePercent: data.routing.confidence_percent,
                intent: data.routing.intent,
                rationale: data.routing.rationale,
                estimatedCost: data.routing.estimated_cost,
                baselineCost: data.routing.baseline_cost,
              }
            : undefined,
        };
      },
    },
  };

  readonly tasks = {
    create: async (params: CreateTaskParams): Promise<{ id: string; status: string }> => {
      const url = `${this.baseURL}/v1/tasks`;
      const response = await fetch(url, {
        method: 'POST',
        headers: this.getHeaders(),
        body: JSON.stringify({
          project_id: params.projectId,
          prompt: params.prompt,
          preset: params.preset || 'auto',
          policy: params.policy || 'approve',
          budget: params.budget || 0,
        }),
      });

      if (!response.ok) {
        throw new Error(`Failed to create task [${response.status}]: ${await response.text()}`);
      }

      return response.json();
    },

    listPending: async (projectId: string): Promise<RemoteTask[]> => {
      const url = `${this.baseURL}/v1/tasks/pending?project_id=${encodeURIComponent(projectId)}`;
      const response = await fetch(url, {
        headers: this.getHeaders(),
      });

      if (!response.ok) {
        throw new Error(`Failed to list pending tasks: ${await response.text()}`);
      }

      const data = await response.json();
      return (data.tasks || []).map((t: any) => ({
        id: t.id,
        projectId: t.project_id,
        prompt: t.prompt,
        preset: t.preset,
        policy: t.policy,
        budget: t.budget,
        status: t.status,
        workerId: t.worker_id,
        summary: t.summary,
        diff: t.diff,
        createdAt: t.created_at,
      }));
    },

    claim: async (taskId: string, workerId: string): Promise<void> => {
      const url = `${this.baseURL}/v1/tasks/${taskId}/claim`;
      const response = await fetch(url, {
        method: 'POST',
        headers: this.getHeaders(),
        body: JSON.stringify({ worker_id: workerId }),
      });

      if (!response.ok) {
        throw new Error(`Failed to claim task: ${await response.text()}`);
      }
    },

    complete: async (
      taskId: string,
      result: { summary: string; diff?: string; failed?: boolean }
    ): Promise<void> => {
      const url = `${this.baseURL}/v1/tasks/${taskId}/complete`;
      const response = await fetch(url, {
        method: 'POST',
        headers: this.getHeaders(),
        body: JSON.stringify({
          summary: result.summary,
          diff: result.diff || '',
          failed: !!result.failed,
        }),
      });

      if (!response.ok) {
        throw new Error(`Failed to complete task: ${await response.text()}`);
      }
    },
  };

  readonly usage = {
    get: async (projectId: string): Promise<UsageMetrics> => {
      const url = `${this.baseURL}/v1/usage?project_id=${encodeURIComponent(projectId)}`;
      const response = await fetch(url, {
        headers: this.getHeaders(),
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch usage: ${await response.text()}`);
      }

      const data = await response.json();
      return {
        projectId: data.project_id,
        totalRequests: data.total_requests || 0,
        totalInputTokens: data.total_input_tokens || 0,
        totalOutputTokens: data.total_output_tokens || 0,
        totalTokens: data.total_tokens || 0,
        totalEstimatedCost: data.total_estimated_cost || 0,
      };
    },
  };
}

export default Switchyard;
