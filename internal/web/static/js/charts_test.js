import assert from 'node:assert/strict'
import test from 'node:test'

class FakeElement {
  constructor(tagName, { dataset = {}, attributes = {}, textContent = '' } = {}) {
    this.tagName = tagName.toUpperCase()
    this.dataset = { ...dataset }
    this.attributes = new Map(Object.entries(attributes))
    this.textContent = textContent
    this.children = []
    this.parentElement = null
    this.listeners = new Map()
    this.style = {}
    this.isConnected = true
    this.classList = { contains: (name) => (this.getAttribute('class') || '').split(' ').includes(name) }
    this.ownerDocument = fakeDocument
  }

  append(...children) {
    for (const child of children) {
      child.parentElement = this
      child.ownerDocument = this.ownerDocument
      this.children.push(child)
    }
  }

  addEventListener(name, listener) {
    if (!this.listeners.has(name)) this.listeners.set(name, [])
    this.listeners.get(name).push(listener)
  }

  emit(name, event = {}) {
    for (const listener of this.listeners.get(name) || []) listener(event)
  }

  matches(selector) {
    if (selector.includes(',')) return selector.split(',').some((part) => this.matches(part.trim()))
    if (selector.startsWith('.')) return (this.getAttribute('class') || '').split(' ').includes(selector.slice(1))
    if (selector === 'path') return this.tagName === 'PATH'
    if (selector === '*') return true
    if (selector === 'svg') return this.tagName === 'SVG'
    const attribute = selector.match(/^\[([^\]]+)\]$/)?.[1]
    if (!attribute) return false
    if (attribute.startsWith('data-')) {
      const key = attribute.slice(5).replace(/-([a-z])/g, (_, letter) => letter.toUpperCase())
      return Object.hasOwn(this.dataset, key) || this.attributes.has(attribute)
    }
    return this.attributes.has(attribute)
  }

  closest(selector) {
    let element = this
    while (element) {
      if (element.matches(selector)) return element
      element = element.parentElement
    }
    return null
  }

  contains(element) {
    let candidate = element
    while (candidate) {
      if (candidate === this) return true
      candidate = candidate.parentElement
    }
    return false
  }

  querySelectorAll(selector) {
    const matches = []
    const visit = (element) => {
      for (const child of element.children) {
        if (child.matches(selector)) matches.push(child)
        visit(child)
      }
    }
    visit(this)
    return matches
  }

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] || null
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value))
  }

  getAttribute(name) {
    return this.attributes.get(name) ?? null
  }

  hasAttribute(name) {
    return this.attributes.has(name)
  }

  removeAttribute(name) {
    this.attributes.delete(name)
  }

  toggleAttribute(name, force) {
    if (force) this.attributes.set(name, '')
    else this.attributes.delete(name)
  }

  remove() {
    if (!this.parentElement) return
    this.parentElement.children = this.parentElement.children.filter((child) => child !== this)
    this.parentElement = null
    this.isConnected = false
  }
}

const documentListeners = new Map()
const fakeDocument = {
  addEventListener(name, listener) {
    if (!documentListeners.has(name)) documentListeners.set(name, [])
    documentListeners.get(name).push(listener)
  },
  createElementNS(_namespace, name) {
    return new FakeElement(name)
  },
  querySelector() {
    return null
  },
  querySelectorAll() {
    return []
  }
}

globalThis.document = fakeDocument
globalThis.matchMedia = () => ({ matches: true })

const {
  applyChartInspection,
  createInspectionCoordinator,
  inspectionIndexForKey,
  invertScale,
  nearestVertex,
  normalizeTrendLayout,
  parsePathVertices,
  scaleValue,
  seriesValuesAtX,
  zoomedPlotConfig
} = await import('./charts.js')

test('normalizes trend layouts to the server-rendered overlay fallback', () => {
  assert.equal(normalizeTrendLayout('split'), 'split')
  assert.equal(normalizeTrendLayout('overlay'), 'overlay')
  assert.equal(normalizeTrendLayout('unexpected'), 'overlay')
  assert.equal(normalizeTrendLayout(null), 'overlay')
})

function seriesPath(key, name, pathData, hidden = false) {
  const path = new FakeElement('path', {
    dataset: { seriesPath: key, seriesName: name },
    attributes: { d: pathData }
  })
  if (hidden) path.setAttribute('hidden', '')
  return path
}

let plotSequence = 0

