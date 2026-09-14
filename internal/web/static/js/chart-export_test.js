import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'

const source = readFileSync(new URL('./chart-export.js', import.meta.url), 'utf8').replace('export async function', 'async function')

test('PNG export uses chart proportions and an image source allowed by the app policy', async () => {
  const copy = { style: { setProperty() {} }, setAttribute() {}, querySelectorAll: () => [] }
  const svg = {
    cloneNode: () => copy, querySelectorAll: () => [],
    getBoundingClientRect: () => ({ width: 1000, height: 228 }),
    viewBox: { baseVal: { width: 780, height: 258 } }
  }
  const context = { fillRect() {}, fillText() {}, drawImage(...args) { this.draw = args } }
  const canvas = { getContext: () => context, toBlob: fn => fn({ type: 'image/png' }), toDataURL: () => 'data:image/png;base64,test' }
  let imageSource, clicked = false
  const link = { click() { clicked = true }, remove() {} }
  const readout = { append() {} }
  const sandbox = {
    getComputedStyle: () => ({ getPropertyValue: () => '#ffffff' }),
    XMLSerializer: class { serializeToString() { return '<svg xmlns="http://www.w3.org/2000/svg"/>' } },
    Image: class { set src(value) { imageSource = value } async decode() { assert.match(imageSource, /^data:image\/svg\+xml/) } },
    document: { documentElement: {}, body: { append() {} }, createElement: tag => tag === 'canvas' ? canvas : link },
    URL: { createObjectURL: () => 'blob:download', revokeObjectURL() {} },
    setTimeout() {}, encodeURIComponent,
  }
  vm.runInNewContext(source, sandbox)
  const root = { querySelector: selector => selector === '[data-chart-readout]' ? readout : { textContent: 'Temperature' } }
  await sandbox.saveChartPNG(root, svg)
  assert.equal(canvas.width, 2000)
  assert.equal(canvas.height, Math.round(2000 * 258 / 780) + 94)
  assert.equal(context.draw[2], 70)
  assert.equal(link.download, 'Temperature.png')
  assert.equal(clicked, true)
  assert.match(readout.textContent, /PNG prepared/)
})
