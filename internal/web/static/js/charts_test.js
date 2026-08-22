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
  zoomedViewBox
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

test('computes an anchored chart view box within bounded zoom limits', () => {
  const base = { x: 0, y: 0, width: 400, height: 200 }
  assert.deepEqual(zoomedViewBox(base, 2, { x: 100, y: 50 }), {
    x: 50,
    y: 25,
    width: 200,
    height: 100,
    zoom: 2
  })
  assert.equal(zoomedViewBox(base, 100).zoom, 4)
  assert.equal(zoomedViewBox(base, 0.1).zoom, 1)
  assert.equal(zoomedViewBox({ ...base, width: 0 }, 2), null)
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
  assert.notEqual(plot.svg.getAttribute('viewBox'), '0 0 120 100')
  assert.match(plot.controls.zoomOut.getAttribute('aria-label'), /400%/)

  plot.root.emit('pointermove', { plotX: 60, plotY: 50 })
  plot.root.emit('click', { target: plot.controls.reset })
  assert.equal(plot.root.dataset.chartZoom, '1')
  assert.equal(plot.svg.getAttribute('viewBox'), '0 0 120 100')
  assert.equal(cursor.hasAttribute('hidden'), true)
  assert.equal(plot.controls.zoomOut.disabled, true)
})
