import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'

const source = readFileSync(new URL('../theme.js', import.meta.url), 'utf8')

function boot(saved, dark = false, blocked = false) {
  const events = new Map()
  const selectors = ['light', 'dark', 'system'].map(value => ({ value, checked: false }))
  const media = { matches: dark, addEventListener: (type, fn) => events.set('media', fn) }
  const root = { dataset: {} }
  const storage = new Map([['processlab:theme', saved]])
  vm.runInNewContext(source, {
    document: { documentElement: root, querySelectorAll: () => selectors, addEventListener: (type, fn) => events.set(type, fn) },
    window: {
      matchMedia: () => media,
      addEventListener: (type, fn) => events.set(type, fn),
      localStorage: {
        getItem(key) { if (blocked) throw Error('disabled'); return storage.get(key) },
        setItem(key, value) { if (blocked) throw Error('disabled'); storage.set(key, value) }
      }
    }
  })
  return { root, media, storage, selectors, events, change(value) {
    events.get('change')({ target: { value, matches: () => true } })
  } }
}

test('defaults to neutral light, validates saved values, and applies saved dark immediately', () => {
  for (const value of [null, 'unknown', '']) assert.equal(boot(value, true).root.dataset.theme, 'light')
  assert.equal(boot('dark').root.dataset.theme, 'dark')
})

test('persists choice, restores selectors after swaps, and follows system only when requested', () => {
  const app = boot('light')
  app.change('dark')
  assert.equal(app.storage.get('processlab:theme'), 'dark')
  app.selectors[1].checked = false
  app.events.get('htmx:after:swap')()
  assert.deepEqual(app.selectors.map(input => input.checked), [false, true, false])
  app.media.matches = false
  app.events.get('media')()
  assert.equal(app.root.dataset.theme, 'dark')
  app.change('system')
  assert.equal(app.root.dataset.theme, 'light')
  app.media.matches = true
  app.events.get('media')()
  assert.equal(app.root.dataset.theme, 'dark')
  assert.deepEqual(app.selectors.map(input => input.checked), [false, false, true])
  app.events.get('storage')({ key: 'processlab:theme', newValue: 'light' })
  assert.equal(app.root.dataset.theme, 'light')
})

test('theme switching remains usable with browser storage disabled', () => {
  const app = boot('dark', true, true)
  assert.equal(app.root.dataset.theme, 'light')
  app.change('dark')
  assert.equal(app.root.dataset.theme, 'dark')
})
