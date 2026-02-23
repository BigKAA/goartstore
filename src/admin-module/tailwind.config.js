/** @type {import('tailwindcss').Config} */
module.exports = {
  // Сканируем .templ файлы для утилитарных классов
  content: [
    "./internal/ui/**/*.templ",
  ],
  theme: {
    extend: {
      // Цвета тёмной зелёной темы (из спецификации admin-ui-requirements.md)
      colors: {
        // Фоновые слои
        'bg-base': '#0a0f0a',
        'bg-surface': '#111c11',
        'bg-elevated': '#1a2e1a',
        'bg-hover': '#243824',

        // Текст
        'text-primary': '#e8f5e8',
        'text-secondary': '#9cb89c',
        'text-muted': '#5a7a5a',

        // Акцентные цвета (бренд/действия)
        'accent-primary': '#22c55e',
        'accent-light': '#4ade80',
        'accent-bright': '#86efac',
        'accent-lime': '#a3e635',

        // Семантические цвета
        'status-success': '#22c55e',
        'status-warning': '#eab308',
        'status-error': '#ef4444',
        'status-info': '#3b82f6',

        // Цвета режимов Storage Element
        'mode-edit': '#22c55e',
        'mode-rw': '#3b82f6',
        'mode-ro': '#eab308',
        'mode-ar': '#6b7280',

        // Границы и разделители
        'border-subtle': '#1a2e1a',
        'border-default': '#243824',
        'border-accent': '#22c55e',
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
