import assert from 'node:assert/strict'
import test from 'node:test'

const listeners = new Map()
globalThis.document = {
  addEventListener(name, listener) {
    listeners.set(name, listener)
  }
}

const { onBeforeSwap, onReapply } = await import('./reapply.js')

test('rebuilds client state once after the complete htmx 4 swap', () => {
  const calls = []
  onBeforeSwap((event) => calls.push(`before:${event.type}`))
  onReapply((event) => calls.push(`reapply:${event.type}`))

  assert.equal(listeners.has('htmx:after:settle'), false)
  assert.equal(listeners.has('htmx:before:history:restore'), false)
  listeners.get('htmx:before:swap')({ type: 'htmx:before:swap' })
  listeners.get('htmx:after:swap')({ type: 'htmx:after:swap' })

  assert.deepEqual(calls, [
    'before:htmx:before:swap',
    'reapply:htmx:after:swap'
  ])
})
