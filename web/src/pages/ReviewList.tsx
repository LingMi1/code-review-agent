import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { fetchReviews, triggerReview, ReviewRecord } from '../api';

export default function ReviewList() {
  const [reviews, setReviews] = useState<ReviewRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Trigger form state
  const [showForm, setShowForm] = useState(false);
  const [owner, setOwner] = useState('');
  const [repo, setRepo] = useState('');
  const [prNumber, setPrNumber] = useState('');
  const [triggering, setTriggering] = useState(false);
  const [triggerMsg, setTriggerMsg] = useState<string | null>(null);

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
    const interval = setInterval(load, 5000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  const handleTrigger = async (e: React.FormEvent) => {
    e.preventDefault();
    setTriggering(true);
    setTriggerMsg(null);
    try {
      const result = await triggerReview({
        owner: owner.trim(),
        repo: repo.trim(),
        pr_number: parseInt(prNumber, 10),
      });
      setTriggerMsg(`Review #${result.review_id} started! Watch it appear below.`);
      setOwner(''); setRepo(''); setPrNumber('');
      setShowForm(false);
      // Immediately reload
      const data = await fetchReviews();
      setReviews(data);
    } catch (err) {
      setTriggerMsg(err instanceof Error ? err.message : 'Trigger failed');
    } finally {
      setTriggering(false);
    }
  };

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
        <p className="text-gray-500 max-w-md mx-auto mb-8">
          Trigger a manual review by entering a GitHub PR URL, or configure a webhook for automatic reviews.
        </p>

        {/* Trigger button + form */}
        {!showForm ? (
          <button
            onClick={() => setShowForm(true)}
            className="px-6 py-3 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg font-medium transition-colors"
          >
            Trigger Manual Review
          </button>
        ) : (
          <form onSubmit={handleTrigger} className="max-w-md mx-auto bg-gray-900 border border-gray-800 rounded-lg p-6 space-y-4">
            <h3 className="text-lg font-semibold text-gray-200">Trigger Code Review</h3>
            <p className="text-xs text-gray-500">Enter a public GitHub PR to review (e.g. <code className="text-gray-400">LingMi1/agent-go/pull/42</code>)</p>
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="Owner"
                value={owner}
                onChange={(e) => setOwner(e.target.value)}
                className="flex-1 bg-gray-800 border border-gray-700 rounded px-3 py-2 text-gray-200 text-sm placeholder-gray-500 focus:outline-none focus:border-emerald-500"
                required
              />
              <span className="text-gray-500 self-center">/</span>
              <input
                type="text"
                placeholder="Repo"
                value={repo}
                onChange={(e) => setRepo(e.target.value)}
                className="flex-1 bg-gray-800 border border-gray-700 rounded px-3 py-2 text-gray-200 text-sm placeholder-gray-500 focus:outline-none focus:border-emerald-500"
                required
              />
            </div>
            <input
              type="number"
              placeholder="PR Number (e.g. 42)"
              value={prNumber}
              onChange={(e) => setPrNumber(e.target.value)}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-gray-200 text-sm placeholder-gray-500 focus:outline-none focus:border-emerald-500"
              required
            />
            <div className="flex gap-2">
              <button
                type="submit"
                disabled={triggering}
                className="flex-1 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:bg-gray-700 text-white rounded font-medium text-sm transition-colors"
              >
                {triggering ? 'Starting...' : 'Start Review'}
              </button>
              <button
                type="button"
                onClick={() => setShowForm(false)}
                className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-400 rounded text-sm transition-colors"
              >
                Cancel
              </button>
            </div>
            {triggerMsg && (
              <div className={`text-sm p-3 rounded ${triggerMsg.startsWith('Review #') ? 'bg-emerald-900/30 text-emerald-300 border border-emerald-800' : 'bg-red-900/30 text-red-300 border border-red-800'}`}>
                {triggerMsg}
              </div>
            )}
          </form>
        )}
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Review History</h1>
        <div className="flex items-center gap-3">
          <span className="text-sm text-gray-400">{reviews.length} total</span>
          {!showForm ? (
            <button
              onClick={() => setShowForm(true)}
              className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-sm font-medium transition-colors"
            >
              + New Review
            </button>
          ) : (
            <button
              onClick={() => { setShowForm(false); setTriggerMsg(null); }}
              className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-400 rounded-lg text-sm transition-colors"
            >
              Cancel
            </button>
          )}
        </div>
      </div>

      {/* Trigger form (collapsible) */}
      {showForm && (
        <form onSubmit={handleTrigger} className="mb-6 bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-4">
          <h3 className="text-sm font-semibold text-gray-300">Manual Code Review</h3>
          <div className="flex gap-3 items-end">
            <div className="flex-1">
              <label className="text-xs text-gray-500 block mb-1">GitHub URL or Owner/Repo</label>
              <div className="flex gap-1.5">
                <input
                  type="text"
                  placeholder="owner"
                  value={owner}
                  onChange={(e) => setOwner(e.target.value)}
                  className="flex-1 bg-gray-800 border border-gray-700 rounded px-3 py-2 text-gray-200 text-sm placeholder-gray-500 focus:outline-none focus:border-emerald-500"
                  required
                />
                <span className="text-gray-500 self-center">/</span>
                <input
                  type="text"
                  placeholder="repo"
                  value={repo}
                  onChange={(e) => setRepo(e.target.value)}
                  className="flex-1 bg-gray-800 border border-gray-700 rounded px-3 py-2 text-gray-200 text-sm placeholder-gray-500 focus:outline-none focus:border-emerald-500"
                  required
                />
              </div>
            </div>
            <div>
              <label className="text-xs text-gray-500 block mb-1">PR #</label>
              <input
                type="number"
                placeholder="42"
                value={prNumber}
                onChange={(e) => setPrNumber(e.target.value)}
                className="w-24 bg-gray-800 border border-gray-700 rounded px-3 py-2 text-gray-200 text-sm placeholder-gray-500 focus:outline-none focus:border-emerald-500"
                required
              />
            </div>
            <button
              type="submit"
              disabled={triggering}
              className="px-5 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:bg-gray-700 text-white rounded font-medium text-sm transition-colors"
            >
              {triggering ? 'Starting...' : 'Review'}
            </button>
          </div>
          {triggerMsg && (
            <div className={`text-sm p-3 rounded ${triggerMsg.startsWith('Review #') ? 'bg-emerald-900/30 text-emerald-300 border border-emerald-800' : 'bg-red-900/30 text-red-300 border border-red-800'}`}>
              {triggerMsg}
            </div>
          )}
        </form>
      )}

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
