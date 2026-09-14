// Runs before stylesheets so a saved theme is applied before first paint.
(() => {
  const key = 'processlab:theme'
  const normalize = (value) => ['light', 'dark', 'system'].includes(value) ? value : 'light'
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  let preference = 'light'
  try { preference = normalize(window.localStorage.getItem(key)) } catch {}

  function apply() {
    document.documentElement.dataset.theme = preference === 'system'
      ? (media.matches ? 'dark' : 'light') : preference
    document.querySelectorAll('[data-theme-picker]').forEach((input) => {
      input.checked = input.value === preference
    })
  }

  document.addEventListener('change', (event) => {
    if (!event.target.matches('[data-theme-picker]')) return
    preference = normalize(event.target.value)
    try { window.localStorage.setItem(key, preference) } catch {}
    apply()
  })
  window.addEventListener('storage', (event) => {
    if (event.key !== key && event.key !== null) return
    preference = normalize(event.newValue)
    apply()
  })
  media.addEventListener('change', apply)
  document.addEventListener('DOMContentLoaded', apply)
  document.addEventListener('htmx:after:swap', apply)
  apply()
})()
