import assert from 'node:assert/strict'
import test from 'node:test'

const listeners = new Map()
const root = { dataset: { flowId: '1' } }
const field = { id: 'block-search', value: '' }
const status = { textContent: '' }
const forms = ['Sources Step Initial-to-final step', 'Math Gain Scale a signal', 'Math Matrix Gain Named vector gain y = Du'].map((text) => ({
  hidden: false,
  querySelector: () => ({ textContent: text })
}))
let focused = false
globalThis.document = {
  addEventListener: (name, callback) => listeners.set(name, callback),
  querySelector: (selector) => ({
    '#workbench': root,
    '#block-search': field,
    '#block-search-status': status,
    '.palette-list form:not([hidden]) button': forms.some((form) => !form.hidden) ? { focus() { focused = true } } : null
  })[selector],
  querySelectorAll: () => forms
}
const { applyLibrarySearch, matchesBlock } = await import('./library.js')

test('search matches all words across label, category, and description', () => {
  assert.equal(matchesBlock('Math Matrix Gain Named vector gain', '  GAIN vector '), true)
  assert.equal(matchesBlock('Math Gain Scale a signal', 'gain vector'), false)
  assert.equal(matchesBlock('Sources Step Initial-to-final step', 'sources'), true)
  assert.equal(matchesBlock('Sources Step', '   '), true)
})

test('search survives edits, reports no matches, and resets on sheet navigation', () => {
  applyLibrarySearch()
  assert.equal(status.textContent, '3 blocks available')
  field.value = 'vector gain'
  listeners.get('input')({ target: field })
  assert.deepEqual(forms.map((form) => form.hidden), [true, true, false])
  assert.equal(status.textContent, '1 block found')
  field.value = '' // server-rendered field after a workbench swap
  forms.forEach((form) => { form.hidden = false })
  applyLibrarySearch()
  assert.equal(field.value, 'vector gain')
  assert.deepEqual(forms.map((form) => form.hidden), [true, true, false])
  listeners.get('keydown')({ target: field, key: 'ArrowDown', preventDefault() {} })
  assert.equal(focused, true)
  field.value = 'not-a-block'
  listeners.get('input')({ target: field })
  assert.ok(forms.every((form) => form.hidden))
  assert.match(status.textContent, /No matching blocks/)
  focused = false
  listeners.get('keydown')({ target: field, key: 'ArrowDown', preventDefault() {} })
  assert.equal(focused, false)
  listeners.get('keydown')({ target: field, key: 'Escape', preventDefault() {} })
  assert.equal(field.value, '')
  assert.ok(forms.every((form) => !form.hidden))
  field.value = 'gain'
  listeners.get('input')({ target: field })
  root.dataset.flowId = '2'
  applyLibrarySearch()
  assert.equal(field.value, '')
  assert.ok(forms.every((form) => !form.hidden))
})