function engineeringPlot({ group, paths, readoutText = 'Move over the plot for exact values.' }) {
  plotSequence += 1
  const root = new FakeElement('figure', {
    dataset: {
      engineeringPlot: '',
      plotId: `plot-${plotSequence}`,
      plotGroup: group,
      xMin: '0',
      xMax: '10',
      yMin: '0',
      yMax: '100',
      xScale: 'linear',
      yScale: 'linear',
      plotLeft: '10',
      plotRight: '110',
      plotTop: '10',
      plotBottom: '90'
    }
  })
  const svg = new FakeElement('svg', { attributes: { viewBox: '0 0 120 100' } })
  const characteristicLines = new FakeElement('g', {
    dataset: { chartCharacteristicLines: '' }
  })
  characteristicLines.append(new FakeElement('line'))
  const controls = new FakeElement('div')
  const characteristics = new FakeElement('button', {
    dataset: { chartCharacteristics: '' },
    attributes: { 'aria-pressed': 'true' }
  })
  const clear = new FakeElement('button', { dataset: { chartClear: '' } })
  const zoomIn = new FakeElement('button', { dataset: { chartZoomIn: '' } })
  const zoomOut = new FakeElement('button', { dataset: { chartZoomOut: '' } })
  const reset = new FakeElement('button', { dataset: { chartReset: '' } })
  // Toolbar icons precede the actual plot and must never become its SVG target.
  for (const button of [characteristics, clear, zoomIn, zoomOut, reset]) {
    button.append(new FakeElement('svg', { attributes: { 'aria-hidden': 'true', viewBox: '0 0 24 24' } }))
  }
  controls.append(characteristics, clear, zoomIn, zoomOut, reset)
  const readout = new FakeElement('output', {
    dataset: { chartReadout: '' },
    textContent: readoutText
  })
  svg.append(characteristicLines, ...paths)
  root.append(controls, svg, readout)
  return {
    root,
    svg,
    readout,
    characteristicLines,
    controls: { characteristics, clear, zoomIn, zoomOut, reset }
  }
}

test('parses absolute, implicit, relative, and scientific M/L vertices', () => {
  assert.deepEqual(parsePathVertices('M 10,20 L 30 40 50 60'), [
    { x: 10, y: 20 },
    { x: 30, y: 40 },
    { x: 50, y: 60 }
  ])
  assert.deepEqual(parsePathVertices('m 1 2 3 4 l -2 1e1'), [
    { x: 1, y: 2 },
    { x: 4, y: 6 },
    { x: 2, y: 16 }
  ])
  assert.deepEqual(parsePathVertices('M 0 0 C 1 2 3 4 5 6'), [])
  assert.deepEqual(parsePathVertices('M 0'), [])
})

test('inverts linear and logarithmic scales, including reversed pixel axes', () => {
  assert.equal(invertScale(50, 0, 100, -10, 10), 0)
  assert.equal(invertScale(25, 100, 0, 0, 40), 30)
  assert.ok(Math.abs(invertScale(50, 0, 100, 0.1, 1000, 'log10') - 10) < 1e-12)
  assert.ok(Math.abs(scaleValue(10, 0.1, 1000, 0, 100, 'log10') - 50) < 1e-12)
  assert.ok(Number.isNaN(invertScale(50, 0, 100, -1, 100, 'log10')))
  assert.ok(Number.isNaN(invertScale(50, 0, 100, 0, 1, 'unknown')))
})

test('selects the nearest rendered vertex by x or by pointer distance', () => {
  const points = [{ x: 10, y: 80 }, { x: 50, y: 20 }, { x: 90, y: 70 }]
  assert.deepEqual(nearestVertex(points, 54), { x: 50, y: 20, index: 1 })
  assert.deepEqual(nearestVertex(points, 60, 68), { x: 90, y: 70, index: 2 })
})

test('synchronizes inspection and clearing within a linked plot group', () => {
  const coordinator = createInspectionCoordinator()
  const calls = []
  const magnitude = {
    show: (x, detail) => calls.push(['magnitude', x, detail]),
    clear: () => calls.push(['clear-magnitude'])
  }
  const phase = {
    show: (x, detail) => calls.push(['phase', x, detail]),
    clear: () => calls.push(['clear-phase'])
  }
  coordinator.register('bode', magnitude)
  coordinator.register('bode', phase)
  coordinator.inspect('bode', magnitude, 12.5, { activeKey: 'gain' })
  coordinator.clear('bode')

  assert.deepEqual(calls, [
    ['magnitude', 12.5, { activeKey: 'gain' }],
    ['phase', 12.5, {}],
    ['clear-magnitude'],
    ['clear-phase']
  ])
})

