package render

// TailwindConfig provides default Tailwind CSS configuration for irgo apps.
const TailwindConfig = `/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./**/*.templ",
    "./**/*.go",
  ],
  theme: {
    extend: {
      // HTMX-specific animations
      animation: {
        'htmx-added': 'htmx-added 0.5s ease-out',
        'htmx-settling': 'htmx-settling 0.5s ease-out',
        'htmx-swapping': 'htmx-swapping 0.5s ease-out',
        'fade-in': 'fadeIn 0.3s ease-out',
        'fade-out': 'fadeOut 0.3s ease-out',
        'slide-in': 'slideIn 0.3s ease-out',
        'slide-out': 'slideOut 0.3s ease-out',
      },
      keyframes: {
        'htmx-added': {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        'htmx-settling': {
          '0%': { opacity: '0.5' },
          '100%': { opacity: '1' },
        },
        'htmx-swapping': {
          '0%': { opacity: '1' },
          '100%': { opacity: '0' },
        },
        'fadeIn': {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        'fadeOut': {
          '0%': { opacity: '1' },
          '100%': { opacity: '0' },
        },
        'slideIn': {
          '0%': { opacity: '0', transform: 'translateX(-10px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' },
        },
        'slideOut': {
          '0%': { opacity: '1', transform: 'translateX(0)' },
          '100%': { opacity: '0', transform: 'translateX(10px)' },
        },
      },
    },
  },
  plugins: [],
}
`

// TailwindCSS provides base CSS including HTMX integration styles.
const TailwindCSS = `@tailwind base;
@tailwind components;
@tailwind utilities;

/* HTMX indicator styles */
.htmx-indicator {
  opacity: 0;
  transition: opacity 200ms ease-in;
}

.htmx-request .htmx-indicator {
  opacity: 1;
}

.htmx-request.htmx-indicator {
  opacity: 1;
}

/* HTMX request state styling */
.htmx-request {
  cursor: wait;
}

.htmx-request button,
.htmx-request input[type="submit"] {
  pointer-events: none;
  opacity: 0.7;
}

/* HTMX added element animation */
.htmx-added {
  animation: htmx-added 0.5s ease-out;
}

/* HTMX settling animation */
.htmx-settling {
  animation: htmx-settling 0.5s ease-out;
}

/* HTMX swapping animation */
.htmx-swapping {
  animation: htmx-swapping 0.5s ease-out;
}

@keyframes htmx-added {
  0% { opacity: 0; transform: translateY(-10px); }
  100% { opacity: 1; transform: translateY(0); }
}

@keyframes htmx-settling {
  0% { opacity: 0.5; }
  100% { opacity: 1; }
}

@keyframes htmx-swapping {
  0% { opacity: 1; }
  100% { opacity: 0; }
}

/* Error styling */
.error {
  @apply bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded relative;
}

/* Success styling */
.success {
  @apply bg-green-50 border border-green-200 text-green-700 px-4 py-3 rounded relative;
}

/* Loading spinner */
.spinner {
  @apply animate-spin rounded-full h-5 w-5 border-2 border-gray-300 border-t-blue-600;
}

/* Mobile-first responsive utilities */
@layer utilities {
  .safe-top {
    padding-top: env(safe-area-inset-top);
  }
  .safe-bottom {
    padding-bottom: env(safe-area-inset-bottom);
  }
  .safe-left {
    padding-left: env(safe-area-inset-left);
  }
  .safe-right {
    padding-right: env(safe-area-inset-right);
  }
  .safe-area {
    padding-top: env(safe-area-inset-top);
    padding-bottom: env(safe-area-inset-bottom);
    padding-left: env(safe-area-inset-left);
    padding-right: env(safe-area-inset-right);
  }
}
`

// PackageJSON provides the default package.json for Tailwind setup.
const PackageJSON = `{
  "name": "irgo-app",
  "version": "1.0.0",
  "scripts": {
    "build:css": "tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --minify",
    "watch:css": "tailwindcss -i ./assets/css/input.css -o ./assets/css/output.css --watch"
  },
  "devDependencies": {
    "tailwindcss": "^3.4.0"
  }
}
`

// HTMX4Script returns the script tag for HTMX 4.
// For mobile apps, this would be bundled locally.
const HTMX4Script = `<script src="https://four.htmx.org/js/htmx.min.js"></script>`

// BaseHTML provides a minimal HTML template with HTMX 4 and Tailwind.
const BaseHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=no, viewport-fit=cover">
    <meta name="mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="default">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/assets/css/output.css">
    <script src="/assets/js/htmx.min.js"></script>
    <script src="/assets/js/irgo-bridge.js"></script>
</head>
<body class="bg-gray-50 text-gray-900 safe-area" hx-ext="irgo-bridge">
    <div id="app">
        {{.Content}}
    </div>
</body>
</html>
`
