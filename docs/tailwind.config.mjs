/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}'],
  theme: {
    extend: {
      colors: {
        // FrankenPHP brand colors
        primary: {
          DEFAULT: '#390075',
          50: '#f3e8ff',
          100: '#e9d5ff',
          200: '#d4b4fe',
          300: '#C3B2D3',
          400: '#a855f7',
          500: '#9333ea',
          600: '#7e22ce',
          700: '#390075',
          800: '#230143',
          900: '#1a0033',
          950: '#0f001f',
        },
        lime: {
          DEFAULT: '#B3D133',
          light: '#CBDB8B',
          dark: '#9bc020',
          50: '#f7fee7',
          100: '#ecfccb',
          200: '#d9f99d',
          300: '#CBDB8B',
          400: '#B3D133',
          500: '#84cc16',
          600: '#65a30d',
        },
        lavender: {
          DEFAULT: '#6b7280',
          light: '#9ca3af',
          dark: '#4b5563',
        },
        // Light theme surface colors
        surface: {
          DEFAULT: '#ffffff',
          50: '#f9fafb',
          100: '#f3f4f6',
          200: '#e5e7eb',
          300: '#d1d5db',
          400: '#9ca3af',
          500: '#6b7280',
        },
        // Glass colors for light theme
        glass: {
          bg: 'rgba(255, 255, 255, 0.8)',
          border: 'rgba(0, 0, 0, 0.08)',
          hover: 'rgba(255, 255, 255, 0.9)',
        },
      },
      fontFamily: {
        sans: ['Inter Variable', 'Inter', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      boxShadow: {
        glow: '0 0 20px rgba(179, 209, 51, 0.3)',
        'glow-lg': '0 0 40px rgba(179, 209, 51, 0.4)',
        'glow-purple': '0 0 30px rgba(57, 0, 117, 0.2)',
        glass: '0 4px 30px rgba(0, 0, 0, 0.1)',
        'soft': '0 2px 15px rgba(0, 0, 0, 0.08)',
        'card': '0 4px 20px rgba(0, 0, 0, 0.06)',
      },
      animation: {
        'gradient-flow': 'gradient-flow 4s ease infinite',
        'fade-in': 'fade-in 0.3s ease forwards',
        'slide-up': 'slide-up 0.8s ease-out',
      },
      typography: {
        DEFAULT: {
          css: {
            '--tw-prose-body': '#374151',
            '--tw-prose-headings': '#111827',
            '--tw-prose-lead': '#4b5563',
            '--tw-prose-links': '#390075',
            '--tw-prose-bold': '#111827',
            '--tw-prose-counters': '#6b7280',
            '--tw-prose-bullets': '#9ca3af',
            '--tw-prose-hr': '#e5e7eb',
            '--tw-prose-quotes': '#374151',
            '--tw-prose-quote-borders': '#390075',
            '--tw-prose-captions': '#6b7280',
            '--tw-prose-code': '#390075',
            '--tw-prose-pre-code': '#e5e7eb',
            '--tw-prose-pre-bg': '#1f2937',
            '--tw-prose-th-borders': '#d1d5db',
            '--tw-prose-td-borders': '#e5e7eb',
            maxWidth: 'none',
            code: {
              backgroundColor: 'rgba(57, 0, 117, 0.1)',
              padding: '0.2em 0.4em',
              borderRadius: '0.25rem',
              fontWeight: '500',
              color: '#390075',
            },
            'code::before': { content: '""' },
            'code::after': { content: '""' },
            a: {
              color: '#390075',
              textDecoration: 'none',
              fontWeight: '500',
              '&:hover': {
                textDecoration: 'underline',
              },
            },
            pre: {
              backgroundColor: '#1f2937',
              borderRadius: '0.75rem',
            },
          },
        },
      },
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
};