test('maps keyboard commands to bounded vertex navigation', () => {
  assert.equal(inspectionIndexForKey('ArrowRight', -1, 4), 0)
  assert.equal(inspectionIndexForKey('ArrowRight', 3, 4), 3)
  assert.equal(inspectionIndexForKey('ArrowLeft', -1, 4), 3)
  assert.equal(inspectionIndexForKey('ArrowLeft', 0, 4), 0)
  assert.equal(inspectionIndexForKey('Home', 2, 4), 0)
  assert.equal(inspectionIndexForKey('End', 1, 4), 3)
  assert.equal(inspectionIndexForKey('Enter', 1, 4), undefined)
  assert.equal(inspectionIndexForKey('End', 0, 0), null)
})

test('excludes hidden series from values at the selected domain position', () => {
  const values = seriesValuesAtX([
    { key: 'visible', hidden: false, vertices: [{ x: 10, y: 20 }, { x: 50, y: 60 }] },
    { key: 'hidden', hidden: true, vertices: [{ x: 10, y: 30 }, { x: 50, y: 70 }] }
  ], 48)

  assert.equal(values.length, 1)
  assert.equal(values[0].key, 'visible')
  assert.deepEqual(values[0].point, { x: 50, y: 60, index: 1 })
})

test('initializes once and supports pointer, keyboard, hidden-series, and reduced-motion behavior', () => {
  const { root, svg, readout } = engineeringPlot({
    group: `interaction-${plotSequence}`,
    paths: [
      seriesPath('temperature', 'Temperature', 'M 10 90 L 60 50 L 110 10'),
      seriesPath('valve', 'Valve', 'M 10 10 L 60 30 L 110 50', true)
    ]
  })

  applyChartInspection(root)
  applyChartInspection(root)
  assert.equal(root.listeners.get('pointermove').length, 1)
  assert.equal(root.listeners.get('pointerleave').length, 1)
  assert.equal(root.listeners.get('keydown').length, 1)
  assert.equal(root.getAttribute('tabindex'), '0')

  root.emit('pointermove', { plotX: 60, plotY: 50 })
  assert.equal(readout.textContent, 'x 5; Temperature 50')
  assert.doesNotMatch(readout.textContent, /Valve/)
  const cursor = svg.querySelector('[data-chart-cursor]')
  assert.ok(cursor)
  assert.equal(cursor.hasAttribute('hidden'), false)
  assert.equal(cursor.style.transition, 'none')
  assert.equal(cursor.querySelectorAll('[data-chart-cursor-point]').length, 1)

  let prevented = false
  root.emit('keydown', { key: 'ArrowRight', preventDefault: () => { prevented = true } })
  assert.equal(prevented, true)
  assert.equal(readout.textContent, 'x 10; Temperature 100')
  root.emit('keydown', { key: 'Home', preventDefault() {} })
  assert.equal(readout.textContent, 'x 0; Temperature 0')
  root.emit('keydown', { key: 'End', preventDefault() {} })
  assert.equal(readout.textContent, 'x 10; Temperature 100')
  root.emit('keydown', { key: 'Escape', preventDefault() {} })
  assert.equal(cursor.hasAttribute('hidden'), true)
  assert.equal(readout.textContent, 'Move over the plot for exact values.')
})

test('pointer inspection synchronizes linked plot roots by domain x', () => {
  const group = `linked-${plotSequence}`
  const magnitude = engineeringPlot({
    group,
    paths: [seriesPath('magnitude', 'Magnitude', 'M 10 90 L 60 50 L 110 10')]
  })
  const phase = engineeringPlot({
    group,
    paths: [seriesPath('phase', 'Phase', 'M 10 10 L 60 30 L 110 50')]
  })
  const scope = new FakeElement('section')
  scope.append(magnitude.root, phase.root)

  applyChartInspection(scope)
  magnitude.root.emit('pointermove', { plotX: 60, plotY: 50 })

  assert.match(magnitude.readout.textContent, /^x 5;/)
  assert.match(phase.readout.textContent, /^x 5;/)
  assert.equal(phase.svg.querySelector('[data-chart-cursor]').hasAttribute('hidden'), false)
  magnitude.root.emit('pointerleave')
  assert.equal(magnitude.svg.querySelector('[data-chart-cursor]').hasAttribute('hidden'), true)
  assert.equal(phase.svg.querySelector('[data-chart-cursor]').hasAttribute('hidden'), true)
})

