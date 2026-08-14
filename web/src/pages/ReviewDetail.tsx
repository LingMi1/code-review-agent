import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, Link } from 'react-router-dom';
import {
  ChevronLeft,
  GitPullRequest,
  Clock,
  AlertTriangle,
  CheckCircle2,
  Loader2,
  X,
  Sparkles,
  Terminal,
  FileCode2,
  Hash,
} from 'lucide-react';
import { fetchReview, subscribeSSE, ReviewRecord, SSEEvent } from '../api';

export default function ReviewDetail() {
  const { id } = useParams<{ id: string }>();
  const [review, setReview] = useState<ReviewRecord | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sseLog, setSseLog] = useState<SSEEvent[]>([]);
  const [sseActive, setSseActive] = useState(false);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    const load = async () => {
      try {
        const data = await fetchReview(Number(id));
        if (!cancelled) {
          setReview(data);
          setLoading(false);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Unknown error');
          setLoading(false);
        }
      }
    };
    load();
    return () => { cancelled = true; };
  }, [id]);

  const closeRef = useRef<(() => void) | null>(null);

  const connectSSE = useCallback(() => {
    if (!review) return;
    const session = `pr-review-${review.repo_url}/${review.pr_number}`;
    setSseActive(true);
    // 保存关闭函数，供组件卸载或状态切换时清理，避免 EventSource 泄漏。
    closeRef.current = subscribeSSE(
      session,
      (evt) => {
        setSseLog((prev) => [...prev, evt]);
        if (evt.event === 'review.completed' || evt.event === 'review.failed') {
          setSseActive(false);
          if (id) fetchReview(Number(id)).then(setReview).catch(() => {});
        }
      },
      () => setSseActive(false),
    );
  }, [review, id]);

  useEffect(() => {
    if (review?.status === 'running') {
      connectSSE();
    }
    return () => {
      closeRef.current?.();
      closeRef.current = null;
    };
  }, [review?.status, connectSSE]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-24">
        <Loader2 className="w-6 h-6 text-brand animate-spin" />
      </div>
    );
  }

  if (error || !review) {
    return (
      <div className="text-center py-24">
        <span className="inline-flex w-14 h-14 rounded-xl bg-red-50 items-center justify-center mb-4">
          <X className="w-7 h-7 text-red-500" />
        </span>
        <p className="text-red-600">{error || 'Review not found'}</p>
        <Link to="/" className="inline-flex items-center gap-1.5 text-brand hover:text-brand-hover mt-4 text-sm">
          <ChevronLeft className="w-4 h-4" />
          Back to reviews
        </Link>
      </div>
    );
  }

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-500 mb-6">
        <Link to="/" className="inline-flex items-center gap-1 hover:text-brand transition-colors">
          <ChevronLeft className="w-4 h-4" />
          Reviews
        </Link>
        <span>/</span>
        <span className="font-mono text-gray-700">#{review.pr_number}</span>
      </div>

      {/* Header */}
      <div className="bg-white border border-gray-200 rounded-xl p-6 mb-6 shadow-sm">
        <div className="flex items-start justify-between gap-4 mb-5">
          <div className="min-w-0">
            <div className="flex items-center gap-3 mb-1">
              <h1 className="text-2xl font-bold text-gray-900">PR #{review.pr_number}</h1>
              {review.repo_url && (
                <span className="inline-flex items-center gap-1.5 text-sm text-gray-500">
                  <GitPullRequest className="w-4 h-4" />
                  {review.repo_url}
                </span>
              )}
            </div>
            <div className="flex items-center gap-3 text-xs text-gray-400 mt-1">
              <span className="inline-flex items-center gap-1.5">
                <Clock className="w-3.5 h-3.5" />
                {formatTime(review.created_at)}
              </span>
              {review.head_sha && (
                <span className="inline-flex items-center gap-1 font-mono bg-gray-100 px-2 py-0.5 rounded">
                  <Hash className="w-3 h-3" />
                  {review.head_sha.slice(0, 12)}
                </span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-3 shrink-0">
            <StatusBadge status={review.status} />
            {review.status === 'running' && (
              <button
                onClick={connectSSE}
                disabled={sseActive}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs bg-brand hover:bg-brand-hover text-white rounded-lg disabled:opacity-50 transition-colors"
              >
                <Terminal className="w-3.5 h-3.5" />
                {sseActive ? 'Streaming...' : 'Watch live'}
              </button>
            )}
          </div>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-3 gap-4">
          <StatCard label="Issues Found" value={review.issues || 0} icon={<AlertTriangle className="w-4 h-4" />} tone="amber" />
          <StatCard label="Duration" value={review.duration || '—'} icon={<Clock className="w-4 h-4" />} tone="brand" />
          <StatCard label="Status" value={review.status} icon={<CheckCircle2 className="w-4 h-4" />} tone="green" />
        </div>
      </div>

      {/* Error */}
      {review.error && (
        <div className="bg-red-50 border border-red-200 rounded-xl p-5 mb-6">
          <p className="flex items-center gap-2 text-red-700 font-medium text-sm mb-1">
            <X className="w-4 h-4" />
            Review failed
          </p>
          <p className="text-red-600 font-mono text-xs whitespace-pre-wrap">{review.error}</p>
        </div>
      )}

      {/* Result */}
      {review.summary && (
        <div className="bg-white border border-gray-200 rounded-xl p-6 mb-6 shadow-sm">
          <div className="flex items-center gap-2 mb-3">
            <FileCode2 className="w-4 h-4 text-brand" />
            <h2 className="text-base font-semibold text-gray-900">Review Result</h2>
          </div>
          <p className="text-sm text-gray-700 whitespace-pre-wrap leading-relaxed">{review.summary}</p>
        </div>
      )}

      {/* Agent thinking stream (SSE) */}
      {(sseLog.length > 0 || review.status === 'running') && (
        <div className="bg-brand-soft border border-brand/20 rounded-xl p-6 mb-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Sparkles className="w-4 h-4 text-brand" />
              <h2 className="text-base font-semibold text-gray-900">Agent Thinking Stream</h2>
            </div>
            {review.status === 'running' && sseActive && (
              <span className="inline-flex items-center gap-1.5 text-xs text-brand">
                <span className="w-2 h-2 rounded-full bg-brand animate-pulse" />
                Live
              </span>
            )}
          </div>
          {sseLog.length === 0 ? (
            <p className="text-sm text-gray-500">
              {sseActive ? 'Waiting for agent events...' : 'Click "Watch live" to connect to the agent stream.'}
            </p>
          ) : (
            <div className="space-y-2 max-h-72 overflow-y-auto">
              {sseLog.map((evt, i) => (
                <div key={i} className="flex items-start gap-2.5">
                  <span className="mt-0.5 shrink-0">{eventIcon(evt.event)}</span>
                  <div className="min-w-0">
                    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-brand mb-0.5">
                      {evt.event}
                    </span>
                    <code className="block text-xs text-gray-600 break-all bg-white/60 rounded px-2 py-1 font-mono">
                      {formatSSEData(evt.data)}
                    </code>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function StatCard({ label, value, icon, tone }: { label: string; value: string | number; icon: React.ReactNode; tone: 'brand' | 'amber' | 'green' }) {
  const toneCls = {
    brand: 'text-brand bg-brand-soft',
    amber: 'text-amber-600 bg-amber-50',
    green: 'text-emerald-600 bg-emerald-50',
  }[tone];
  return (
    <div className="bg-gray-50 rounded-xl p-4">
      <div className="flex items-center gap-2.5">
        <span className={`w-8 h-8 rounded-lg flex items-center justify-center ${toneCls}`}>
          {icon}
        </span>
        <div>
          <div className="text-xs text-gray-500">{label}</div>
          <div className="text-lg font-bold text-gray-900">{value}</div>
        </div>
      </div>
    </div>
  );
}

function eventIcon(event: string) {
  switch (event) {
    case 'review.completed':
      return <CheckCircle2 className="w-4 h-4 text-emerald-600" />;
    case 'review.failed':
      return <X className="w-4 h-4 text-red-500" />;
    case 'review.progress':
      return <Loader2 className="w-4 h-4 text-brand animate-spin" />;
    default:
      return <Sparkles className="w-4 h-4 text-brand" />;
  }
}

function formatSSEData(data: string): string {
  try {
    const obj = JSON.parse(data);
    const parts: string[] = [];
    if (obj.pr !== undefined) parts.push(`pr=${obj.pr}`);
    if (obj.status) parts.push(`status=${obj.status}`);
    if (obj.files !== undefined) parts.push(`files=${obj.files}`);
    if (obj.chunks !== undefined) parts.push(`chunks=${obj.chunks}`);
    if (obj.mode) parts.push(`mode=${obj.mode}`);
    if (obj.issues !== undefined) parts.push(`issues=${obj.issues}`);
    if (obj.duration_ms !== undefined) parts.push(`duration_ms=${obj.duration_ms}`);
    if (obj.error) parts.push(`error=${obj.error}`);
    return parts.length > 0 ? parts.join(' · ') : data;
  } catch {
    return data;
  }
}

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { cls: string; icon: React.ReactNode; label: string }> = {
    running: { cls: 'bg-blue-50 text-blue-700 border-blue-200', icon: <Loader2 className="w-3 h-3 animate-spin" />, label: 'Running' },
    success: { cls: 'bg-emerald-50 text-emerald-700 border-emerald-200', icon: <CheckCircle2 className="w-3 h-3" />, label: 'Succeeded' },
    failed: { cls: 'bg-red-50 text-red-700 border-red-200', icon: <X className="w-3 h-3" />, label: 'Failed' },
    pending: { cls: 'bg-gray-100 text-gray-600 border-gray-200', icon: <Clock className="w-3 h-3" />, label: 'Pending' },
  };
  const c = cfg[status] || cfg.pending;
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border ${c.cls}`}>
      {c.icon}
      {c.label}
    </span>
  );
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString();
  } catch {
    return iso;
  }
}
