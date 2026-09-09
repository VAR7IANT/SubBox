import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        cream: 'var(--sb-cream)',
        surface: 'var(--sb-surface)',
        apricot: 'var(--sb-apricot)',
        'apricot-soft': 'var(--sb-apricot-soft)',
        'apricot-strong': 'var(--sb-apricot-strong)',
        ink: 'var(--sb-ink)',
        'warm-muted': 'var(--sb-warm-muted)',
        line: 'var(--sb-line)',
        success: 'var(--sb-success)',
        'success-soft': 'var(--sb-success-soft)',
      },
      boxShadow: {
        card: '0 18px 46px -32px rgba(166, 92, 42, 0.42)',
      },
    },
  },
  plugins: [],
} satisfies Config
