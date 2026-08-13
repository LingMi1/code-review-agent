import { Routes, Route, Link, useLocation } from 'react-router-dom';
import {
  GitPullRequest,
  ExternalLink,
  Bot,
  Settings,
} from 'lucide-react';
import ReviewList from './pages/ReviewList';
import ReviewDetail from './pages/ReviewDetail';

export default function App() {
  const location = useLocation();
  const isList = location.pathname === '/';

  return (
    <div className="min-h-screen bg-surface flex">
      {/* Sidebar */}
      <aside className="w-60 shrink-0 bg-sidebar text-gray-300 flex flex-col fixed inset-y-0">
        {/* Logo */}
        <div className="px-5 py-5 border-b border-white/5">
          <Link to="/" className="flex items-center gap-3">
            <Logo />
            <span className="leading-tight">
              <span className="block text-sm font-semibold text-white">Code Review</span>
              <span className="block text-xs text-gray-400">Agent</span>
            </span>
          </Link>
        </div>

        {/* Navigation */}
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
              <Bot className="w-4 h-4" />
              agent-go
            </a>
          </div>
        </nav>

        {/* Bottom */}
        <div className="px-3 py-4 border-t border-white/5">
          <div className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 cursor-not-allowed">
            <Settings className="w-4 h-4" />
            Settings
          </div>
        </div>
      </aside>

      {/* Main content */}
      <div className="flex-1 ml-60 flex flex-col min-h-screen">
        {/* Top bar */}
        <header className="sticky top-0 z-40 bg-surface/80 backdrop-blur-sm border-b border-gray-200">
          <div className="max-w-5xl mx-auto px-8 h-14 flex items-center justify-between">
            <div className="text-sm text-gray-500">
              {isList ? 'Code Review Dashboard' : 'Review Detail'}
            </div>
            <div className="flex items-center gap-3 text-sm text-gray-500">
              <span className="hidden sm:inline-flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-emerald-500" />
                Agent Online
              </span>
            </div>
          </div>
        </header>

        {/* Content */}
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

function Logo() {
  return (
    <svg width="36" height="36" viewBox="0 0 36 36" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <defs>
        <linearGradient id="logo-grad" x1="0" y1="0" x2="36" y2="36" gradientUnits="userSpaceOnUse">
          <stop stopColor="#7C3AED" />
          <stop offset="1" stopColor="#4F46E5" />
        </linearGradient>
      </defs>
      <rect width="36" height="36" rx="9" fill="url(#logo-grad)" />
      {/* angle brackets </> */}
      <path d="M13 12.5L9 18L13 23.5" stroke="white" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M23 12.5L27 18L23 23.5" stroke="white" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M20 14L16 22" stroke="white" strokeWidth="2.5" strokeLinecap="round" />
      {/* spark */}
      <path d="M28 5.5L29.2 6.7L30.5 8L29.2 9.3L28 10.5L26.8 9.3L25.5 8L26.8 6.7Z" fill="white" fillOpacity="0.9" />
    </svg>
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
