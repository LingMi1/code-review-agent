export interface ReviewRecord {
  id: number;
  pr_number: number;
  repo_url: string;
  head_sha: string;
  status: string;
  issues: number;
  summary: string;
  duration: string;
  error: string;
  created_at: string;
  updated_at: string;
}

const BASE = '/api';
const TOKEN_KEY = 'cra.apiToken';

// API token 运行时获取/设置：不再从构建期 VITE_API_TOKEN 内联进 bundle
// （静态资源公开即 token 公开）。改为 sessionStorage，由操作者粘贴一次、会话内复用。
export function getApiToken(): string {
  return sessionStorage.getItem(TOKEN_KEY) || '';
}

export function setApiToken(token: string): void {
  if (token) sessionStorage.setItem(TOKEN_KEY, token);
  else sessionStorage.removeItem(TOKEN_KEY);
}

function authHeaders(extra?: Record<string, string>): Record<string, string> {
  const headers: Record<string, string> = { ...(extra || {}) };
  const token = getApiToken();
  if (token) headers['Authorization'] = `Bearer ${token}`;
  return headers;
}

export async function fetchHealth(): Promise<boolean> {
  try {
    const res = await fetch('/health');
    return res.ok;
  } catch {
    return false;
  }
}

export async function fetchReviews(): Promise<ReviewRecord[]> {
  const res = await fetch(`${BASE}/reviews`, { headers: authHeaders() });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

export interface TriggerRequest {
  owner: string;
  repo: string;
  pr_number: number;
}

export async function triggerReview(req: TriggerRequest): Promise<{ review_id: number; status: string }> {
  const res = await fetch(`${BASE}/reviews`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function fetchReview(id: number): Promise<ReviewRecord> {
  const res = await fetch(`${BASE}/reviews/${id}`, { headers: authHeaders() });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export type SSEEvent = {
  event: string;
  data: string;
};

// 换取 SSE 短时签名 token（需 API token）。EventSource 无法带 Authorization header，
// 所以先鉴权签出一个绑定 session 的短时 token，再拼到 EventSource URL 的 query 里。
async function fetchStreamToken(session: string): Promise<string> {
  const res = await fetch(`${BASE}/reviews/stream-token`, {
    method: 'POST',
    headers: authHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ session }),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();
  return data.token || '';
}

export function subscribeSSE(
  session: string,
  onEvent: (evt: SSEEvent) => void,
  onError?: (err: Error) => void,
): () => void {
  let es: EventSource | null = null;
  let closed = false;

  (async () => {
    try {
      const token = await fetchStreamToken(session);
      if (closed) return;
      const params = new URLSearchParams({ session });
      if (token) params.set('token', token);
      es = new EventSource(`${BASE}/reviews/stream?${params.toString()}`);

      es.addEventListener('review.started', (e: MessageEvent) => {
        onEvent({ event: 'review.started', data: e.data });
      });
      es.addEventListener('review.progress', (e: MessageEvent) => {
        onEvent({ event: 'review.progress', data: e.data });
      });
      es.addEventListener('review.completed', (e: MessageEvent) => {
        onEvent({ event: 'review.completed', data: e.data });
      });
      es.addEventListener('review.failed', (e: MessageEvent) => {
        onEvent({ event: 'review.failed', data: e.data });
      });

      es.onerror = () => {
        if (!closed && onError) onError(new Error('SSE connection error'));
      };
    } catch (err) {
      if (!closed && onError) {
        onError(err instanceof Error ? err : new Error('failed to obtain stream token'));
      }
    }
  })();

  return () => {
    closed = true;
    es?.close();
  };
}
