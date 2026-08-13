import { Routes, Route, Link, useLocation } from 'react-router-dom';
import {
  GitPullRequest,
  ExternalLink,
  Code2,
  Settings,
} from 'lucide-react';
import ReviewList from './pages/ReviewList';
import ReviewDetail from './pages/ReviewDetail';

export default function App() {
  const location = useLocation();
  const isList = location.pathname === '/';

  return (
    <div className="min-h-screen bg-surface flex">
      {/* 侧边栏 */}
      <aside className="w-60 shrink-0 bg-sidebar text-gray-300 flex flex-col fixed inset-y-0">
        {/* Logo */}
        <div className="px-5 py-5 border-b border-white/5">
          <Link to="/" className="flex items-center gap-3 group">
            <span className="w-9 h-9 rounded-lg bg-gradient-to-br from-brand-from to-brand-to flex items-center justify-center shadow-lg shadow-brand/30">
              <Code2 className="w-5 h-5 text-white" strokeWidth={2.25} />
            </span>
            <span className="leading-tight">
              <span className="block text-sm font-semibold text-white">Code Review</span>
              <span className="block text-xs text-gray-400">Agent</span>
            </span>
          </Link>
        </div>

        {/* 导航 */}
        <nav className="flex-1 px-3 py-4 space-y-1">
          <NavItem to="/" icon={<GitPullRequest className="w-4 h-4" />} label="Reviews" active={isList} />
          <div className="pt-4 mt-4 border-t border-white/5 space-y-1">
            <a
              href="https://github.com/LingMi1/code-review-agent"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 hover:text-white hover:bg-white/5 transition-colors"
            >
              <ExternalLink className="w-4 h-4" />
              GitHub Repo
            </a>
            <a
              href="https://github.com/LingMi1/agent-go"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 hover:text-white hover:bg-white/5 transition-colors"
            >
              <Code2 className="w-4 h-4" />
              agent-go
            </a>
          </div>
        </nav>

        {/* 底部设置占位 */}
        <div className="px-3 py-4 border-t border-white/5">
          <div className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 cursor-not-allowed">
            <Settings className="w-4 h-4" />
            Settings
          </div>
        </div>
      </aside>

      {/* 主内容区 */}
      <div className="flex-1 ml-60 flex flex-col min-h-screen">
        {/* 顶栏 */}
        <header className="sticky top-0 z-40 bg-surface/80 backdrop-blur-sm border-b border-gray-200">
          <div className="max-w-5xl mx-auto px-8 h-14 flex items-center justify-between">
            <div className="text-sm text-gray-500">
              {isList ? '代码审查面板' : '审查详情'}
            </div>
            <div className="flex items-center gap-3 text-sm text-gray-500">
              <span className="hidden sm:inline-flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-emerald-500" />
                Agent 在线
              </span>
            </div>
          </div>
        </header>

        {/* 内容 */}
        <main className="flex-1 max-w-5xl mx-auto w-full px-8 py-8">
          <Routes>
            <Route path="/" element={<ReviewList />} />
            <Route path="/reviews/:id" element={<ReviewDetail />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}

function NavItem({ to, icon, label, active }: { to: string; icon: React.ReactNode; label: string; active: boolean }) {
  return (
    <Link
      to={to}
      className={`flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors ${
        active
          ? 'bg-brand text-white font-medium'
          : 'text-gray-400 hover:text-white hover:bg-white/5'
      }`}
    >
      {icon}
      {label}
    </Link>
  );
}
