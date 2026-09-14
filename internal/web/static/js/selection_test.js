import assert from 'node:assert/strict'
import test from 'node:test'

const stored = new Map([['processlab:rail-right', 'collapsed']])
const root = { dataset: { flowId: '1', selectedId: '1' }, style: { setProperty() {} } }
let focused = 0
const field = { focus() { focused++ }, select() {} }
globalThis.window = {
  innerHeight: 800,
  matchMedia: () => ({ matches: false }),
  localStorage: { getItem: (key) => stored.get(key), setItem: (key, value) => stored.set(key, value) }
}
globalThis.getComputedStyle = () => ({ getPropertyValue: () => '58' })
globalThis.document = {
  querySelector: (selector) => ({ '#workbench': root, '.property-form input[name="name"]': field })[selector] ?? null,
  querySelectorAll: () => []
}
let requests = 0
let complete
globalThis.htmx = {
  ajax(method, path) {
    requests++
    assert.equal(method, 'GET')
    assert.match(path, /selected=2$/)
    return new Promise((resolve) => { complete = resolve })
  }
}
const { editBlock, selectBlock } = await import('./selection.js')

test('editing opens a collapsed inspector and focuses only the requested block after rendering', async () => {
  const pending = editBlock({ dataset: { blockId: '2' } })
  assert.equal(root.dataset.railRight, 'expanded')
  assert.equal(stored.get('processlab:rail-right'), 'expanded')
  assert.equal(focused, 0, 'must not focus the previous block editor')
  root.dataset.selectedId = '2'
  complete()
  await pending
  assert.equal(focused, 1)

  const superseded = editBlock({ dataset: { blockId: '2' } })
  root.dataset.selectedId = '3'
  complete()
  await superseded
  assert.equal(focused, 1, 'a newer selection must not lose focus')
})


test('double-click reuses both pointer releases while selection is in flight', async () => {
  root.dataset.selectedId = '1'
  const before = requests
  const node = { dataset: { blockId: '2' } }
  const first = selectBlock(node)
  assert.equal(selectBlock(node), first)
  const editing = editBlock(node)
  assert.equal(requests - before, 1)
  const previousFocus = focused
  root.dataset.selectedId = '2'
  complete()
  await editing
  assert.equal(focused, previousFocus + 1)
})
