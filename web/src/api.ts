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

export async function fetchReviews(): Promise<ReviewRecord[]> {
  const res = await fetch(`${BASE}/reviews`);
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
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const token = import.meta.env.VITE_API_TOKEN as string | undefined;
  if (token) headers['Authorization'] = `Bearer ${token}`;
  const res = await fetch(`${BASE}/reviews`, {
    method: 'POST',
    headers,
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function fetchReview(id: number): Promise<ReviewRecord> {
  const res = await fetch(`${BASE}/reviews/${id}`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export type SSEEvent = {
  event: string;
  data: string;
};

export function subscribeSSE(
  session: string,
  onEvent: (evt: SSEEvent) => void,
  onError?: (err: Error) => void,
): () => void {
  const url = `${BASE}/reviews/stream?session=${encodeURIComponent(session)}`;
  const es = new EventSource(url);

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
    if (onError) onError(new Error('SSE connection error'));
  };

  return () => es.close();
}