test('characteristic, clear, zoom, and reset controls remain functional after repeated initialization', () => {
  const plot = engineeringPlot({
    group: `controls-${plotSequence}`,
    paths: [seriesPath('response', 'Response', 'M 10 90 L 60 50 L 110 10')]
  })

  applyChartInspection(plot.root)
  applyChartInspection(plot.root)
  assert.equal(plot.root.listeners.get('click').length, 1)
  assert.equal(plot.controls.characteristics.getAttribute('aria-pressed'), 'true')
  assert.equal(plot.controls.zoomOut.disabled, true)
  assert.equal(plot.root.dataset.chartZoom, '1')

  plot.root.emit('click', { target: plot.controls.characteristics })
  assert.equal(plot.characteristicLines.hasAttribute('hidden'), true)
  assert.equal(plot.controls.characteristics.getAttribute('aria-pressed'), 'false')
  plot.root.emit('click', { target: plot.controls.characteristics })
  assert.equal(plot.characteristicLines.hasAttribute('hidden'), false)
  assert.equal(plot.controls.characteristics.getAttribute('aria-pressed'), 'true')

  plot.root.emit('pointermove', { plotX: 60, plotY: 50 })
  const cursor = plot.svg.querySelector('[data-chart-cursor]')
  assert.equal(cursor.hasAttribute('hidden'), false)
  plot.root.emit('click', { target: plot.controls.clear })
  assert.equal(cursor.hasAttribute('hidden'), true)
  assert.equal(plot.readout.textContent, 'Move over the plot for exact values.')

  for (let index = 0; index < 20; index += 1) {
    plot.root.emit('click', { target: plot.controls.zoomIn })
  }
  assert.equal(plot.root.dataset.chartZoom, '4')
  assert.equal(plot.controls.zoomIn.disabled, true)
  assert.equal(plot.svg.getAttribute('viewBox'), '0 0 120 100')
  assert.equal(Number(plot.root.dataset.xMax) - Number(plot.root.dataset.xMin), 2.5)
  assert.match(plot.controls.zoomOut.getAttribute('aria-label'), /400%/)

  plot.root.emit('pointermove', { plotX: 60, plotY: 50 })
  plot.root.emit('click', { target: plot.controls.reset })
  assert.equal(plot.root.dataset.chartZoom, '1')
  assert.equal(plot.svg.getAttribute('viewBox'), '0 0 120 100')
  assert.equal(cursor.hasAttribute('hidden'), true)
  assert.equal(plot.controls.zoomOut.disabled, true)
})


test('zoom narrows logarithmic decades and preserves an anchor in the current data window', () => {
  const base = { xMin: 0.1, xMax: 1000, yMin: -100, yMax: 100, xScale: 'log10', yScale: 'linear' }
  const zoomed = zoomedPlotConfig(base, base, 2)
  assert.ok(Math.abs(zoomed.xMin - 1) < 1e-12)
  assert.ok(Math.abs(zoomed.xMax - 100) < 1e-12)
  assert.equal(zoomed.yMin, -50)
  assert.equal(zoomed.yMax, 50)
  const anchored = zoomedPlotConfig(base, zoomed, 4, { x: 1, y: -50 })
  assert.equal(anchored.xMin, 1)
  assert.equal(anchored.xMax, 10)
  assert.equal(anchored.yMin, -50)
  assert.equal(anchored.yMax, 0)
  assert.deepEqual(zoomedPlotConfig(base, anchored, 1), base)
})

