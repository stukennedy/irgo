/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./templates/**/*.{templ,go}",
    "./handlers/**/*.go",
    "./static/**/*.html",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
