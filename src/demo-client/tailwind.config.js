/** @type {import('tailwindcss').Config} */
module.exports = {
  // Сканируем .templ файлы для утилитарных классов
  content: [
    "./internal/ui/**/*.templ",
  ],
  theme: {
    extend: {
      // Цвета тёмной синей темы (Demo Client — синий акцент, отличие от зелёного AM)
      colors: {
        // Фоновые слои
        'bg-base': '#0a0d12',
        'bg-surface': '#111827',
        'bg-elevated': '#1e293b',
        'bg-hover': '#334155',

        // Текст
        'text-primary': '#e2e8f0',
        'text-secondary': '#94a3b8',
        'text-muted': '#64748b',

        // Акцентные цвета (синий — основной акцент Demo Client)
        'accent-primary': '#3b82f6',
        'accent-light': '#60a5fa',
        'accent-bright': '#93c5fd',

        // Семантические цвета
        'status-success': '#22c55e',
        'status-warning': '#eab308',
        'status-error': '#ef4444',
        'status-info': '#3b82f6',

        // Цвета режимов Storage Element (retention policy)
        'mode-edit': '#22c55e',
        'mode-rw': '#3b82f6',
        'mode-ro': '#eab308',
        'mode-ar': '#6b7280',

        // Retention policy цвета
        'retention-temporary': '#f59e0b',
        'retention-permanent': '#3b82f6',

        // Границы и разделители
        'border-subtle': '#1e293b',
        'border-default': '#334155',
        'border-accent': '#3b82f6',
      },

      // Шрифты
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },

      // Скругления
      borderRadius: {
        'card': '0.5rem',
        'button': '0.375rem',
      },
    },
  },
  plugins: [],
}