test('zoom redraws ticks and paths inside a fixed frame and reset restores exact source geometry', () => {
  const path = seriesPath('response', 'Response', 'M 10 90 L 60 50 M 80 34 L 110 10')
  const plot = engineeringPlot({ group: `range-${plotSequence}`, paths: [path] })
  const xTick = new FakeElement('text', { attributes: { class: 'chart-label x-label', x: '10', y: '95' }, textContent: '0' })
  const yTick = new FakeElement('text', { attributes: { class: 'chart-label y-label', x: '5', y: '10' }, textContent: '100' })
  plot.svg.append(xTick, yTick)
  applyChartInspection(plot.root)
  plot.root.emit('click', { target: plot.controls.zoomIn })
  assert.equal(plot.svg.getAttribute('viewBox'), '0 0 120 100')
  assert.equal(xTick.getAttribute('x'), '10')
  assert.equal(yTick.getAttribute('y'), '10')
  assert.equal(xTick.textContent, '1')
  assert.equal(yTick.textContent, '90')
  assert.match(path.getAttribute('clip-path'), /^url\(#chart-data-clip-/)
  assert.equal((path.getAttribute('d').match(/M/g) || []).length, 2)
  assert.notEqual(path.getAttribute('d'), 'M 10 90 L 60 50 M 80 34 L 110 10')
  plot.root.emit('pointermove', { plotX: 60, plotY: 50 })
  assert.equal(plot.readout.textContent, 'x 5; Response 50')
  plot.root.emit('keydown', { key: 'Home', preventDefault() {} })
  assert.equal(plot.readout.textContent, 'x 5; Response 50')
  // A visibility/layout refresh must retain the current range and base geometry.
  applyChartInspection(plot.root)
  plot.root.emit('click', { target: plot.controls.reset })
  assert.equal(xTick.textContent, '0')
  assert.equal(yTick.textContent, '100')
  assert.equal(path.getAttribute('d'), 'M 10 90 L 60 50 M 80 34 L 110 10')
  assert.equal(plot.root.dataset.xMin, '0')
  assert.equal(plot.root.dataset.xMax, '10')
})

test('logarithmic zoom keeps linked cursor values accurate across different visible ranges', () => {
  const group = `log-linked-${plotSequence}`
  const magnitude = engineeringPlot({ group, paths: [seriesPath('magnitude', 'Magnitude', 'M 10 90 L 60 50 L 110 10')] })
  const phase = engineeringPlot({ group, paths: [seriesPath('phase', 'Phase', 'M 10 10 L 60 30 L 110 50')] })
  for (const plot of [magnitude, phase]) {
    Object.assign(plot.root.dataset, { xMin: '0.01', xMax: '100', xScale: 'log10' })
  }
  const scope = new FakeElement('section')
  scope.append(magnitude.root, phase.root)
  applyChartInspection(scope)
  magnitude.root.emit('click', { target: magnitude.controls.zoomIn })
  magnitude.root.emit('keydown', { key: 'Home', preventDefault() {} })
  assert.equal(magnitude.readout.textContent, 'x 1; Magnitude 50')
  assert.equal(phase.readout.textContent, 'x 1; Phase 75')
  // A linked cursor outside this plot's visible range must not appear in its margins.
  phase.root.emit('keydown', { key: 'Home', preventDefault() {} })
  assert.equal(magnitude.svg.querySelector('[data-chart-cursor]').hasAttribute('hidden'), true)
  assert.equal(phase.readout.textContent, 'x 0.01; Phase 100')
})

test('pointer and keyboard preserve both branches at repeated complex-plane X coordinates', () => {
  const plot = engineeringPlot({ group: `complex-${plotSequence}`, paths: [seriesPath('nyquist', 'Nyquist', 'M 60 20 L 80 50 L 60 80')] })
  applyChartInspection(plot.root)
  plot.root.emit('pointermove', { plotX: 60, plotY: 80 })
  assert.equal(plot.readout.textContent, 'x 5; Nyquist 12.5')
  plot.root.emit('keydown', { key: 'Home', preventDefault() {} })
  assert.equal(plot.readout.textContent, 'x 5; Nyquist 87.5')
  plot.root.emit('keydown', { key: 'ArrowRight', preventDefault() {} })
  assert.equal(plot.readout.textContent, 'x 7; Nyquist 50')
  plot.root.emit('keydown', { key: 'ArrowRight', preventDefault() {} })
  assert.equal(plot.readout.textContent, 'x 5; Nyquist 12.5')
})

test('pointer inspection uses SVG screen coordinates inside a letterboxed expanded plot', () => {
  const plot = engineeringPlot({ group: `letterbox-${plotSequence}`, paths: [seriesPath('response', 'Response', 'M 10 90 L 60 50 L 110 10')] })
  plot.svg.getScreenCTM = () => ({ inverse: () => ({ scale: 2, left: 100, top: 200 }) })
  plot.svg.createSVGPoint = () => ({ x: 0, y: 0, matrixTransform(matrix) {
    return { x: (this.x - matrix.left) / matrix.scale, y: (this.y - matrix.top) / matrix.scale }
  } })
  applyChartInspection(plot.root)
  plot.root.emit('pointermove', { clientX: 220, clientY: 300 })
  assert.equal(plot.readout.textContent, 'x 5; Response 50')
})

test('pole-zero marker-only plots support pointer and ordered keyboard inspection', () => {
  const plot = engineeringPlot({ group: `markers-${plotSequence}`, paths: [] })
  plot.svg.append(new FakeElement('text', { attributes: { class: 'analysis-marker analysis-marker-pole', x: '40', y: '30' } }),
    new FakeElement('text', { attributes: { class: 'analysis-marker analysis-marker-zero', x: '70', y: '70' } }))
  applyChartInspection(plot.root)
  plot.root.emit('pointermove', { plotX: 70, plotY: 70 })
  assert.equal(plot.readout.textContent, 'x 6; Zero 25')
  plot.root.emit('keydown', { key: 'Home', preventDefault() {} })
  assert.equal(plot.readout.textContent, 'x 3; Pole 75')
  plot.root.emit('keydown', { key: 'ArrowRight', preventDefault() {} })
  assert.equal(plot.readout.textContent, 'x 6; Zero 25')
})

test('expanded plot retains series toggles and cursor interaction after moving outside the workbench', () => {
  const workspace = new FakeElement('main', { dataset: { flowId: 'expanded-test' } })
  const plot = engineeringPlot({ group: `expanded-${plotSequence}`, paths: [seriesPath('response', 'Response', 'M 10 90 L 60 50 L 110 10')] })
  const toggle = new FakeElement('button', { dataset: { seriesToggle: 'response' } })
  plot.root.append(toggle)
  workspace.append(plot.root)
  const oldQuery = fakeDocument.querySelector
  const oldQueryAll = fakeDocument.querySelectorAll
  const oldStorage = globalThis.localStorage
  const storage = new Map()
  fakeDocument.querySelector = (selector) => selector === '#workbench' ? workspace : null
  fakeDocument.querySelectorAll = (selector) => plot.root.matches(selector) ? [plot.root] : []
  globalThis.localStorage = { getItem: (key) => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value) }
  try {
    applyChartInspection(plot.root)
    plot.root.remove()
    new FakeElement('dialog').append(plot.root)
    plot.root.isConnected = true
    for (const handler of documentListeners.get('click')) handler({ target: toggle })
    assert.equal(toggle.getAttribute('aria-pressed'), 'false')
    assert.equal(plot.svg.querySelector('[data-series-path]').hasAttribute('hidden'), true)
    for (const handler of documentListeners.get('click')) handler({ target: toggle })
    assert.equal(toggle.getAttribute('aria-pressed'), 'true')
    plot.root.emit('keydown', { key: 'Home', preventDefault() {} })
    assert.equal(plot.readout.textContent, 'x 0; Response 0')
  } finally {
    fakeDocument.querySelector = oldQuery
    fakeDocument.querySelectorAll = oldQueryAll
    globalThis.localStorage = oldStorage
  }
})


test('zoom rebuilds clipping and source geometry after an identity-preserving morph', () => {
  const path = seriesPath('response', 'Response', 'M 10 90 L 60 50 L 110 10')
  const plot = engineeringPlot({ group: `morph-${plotSequence}`, paths: [path] })
  const originalDataset = { ...plot.root.dataset }
  applyChartInspection(plot.root)
  plot.root.emit('click', { target: plot.controls.zoomIn })
  const clip = path.getAttribute('clip-path')
  // A server morph keeps the SVG but restores server attributes and children.
  plot.svg.children.find((child) => child.tagName === 'DEFS').remove()
  path.setAttribute('d', 'M 10 80 L 60 40 L 110 20')
  path.removeAttribute('clip-path')
  plot.root.dataset = { ...originalDataset }
  applyChartInspection(plot.root)
  assert.equal(plot.root.dataset.chartZoom, '1')
  plot.root.emit('click', { target: plot.controls.zoomIn })
  assert.notEqual(path.getAttribute('clip-path'), clip)
  assert.ok(plot.svg.children.some((child) => child.tagName === 'DEFS'))
  plot.root.emit('click', { target: plot.controls.reset })
  assert.equal(path.getAttribute('d'), 'M 10 80 L 60 40 L 110 20')
})
