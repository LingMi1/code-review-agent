import { Routes, Route, Link, useLocation } from 'react-router-dom';
import ReviewList from './pages/ReviewList';
import ReviewDetail from './pages/ReviewDetail';

export default function App() {
  const location = useLocation();

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      {/* Navbar */}
      <nav className="border-b border-gray-800 bg-gray-900/80 backdrop-blur-sm sticky top-0 z-50">
        <div className="max-w-6xl mx-auto px-6 py-3 flex items-center justify-between">
          <Link to="/" className="flex items-center gap-3 text-lg font-semibold hover:text-emerald-400 transition-colors">
            <span className="text-2xl">🔍</span>
            <span>Code Review Agent</span>
          </Link>
          <div className="flex items-center gap-4 text-sm text-gray-400">
            <Link
              to="/"
              className={`hover:text-white transition-colors ${location.pathname === '/' ? 'text-emerald-400' : ''}`}
            >
              Reviews
            </Link>
            <span className="text-gray-600">|</span>
            <a
              href="https://github.com/LingMi1/agent-go"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-white transition-colors"
            >
              agent-go ↗
            </a>
            <a
              href="https://github.com/LingMi1/code-review-agent"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-white transition-colors"
            >
              GitHub ↗
            </a>
          </div>
        </div>
      </nav>

      {/* Main */}
      <main className="max-w-6xl mx-auto px-6 py-8">
        <Routes>
          <Route path="/" element={<ReviewList />} />
          <Route path="/reviews/:id" element={<ReviewDetail />} />
        </Routes>
      </main>

      {/* Footer */}
      <footer className="border-t border-gray-800 py-6 text-center text-sm text-gray-500">
        Powered by{' '}
        <a
          href="https://github.com/LingMi1/agent-go"
          target="_blank"
          rel="noopener noreferrer"
          className="text-emerald-400 hover:underline"
        >
          agent-go
        </a>
        {' '}— Go webhook + gRPC cognition + React dashboard
      </footer>
    </div>
  );
}
