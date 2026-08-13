import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import {
  Plus,
  GitPullRequest,
  Clock,
  AlertTriangle,
  CheckCircle2,
  Loader2,
  X,
  Search,
} from 'lucide-react';
import { fetchReviews, triggerReview, ReviewRecord } from '../api';

export default function ReviewList() {
  const [reviews, setReviews] = useState<ReviewRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
      setTriggerMsg(`审查 #${result.review_id} 已启动`);
      setOwner(''); setRepo(''); setPrNumber('');
      setShowForm(false);
      const data = await fetchReviews();
      setReviews(data);
    } catch (err) {
      setTriggerMsg(err instanceof Error ? err.message : '触发失败');
    } finally {
      setTriggering(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-24">
        <Loader2 className="w-6 h-6 text-brand animate-spin" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-xl p-6 text-red-700">
        <p className="font-semibold">加载失败</p>
        <p className="text-sm mt-1">{error}</p>
        <p className="text-xs mt-2 text-red-400">请确认后端运行在 http://localhost:8080</p>
      </div>
    );
  }

  const successCount = reviews.filter((r) => r.status === 'success').length;
  const failCount = reviews.filter((r) => r.status === 'failed').length;
  const totalIssues = reviews.reduce((sum, r) => sum + (r.issues || 0), 0);

  return (
    <div>
      {/* 页头 + 统计 */}
      <div className="flex items-end justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">代码审查</h1>
          <p className="text-sm text-gray-500 mt-1">基于 agent-go 认知面的自动化 PR 审查</p>
        </div>
        <button
          onClick={() => setShowForm(true)}
          className="inline-flex items-center gap-2 px-4 py-2 bg-brand hover:bg-brand-hover text-white rounded-lg text-sm font-medium transition-colors"
        >
          <Plus className="w-4 h-4" />
          手动触发
        </button>
      </div>

      {/* 统计卡片 */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <StatCard label="总审查" value={reviews.length} icon={<GitPullRequest className="w-4 h-4" />} tone="brand" />
        <StatCard label="发现问题" value={totalIssues} icon={<AlertTriangle className="w-4 h-4" />} tone="amber" />
        <StatCard label="成功 / 失败" value={`${successCount} / ${failCount}`} icon={<CheckCircle2 className="w-4 h-4" />} tone="green" />
      </div>

      {/* 触发表单 */}
      {showForm && (
        <form onSubmit={handleTrigger} className="mb-6 bg-white border border-gray-200 rounded-xl p-5 space-y-4 shadow-sm">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-gray-900">手动代码审查</h3>
            <button type="button" onClick={() => setShowForm(false)} className="text-gray-400 hover:text-gray-600">
              <X className="w-4 h-4" />
            </button>
          </div>
          <div className="flex gap-3 items-end">
            <div className="flex-1">
              <label className="text-xs text-gray-500 block mb-1.5">GitHub 仓库</label>
              <div className="flex gap-1.5 items-center">
                <input
                  type="text"
                  placeholder="owner"
                  value={owner}
                  onChange={(e) => setOwner(e.target.value)}
                  className="flex-1 bg-gray-50 border border-gray-200 rounded-lg px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-brand/30 focus:border-brand"
                  required
                />
                <span className="text-gray-400">/</span>
                <input
                  type="text"
                  placeholder="repo"
                  value={repo}
                  onChange={(e) => setRepo(e.target.value)}
                  className="flex-1 bg-gray-50 border border-gray-200 rounded-lg px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-brand/30 focus:border-brand"
                  required
                />
              </div>
            </div>
            <div>
              <label className="text-xs text-gray-500 block mb-1.5">PR 编号</label>
              <input
                type="number"
                placeholder="42"
                value={prNumber}
                onChange={(e) => setPrNumber(e.target.value)}
                className="w-28 bg-gray-50 border border-gray-200 rounded-lg px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-brand/30 focus:border-brand"
                required
              />
            </div>
            <button
              type="submit"
              disabled={triggering}
              className="inline-flex items-center gap-2 px-5 py-2 bg-brand hover:bg-brand-hover disabled:bg-gray-300 text-white rounded-lg text-sm font-medium transition-colors"
            >
              {triggering ? <Loader2 className="w-4 h-4 animate-spin" /> : <Search className="w-4 h-4" />}
              {triggering ? '启动中...' : '开始审查'}
            </button>
          </div>
          {triggerMsg && (
            <div className={`text-sm p-3 rounded-lg ${triggerMsg.startsWith('审查 #') ? 'bg-emerald-50 text-emerald-700 border border-emerald-200' : 'bg-red-50 text-red-700 border border-red-200'}`}>
              {triggerMsg}
            </div>
          )}
        </form>
      )}

      {/* 列表 */}
      {reviews.length === 0 ? (
        <EmptyState onTrigger={() => setShowForm(true)} />
      ) : (
        <div className="space-y-3">
          {reviews.map((review) => (
            <ReviewRow key={review.id} review={review} />
          ))}
        </div>
      )}
    </div>
  );
}

