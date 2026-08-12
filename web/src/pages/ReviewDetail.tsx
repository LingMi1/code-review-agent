import { useState, useEffect, useCallback } from 'react';
import { useParams, Link } from 'react-router-dom';
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

  // SSE subscription for live updates
  const connectSSE = useCallback(() => {
    if (!review) return () => {};

    const session = `pr-review-${review.repo_url}/${review.pr_number}`;
    setSseActive(true);

    return subscribeSSE(
      session,
      (evt) => {
        setSseLog((prev) => [...prev, evt]);
        // Update review status on completion
        if (evt.event === 'review.completed' || evt.event === 'review.failed') {
          setSseActive(false);
          // Refresh review data
          if (id) {
            fetchReview(Number(id)).then(setReview).catch(() => {});
          }
        }
      },
      () => setSseActive(false),
    );
  }, [review, id]);

  // Auto-subscribe SSE if review is running
  useEffect(() => {
    if (review?.status === 'running') {
      const close = connectSSE();
      return close;
    }
  }, [review?.status, connectSSE]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="text-gray-400 animate-pulse">Loading review...</div>
      </div>
    );
  }

  if (error || !review) {
    return (
      <div className="text-center py-20">
        <div className="text-4xl mb-4">❌</div>
        <p className="text-red-400">{error || 'Review not found'}</p>
        <Link to="/" className="text-emerald-400 hover:underline mt-4 inline-block">
          Back to list
        </Link>
      </div>
    );
  }

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-400 mb-6">
        <Link to="/" className="hover:text-white transition-colors">
          Reviews
        </Link>
        <span>/</span>
        <span className="text-gray-300">#{review.pr_number}</span>
      </div>

      {/* Header */}
      <div className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
        <div className="flex items-start justify-between gap-4 mb-4">
          <div>
            <h1 className="text-2xl font-bold mb-1">
              PR #{review.pr_number}
              {review.repo_url && (
                <span className="text-lg font-normal text-gray-400 ml-2">{review.repo_url}</span>
              )}
            </h1>
            <p className="text-sm text-gray-500">
              {formatTime(review.created_at)}
              {review.head_sha && (
                <code className="ml-3 px-2 py-0.5 bg-gray-800 rounded text-xs">
                  {review.head_sha.slice(0, 12)}
                </code>
              )}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <span
              className={`inline-flex items-center gap-2 px-3 py-1 rounded-full text-sm font-medium border ${
                review.status === 'running' ? 'bg-blue-900/30 text-blue-300 border-blue-700' :
                review.status === 'success' ? 'bg-emerald-900/30 text-emerald-300 border-emerald-700' :
                review.status === 'failed' ? 'bg-red-900/30 text-red-300 border-red-700' :
                'bg-gray-800 text-gray-400 border-gray-700'
              }`}
            >
              <span
                className={`w-2 h-2 rounded-full ${
                  review.status === 'running' ? 'bg-blue-400 animate-pulse' :
                  review.status === 'success' ? 'bg-emerald-400' :
                  review.status === 'failed' ? 'bg-red-400' : 'bg-gray-500'
                }`}
              />
              {review.status}
            </span>
            {review.status === 'running' && (
              <button
                onClick={connectSSE}
                disabled={sseActive}
                className="px-3 py-1 text-xs bg-emerald-800 text-emerald-300 rounded-lg hover:bg-emerald-700 disabled:opacity-50 transition-colors"
              >
                {sseActive ? 'Streaming...' : 'Watch Live'}
              </button>
            )}
          </div>
        </div>

        {/* Stats grid */}
        <div className="grid grid-cols-3 gap-4">
          <StatCard label="Issues Found" value={review.issues || '—'} color="text-amber-400" />
          <StatCard label="Duration" value={review.duration || '—'} color="text-blue-400" />
          <StatCard label="Status" value={review.status} color={review.status === 'success' ? 'text-emerald-400' : review.status === 'failed' ? 'text-red-400' : 'text-gray-400'} />
        </div>
      </div>

      {/* Error */}
      {review.error && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-4 mb-6">
          <p className="text-red-300 font-mono text-sm">{review.error}</p>
        </div>
      )}

      {/* Summary */}
      {review.summary && (
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
          <h2 className="text-lg font-semibold mb-3">Summary</h2>
          <p className="text-gray-300 whitespace-pre-wrap">{review.summary}</p>
        </div>
      )}

      {/* SSE Live Feed */}
      {(sseLog.length > 0 || review.status === 'running') && (
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">Live Agent Activity</h2>
            {review.status === 'running' && sseActive && (
              <span className="flex items-center gap-2 text-xs text-emerald-400">
                <span className="w-2 h-2 bg-emerald-400 rounded-full animate-pulse" />
                Connected
              </span>
            )}
          </div>
          {sseLog.length === 0 ? (
            <p className="text-sm text-gray-500">
              {sseActive
                ? 'Waiting for agent events...'
                : 'Click "Watch Live" to connect to the agent stream.'}
            </p>
          ) : (
            <div className="space-y-2 max-h-64 overflow-y-auto font-mono text-sm">
              {sseLog.map((evt, i) => (
                <div key={i} className="flex items-start gap-3 text-xs">
                  <span
                    className={`shrink-0 px-1.5 py-0.5 rounded ${
                      evt.event === 'review.completed'
                        ? 'bg-emerald-900/50 text-emerald-400'
                        : evt.event === 'review.failed'
                          ? 'bg-red-900/50 text-red-400'
                          : 'bg-blue-900/50 text-blue-400'
                    }`}
                  >
                    {evt.event}
                  </span>
                  <code className="text-gray-400 break-all">{evt.data}</code>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function StatCard({ label, value, color }: { label: string; value: string | number; color: string }) {
  return (
    <div className="bg-gray-800/50 rounded-lg p-4">
      <div className="text-xs text-gray-500 mb-1">{label}</div>
      <div className={`text-xl font-bold ${color}`}>{value}</div>
    </div>
  );
}

function formatTime(iso: string): string {
  try {
    return new Date(iso + 'Z').toLocaleString();
  } catch {
    return iso;
  }
}
