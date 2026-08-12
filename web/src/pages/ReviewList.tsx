import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { fetchReviews, ReviewRecord } from '../api';

export default function ReviewList() {
  const [reviews, setReviews] = useState<ReviewRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const data = await fetchReviews();
        if (!cancelled) {
          setReviews(data);
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
    const interval = setInterval(load, 5000); // 每 5 秒轮询
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="text-gray-400 animate-pulse">Loading reviews...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-900/30 border border-red-700 rounded-lg p-6 text-red-300">
        <p className="font-semibold">Failed to load reviews</p>
        <p className="text-sm mt-1">{error}</p>
        <p className="text-xs mt-2 text-red-400">
          Make sure the backend is running on http://localhost:8080
        </p>
      </div>
    );
  }

  if (reviews.length === 0) {
    return (
      <div className="text-center py-20">
        <div className="text-6xl mb-4">📋</div>
        <h2 className="text-xl font-semibold text-gray-300 mb-2">No Reviews Yet</h2>
        <p className="text-gray-500 max-w-md mx-auto">
          Push a pull request to a connected repository and the AI reviewer will analyze it automatically.
        </p>
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Review History</h1>
        <span className="text-sm text-gray-400">{reviews.length} total</span>
      </div>

      <div className="space-y-3">
        {reviews.map((review) => (
          <Link
            key={review.id}
            to={`/reviews/${review.id}`}
            className="block bg-gray-900 border border-gray-800 rounded-lg p-5 hover:border-emerald-600/50 hover:bg-gray-800/50 transition-all"
          >
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-3 mb-1">
                  <span className="font-mono text-sm text-gray-300">#{review.pr_number}</span>
                  {review.repo_url && (
                    <span className="text-sm text-gray-500 truncate">{review.repo_url}</span>
                  )}
                </div>
                {review.summary && (
                  <p className="text-sm text-gray-400 mt-1 line-clamp-1">{review.summary}</p>
                )}
              </div>
              <div className="flex items-center gap-3 shrink-0">
                <StatusBadge status={review.status} />
                {review.issues > 0 && (
                  <span className="text-sm text-amber-400 font-mono">{review.issues} issues</span>
                )}
              </div>
            </div>
            <div className="flex items-center gap-4 mt-3 text-xs text-gray-500">
              <span>{formatTime(review.created_at)}</span>
              {review.duration && <span>{review.duration}</span>}
              {review.head_sha && (
                <span className="font-mono">{review.head_sha.slice(0, 7)}</span>
              )}
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    running: 'bg-blue-900/50 text-blue-300 border-blue-700',
    success: 'bg-emerald-900/50 text-emerald-300 border-emerald-700',
    failed: 'bg-red-900/50 text-red-300 border-red-700',
    pending: 'bg-gray-800 text-gray-400 border-gray-700',
  };

  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium border ${
        colors[status] || colors.pending
      }`}
    >
      <span
        className={`w-1.5 h-1.5 rounded-full ${
          status === 'running' ? 'bg-blue-400 animate-pulse' :
          status === 'success' ? 'bg-emerald-400' :
          status === 'failed' ? 'bg-red-400' : 'bg-gray-500'
        }`}
      />
      {status}
    </span>
  );
}

function formatTime(iso: string): string {
  try {
    return new Date(iso + 'Z').toLocaleString();
  } catch {
    return iso;
  }
}