function ReviewRow({ review }: { review: ReviewRecord }) {
  return (
    <Link
      to={`/reviews/${review.id}`}
      className="block bg-white border border-gray-200 rounded-xl p-5 hover:border-brand/40 hover:shadow-sm transition-all"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-3 mb-1.5">
            <span className="font-mono text-sm font-medium text-gray-900">#{review.pr_number}</span>
            {review.repo_url && (
              <span className="text-sm text-gray-500 truncate">{review.repo_url}</span>
            )}
          </div>
          {review.summary && (
            <p className="text-sm text-gray-500 mt-0.5 line-clamp-1">{review.summary}</p>
          )}
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <StatusBadge status={review.status} />
          {review.issues > 0 && (
            <span className="inline-flex items-center gap-1 text-sm text-amber-600 font-mono">
              <AlertTriangle className="w-3.5 h-3.5" />
              {review.issues}
            </span>
          )}
        </div>
      </div>
      <div className="flex items-center gap-4 mt-3 text-xs text-gray-400">
        <span className="inline-flex items-center gap-1.5">
          <Clock className="w-3.5 h-3.5" />
          {formatTime(review.created_at)}
        </span>
        {review.duration && <span>{review.duration}</span>}
        {review.head_sha && (
          <span className="font-mono bg-gray-100 px-1.5 py-0.5 rounded">{review.head_sha.slice(0, 7)}</span>
        )}
      </div>
    </Link>
  );
}

function StatCard({ label, value, icon, tone }: { label: string; value: string | number; icon: React.ReactNode; tone: 'brand' | 'amber' | 'green' }) {
  const toneCls = {
    brand: 'text-brand bg-brand-soft',
    amber: 'text-amber-600 bg-amber-50',
    green: 'text-emerald-600 bg-emerald-50',
  }[tone];
  return (
    <div className="bg-white border border-gray-200 rounded-xl p-5">
      <div className="flex items-center gap-3">
        <span className={`w-9 h-9 rounded-lg flex items-center justify-center ${toneCls}`}>
          {icon}
        </span>
        <div>
          <div className="text-xs text-gray-500">{label}</div>
          <div className="text-xl font-bold text-gray-900">{value}</div>
        </div>
      </div>
    </div>
  );
}

function EmptyState({ onTrigger }: { onTrigger: () => void }) {
  return (
    <div className="text-center py-20 bg-white border border-dashed border-gray-300 rounded-xl">
      <span className="inline-flex w-14 h-14 rounded-xl bg-brand-soft items-center justify-center mb-4">
        <GitPullRequest className="w-7 h-7 text-brand" />
      </span>
      <h2 className="text-lg font-semibold text-gray-900 mb-1">暂无审查记录</h2>
      <p className="text-sm text-gray-500 max-w-sm mx-auto mb-6">
        手动输入一个 GitHub PR 触发审查，或配置 webhook 实现自动审查。
      </p>
      <button
        onClick={onTrigger}
        className="inline-flex items-center gap-2 px-5 py-2.5 bg-brand hover:bg-brand-hover text-white rounded-lg text-sm font-medium transition-colors"
      >
        <Plus className="w-4 h-4" />
        触发首次审查
      </button>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { cls: string; icon: React.ReactNode }> = {
    running: { cls: 'bg-blue-50 text-blue-700 border-blue-200', icon: <Loader2 className="w-3 h-3 animate-spin" /> },
    success: { cls: 'bg-emerald-50 text-emerald-700 border-emerald-200', icon: <CheckCircle2 className="w-3 h-3" /> },
    failed: { cls: 'bg-red-50 text-red-700 border-red-200', icon: <X className="w-3 h-3" /> },
    pending: { cls: 'bg-gray-100 text-gray-600 border-gray-200', icon: <Clock className="w-3 h-3" /> },
  };
  const c = cfg[status] || cfg.pending;
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border ${c.cls}`}>
      {c.icon}
      {status}
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
