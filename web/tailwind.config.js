export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        // 品牌紫罗兰（复用 Tailwind violet，但显式声明语义名）
        brand: {
          DEFAULT: '#7C3AED', // violet-600
          hover: '#6D28D9',   // violet-700
          soft: '#EDE9FE',    // violet-100
          from: '#7C3AED',
          to: '#4F46E5',      // indigo-600
          dark: '#4C1D95',    // violet-900
        },
        // 侧边栏深紫黑
        sidebar: '#1F1A2E',
        // GitHub diff 语义色（spec 要求不可更改）
        diffadd: '#2EA043',
        diffdel: '#F85149',
        // 主背景
        surface: '#FAFAFA',
      },
      fontFamily: {
        sans: [
          'Inter',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'sans-serif',
        ],
        mono: [
          'ui-monospace',
          'SF Mono',
          'JetBrains Mono',
          'Menlo',
          'Consolas',
          'monospace',
        ],
      },
    },
  },
  plugins: [],
};
