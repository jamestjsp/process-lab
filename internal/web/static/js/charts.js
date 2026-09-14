import { saveChartPNG } from './chart-export.js'
import { workbench } from './dom.js'

const SVG_NAMESPACE = 'http://www.w3.org/2000/svg'
const MIN_CHART_ZOOM = 1
const MAX_CHART_ZOOM = 4
const CHART_ZOOM_STEP = 1.25
const plotStates = new WeakMap()

const storageKey = () => {
  const root = workbench()
  return `processlab:hidden-series:${root ? root.dataset.flowId : 'default'}`
}

const trendLayoutKey = () => {
  const root = workbench()
  return `processlab:trend-layout:${root ? root.dataset.flowId : 'default'}`
}

export function normalizeTrendLayout(value) {
  return value === 'split' ? 'split' : 'overlay'
}

function trendLayout() {
  try {
    return normalizeTrendLayout(localStorage.getItem(trendLayoutKey()))
  } catch {
    return 'overlay'
  }
}

function saveTrendLayout(layout) {
  try {
    localStorage.setItem(trendLayoutKey(), normalizeTrendLayout(layout))
  } catch {
    // The server-rendered overlay remains usable when storage is unavailable.
  }
}

function hiddenSeries() {
  try {
    const stored = JSON.parse(localStorage.getItem(storageKey()) || '[]')
    return new Set(Array.isArray(stored) ? stored.map(String) : [])
  } catch {
    return new Set()
  }
}

function saveHidden(series) {
  localStorage.setItem(storageKey(), JSON.stringify([...series].sort()))
}

export function parsePathVertices(pathData) {
  if (typeof pathData !== 'string' || !pathData.trim()) return []
  const tokens = pathData.match(/[a-zA-Z]|[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?/g) || []
  const residue = pathData
    .replace(/[a-zA-Z]|[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?/g, '')
    .replace(/[\s,]/g, '')
  if (residue) return []

  const vertices = []
  let command = ''
  let current = { x: 0, y: 0 }
  let index = 0
  let movePair = false

  while (index < tokens.length) {
    if (/^[a-zA-Z]$/.test(tokens[index])) {
      command = tokens[index]
      index += 1
      movePair = command === 'M' || command === 'm'
      if (!['M', 'm', 'L', 'l'].includes(command)) return []
    }
    if (!command || index + 1 >= tokens.length) return []
    if (/^[a-zA-Z]$/.test(tokens[index]) || /^[a-zA-Z]$/.test(tokens[index + 1])) return []

    const x = Number(tokens[index])
    const y = Number(tokens[index + 1])
    if (!Number.isFinite(x) || !Number.isFinite(y)) return []
    index += 2

    const relative = command === 'm' || command === 'l'
    current = {
      x: relative ? current.x + x : x,
      y: relative ? current.y + y : y
    }
    vertices.push(current)

    if (movePair) {
      command = command === 'm' ? 'l' : 'L'
      movePair = false
    }
  }

  return vertices
}

export function invertScale(pixel, pixelMin, pixelMax, domainMin, domainMax, kind = 'linear') {
  const values = [pixel, pixelMin, pixelMax, domainMin, domainMax]
  if (!values.every(Number.isFinite) || pixelMin === pixelMax) return Number.NaN
  const ratio = (pixel - pixelMin) / (pixelMax - pixelMin)
  if (kind === 'log10') {
    if (domainMin <= 0 || domainMax <= 0) return Number.NaN
    const exponent = Math.log10(domainMin) + ratio * (Math.log10(domainMax) - Math.log10(domainMin))
    return 10 ** exponent
  }
  if (kind !== 'linear') return Number.NaN
  return domainMin + ratio * (domainMax - domainMin)
}

export function scaleValue(value, domainMin, domainMax, pixelMin, pixelMax, kind = 'linear') {
  const values = [value, domainMin, domainMax, pixelMin, pixelMax]
  if (!values.every(Number.isFinite) || domainMin === domainMax) return Number.NaN
  let ratio
  if (kind === 'log10') {
    if (value <= 0 || domainMin <= 0 || domainMax <= 0) return Number.NaN
    ratio = (Math.log10(value) - Math.log10(domainMin)) /
      (Math.log10(domainMax) - Math.log10(domainMin))
  } else if (kind === 'linear') {
    ratio = (value - domainMin) / (domainMax - domainMin)
  } else {
    return Number.NaN
  }
  return pixelMin + ratio * (pixelMax - pixelMin)
}

export function nearestVertex(vertices, targetX, targetY = Number.NaN) {
  let nearest = null
  let nearestDistance = Number.POSITIVE_INFINITY
  const includeY = Number.isFinite(targetY)
  for (let index = 0; index < vertices.length; index += 1) {
    const point = vertices[index]
    if (!Number.isFinite(point?.x) || !Number.isFinite(point?.y)) continue
    const xDistance = point.x - targetX
    const yDistance = includeY ? point.y - targetY : 0
    const distance = xDistance * xDistance + yDistance * yDistance
    if (distance < nearestDistance) {
      nearest = { ...point, index }
      nearestDistance = distance
    }
  }
  return nearest
}

export function seriesValuesAtX(series, pixelX) {
  return series
    .filter((entry) => !entry.hidden)
    .map((entry) => ({ ...entry, point: nearestVertex(entry.vertices, pixelX) }))
    .filter((entry) => entry.point)
}

export function inspectionIndexForKey(key, currentIndex, count) {
  if (count <= 0) return null
  switch (key) {
    case 'ArrowLeft':
      return currentIndex < 0 ? count - 1 : Math.max(0, currentIndex - 1)
    case 'ArrowRight':
      return currentIndex < 0 ? 0 : Math.min(count - 1, currentIndex + 1)
    case 'Home':
      return 0
    case 'End':
      return count - 1
    default:
      return undefined
  }
}

export function createInspectionCoordinator() {
  const groups = new Map()

  function members(group) {
    if (!groups.has(group)) groups.set(group, new Set())
    return groups.get(group)
  }

  function prune(group) {
    for (const plot of members(group)) {
      if (plot.isConnected && !plot.isConnected()) members(group).delete(plot)
    }
  }

  return {
    register(group, plot) {
      members(group).add(plot)
    },
    inspect(group, source, domainX, detail = {}) {
      prune(group)
      for (const plot of members(group)) {
        plot.show(domainX, plot === source ? detail : {})
      }
    },
    clear(group) {
      prune(group)
      for (const plot of members(group)) plot.clear()
    },
    size(group) {
      prune(group)
      return members(group).size
    }
  }
}

const inspectionCoordinator = createInspectionCoordinator()

function numberData(root, name) {
  const value = Number(root.dataset?.[name])
  return Number.isFinite(value) ? value : Number.NaN
}

function plotConfig(root) {
  const config = {
    id: root.dataset?.plotId || '',
    group: root.dataset?.plotGroup || root.dataset?.plotId || '',
    xMin: numberData(root, 'xMin'),
    xMax: numberData(root, 'xMax'),
    yMin: numberData(root, 'yMin'),
    yMax: numberData(root, 'yMax'),
    xScale: root.dataset?.xScale || 'linear',
    yScale: root.dataset?.yScale || 'linear',
    left: numberData(root, 'plotLeft'),
    right: numberData(root, 'plotRight'),
    top: numberData(root, 'plotTop'),
    bottom: numberData(root, 'plotBottom')
  }
  const numbers = [config.xMin, config.xMax, config.yMin, config.yMax,
    config.left, config.right, config.top, config.bottom]
  if (!numbers.every(Number.isFinite) || config.xMin === config.xMax || config.yMin === config.yMax ||
      config.left === config.right || config.top === config.bottom) {
    return null
  }
  if (!['linear', 'log10'].includes(config.xScale) || !['linear', 'log10'].includes(config.yScale)) {
    return null
  }
  if ((config.xScale === 'log10' && (config.xMin <= 0 || config.xMax <= 0)) ||
      (config.yScale === 'log10' && (config.yMin <= 0 || config.yMax <= 0))) return null
  return config
}

function isHidden(path) {
  return path.hidden === true || path.hasAttribute?.('hidden') || path.getAttribute?.('aria-hidden') === 'true'
}

function readSeries(root) {
  const paths = [...root.querySelectorAll('[data-series-path]')]
    .map((path) => ({
      key: path.dataset?.seriesPath || '',
      name: path.dataset?.seriesName || path.dataset?.seriesPath || 'series',
      hidden: isHidden(path),
      vertices: parsePathVertices(path.getAttribute('d') || '')
    }))
  const markers = [...root.querySelectorAll('.analysis-marker')].map((marker, index) => ({
    key: `marker:${index}`,
    name: (marker.getAttribute('class') || '').includes('analysis-marker-pole') ? 'Pole'
      : (marker.getAttribute('class') || '').includes('analysis-marker-zero') ? 'Zero' : 'Marker',
    hidden: isHidden(marker),
    vertices: [{ x: Number(marker.getAttribute('x')), y: Number(marker.getAttribute('y')) }]
  }))
  return [...paths, ...markers].filter((series) => series.vertices.length > 0)
}

function findSVG(root) {
  return root.tagName?.toLowerCase() === 'svg' ? root : [...root.querySelectorAll('svg')].find(svg => svg.getAttribute('aria-hidden') !== 'true')
}

function findReadout(root) {
  return root.querySelector('[data-chart-readout]') ||
    root.parentElement?.querySelector?.('[data-chart-readout]') || null
}

function readViewBox(svg) {
  const values = (svg.getAttribute?.('viewBox') || '').trim().split(/[\s,]+/).map(Number)
  if (values.length === 4 && values.every(Number.isFinite) && values[2] > 0 && values[3] > 0) {
    return { x: values[0], y: values[1], width: values[2], height: values[3] }
  }
  const viewBox = svg.viewBox?.baseVal
  if (Number.isFinite(viewBox?.x) && Number.isFinite(viewBox?.y) &&
      Number.isFinite(viewBox?.width) && viewBox.width > 0 &&
      Number.isFinite(viewBox?.height) && viewBox.height > 0) {
    return { x: viewBox.x, y: viewBox.y, width: viewBox.width, height: viewBox.height }
  }
  return null
}

function formatNumber(value) {
  if (!Number.isFinite(value)) return '—'
  if (value === 0) return '0'
  const absolute = Math.abs(value)
  if (absolute >= 10000 || absolute < 0.001) {
    return value.toExponential(4).replace(/\.0+(?=e)/, '').replace(/(\.\d*?)0+(?=e)/, '$1')
  }
  return Number(value.toPrecision(5)).toString()
}

function createSVGElement(svg, name, className) {
  const element = (svg.ownerDocument || globalThis.document).createElementNS(SVG_NAMESPACE, name)
  if (className) element.setAttribute('class', className)
  element.setAttribute('aria-hidden', 'true')
  return element
}

function ensureCursor(state) {
  if (state.cursor) return state.cursor
  const existing = state.root.querySelector('[data-chart-cursor]')
  if (existing) {
    state.cursor = {
      group: existing,
      xLine: existing.querySelector('[data-chart-cursor-x]'),
      yLine: existing.querySelector('[data-chart-cursor-y]')
    }
    if (state.cursor.xLine && state.cursor.yLine) return state.cursor
    existing.remove()
  }

  const group = createSVGElement(state.svg, 'g', 'chart-cursor')
  group.setAttribute('data-chart-cursor', '')
  group.setAttribute('hidden', '')
  group.style.pointerEvents = 'none'
  if (state.clipURL) group.setAttribute('clip-path', state.clipURL)
  const xLine = createSVGElement(state.svg, 'line', 'chart-cursor-line chart-cursor-line-x')
  const yLine = createSVGElement(state.svg, 'line', 'chart-cursor-line chart-cursor-line-y')
  xLine.setAttribute('data-chart-cursor-x', '')
  yLine.setAttribute('data-chart-cursor-y', '')
  group.append(xLine, yLine)
  state.svg.append(group)
  state.cursor = { group, xLine, yLine }
  return state.cursor
}

function setLine(line, x1, y1, x2, y2) {
  line.setAttribute('x1', String(x1))
  line.setAttribute('y1', String(y1))
  line.setAttribute('x2', String(x2))
  line.setAttribute('y2', String(y2))
}

function updateCursor(state, x, y, values) {
  const cursor = ensureCursor(state)
  const { config } = state
  cursor.group.removeAttribute('hidden')
  setLine(cursor.xLine, x, config.top, x, config.bottom)
  setLine(cursor.yLine, config.left, y, config.right, y)
  cursor.group.querySelectorAll('[data-chart-cursor-point]').forEach((point) => point.remove())
  for (const value of values) {
    const point = createSVGElement(state.svg, 'circle', 'chart-cursor-point')
    point.setAttribute('data-chart-cursor-point', value.key)
    point.setAttribute('cx', String(value.point.x))
    point.setAttribute('cy', String(value.point.y))
    point.setAttribute('r', '3')
    cursor.group.append(point)
  }
  if (globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
    cursor.group.style.transition = 'none'
    cursor.group.querySelectorAll('*').forEach((element) => { element.style.transition = 'none' })
  }
}

function showInspection(state, domainX, detail = {}) {
  state.config = plotConfig(state.root)
  if (!state.config) return
  const targetX = scaleValue(domainX, state.config.xMin, state.config.xMax,
    state.config.left, state.config.right, state.config.xScale)
  if (!Number.isFinite(targetX)) return

  if (targetX < state.config.left || targetX > state.config.right) {
    state.clear()
    return
  }
  let values = seriesValuesAtX(visibleSeries(state), targetX)
  if (values.length === 0) {
    state.clear()
    return
  }
  const active = values.find((entry) => entry.key === detail.activeKey) || values[0]
  if (detail.point && active.key === detail.activeKey) {
    active.point = nearestVertex(active.vertices, detail.point.x, detail.point.y) || active.point
  }
  values = values.filter((entry) => !entry.key.startsWith('marker:') || entry === active)
  state.activeKey = active.key
  const cursorX = active.point.x
  const actualDomainX = invertScale(cursorX, state.config.left, state.config.right,
    state.config.xMin, state.config.xMax, state.config.xScale)
  updateCursor(state, cursorX, active.point.y, values)
  state.cursorPixelX = cursorX
  state.cursorPixelY = active.point.y
  state.domainX = actualDomainX

  const readoutValues = values.map((entry) => {
    const value = invertScale(entry.point.y, state.config.bottom, state.config.top,
      state.config.yMin, state.config.yMax, state.config.yScale)
    return `${entry.name} ${formatNumber(value)}`
  })
  if (state.readout) state.readout.textContent = `x ${formatNumber(actualDomainX)}; ${readoutValues.join('; ')}`
}

function clearInspection(state) {
  state.cursor?.group.setAttribute('hidden', '')
  state.cursorPixelX = Number.NaN
  state.cursorPixelY = Number.NaN
  state.domainX = Number.NaN
  if (state.readout) state.readout.textContent = state.idleReadout
}

function syncCharacteristicControls(state) {
  const lines = [...state.root.querySelectorAll('[data-chart-characteristic-lines]')]
  const visible = lines.length === 0 || lines.some((line) => !line.hasAttribute('hidden'))
  state.root.querySelectorAll('[data-chart-characteristics]').forEach((button) => {
    button.setAttribute('aria-pressed', String(visible))
  })
}

function toggleCharacteristics(state) {
  const lines = [...state.root.querySelectorAll('[data-chart-characteristic-lines]')]
  const visible = lines.some((line) => !line.hasAttribute('hidden'))
  lines.forEach((line) => line.toggleAttribute('hidden', visible))
  syncCharacteristicControls(state)
}

function syncZoomControls(state) {
  const percentage = Math.round(state.zoom * 100)
  state.root.dataset.chartZoom = String(state.zoom)
  state.root.querySelectorAll('[data-chart-zoom-in]').forEach((button) => {
    button.disabled = !state.baseViewBox || state.zoom >= MAX_CHART_ZOOM
    button.setAttribute('aria-disabled', String(button.disabled))
    button.setAttribute('aria-label', `Zoom in chart, currently ${percentage}%`)
  })
  state.root.querySelectorAll('[data-chart-zoom-out]').forEach((button) => {
    button.disabled = !state.baseViewBox || state.zoom <= MIN_CHART_ZOOM
    button.setAttribute('aria-disabled', String(button.disabled))
    button.setAttribute('aria-label', `Zoom out chart, currently ${percentage}%`)
  })
  state.root.querySelectorAll('[data-chart-reset]').forEach((button) => {
    button.setAttribute('aria-label', `Reset chart view from ${percentage}%`)
  })
}

let clipSequence = 0

// Zoom in scale coordinates: logarithmic axes narrow by decades, not by
// arithmetic distance. The SVG viewport and all text sizes stay unchanged.
export function zoomedPlotConfig(base, current, zoom, anchor = {}) {
  const bounded = Math.min(MAX_CHART_ZOOM, Math.max(MIN_CHART_ZOOM, zoom))
  const next = { ...base }
  for (const axis of ['x', 'y']) {
    const min = `${axis}Min`
    const max = `${axis}Max`
    const kind = base[`${axis}Scale`]
    const forward = (value) => kind === 'log10' ? Math.log10(value) : value
    const inverse = (value) => kind === 'log10' ? 10 ** value : value
    const lower = forward(base[min])
    const upper = forward(base[max])
    const currentLower = forward(current[min])
    const currentUpper = forward(current[max])
    const position = Number.isFinite(anchor[axis]) ? forward(anchor[axis]) : (currentLower + currentUpper) / 2
    const fraction = Math.max(0, Math.min(1, (position - currentLower) / (currentUpper - currentLower)))
    const span = (upper - lower) / bounded
    const start = Math.max(lower, Math.min(upper - span, position - fraction * span))
    next[min] = bounded === 1 ? base[min] : inverse(start)
    next[max] = bounded === 1 ? base[max] : inverse(start + span)
  }
  return next
}

function initializeZoomGeometry(state) {
  state.baseConfig = { ...state.config }
  const { svg, config } = state
  const clip = createSVGElement(svg, 'clipPath')
  const id = `chart-data-clip-${++clipSequence}`
  clip.setAttribute('id', id)
  clip.setAttribute('clipPathUnits', 'userSpaceOnUse')
  const rect = createSVGElement(svg, 'rect')
  rect.setAttribute('x', config.left)
  rect.setAttribute('y', config.top)
  rect.setAttribute('width', config.right - config.left)
  rect.setAttribute('height', config.bottom - config.top)
  clip.append(rect)
  const defs = createSVGElement(svg, 'defs')
  state.clip = clip
  defs.append(clip)
  svg.append(defs)
  state.clipURL = `url(#${id})`
  state.geometry = [...svg.querySelectorAll('path, .chart-reference, .chart-reference-label, .analysis-marker')].map((element) => {
    element.setAttribute('clip-path', state.clipURL)
    const attributes = {}
    for (const name of ['d', 'x', 'y', 'x1', 'x2', 'y1', 'y2']) {
      const value = element.getAttribute(name)
      if (value !== null) attributes[name] = value
    }
    return { element, attributes }
  })
  state.ticks = [...svg.querySelectorAll('.x-label, .y-label')].map((element) => ({
    element,
    text: element.textContent,
    axis: element.classList.contains('x-label') ? 'x' : 'y'
  }))
}

function renderZoomGeometry(state) {
  const { baseConfig: base, config } = state
  const mapX = (pixel) => scaleValue(invertScale(pixel, base.left, base.right, base.xMin, base.xMax, base.xScale),
    config.xMin, config.xMax, config.left, config.right, config.xScale)
  const mapY = (pixel) => scaleValue(invertScale(pixel, base.bottom, base.top, base.yMin, base.yMax, base.yScale),
    config.yMin, config.yMax, config.bottom, config.top, config.yScale)
  for (const { element, attributes } of state.geometry) {
    for (const [name, value] of Object.entries(attributes)) {
      if (state.zoom === 1) {
        element.setAttribute(name, value)
      } else if (name === 'd') {
        // Server paths use absolute M/L coordinates. Preserve each move so
        // discontinuities and independent loci never gain a connecting segment.
        element.setAttribute(name, value.replace(/([ML])\s*([-+\d.eE]+)[ ,]+([-+\d.eE]+)/g,
          (_, command, x, y) => `${command} ${mapX(Number(x))} ${mapY(Number(y))}`))
      } else {
        element.setAttribute(name, String(name.startsWith('x') ? mapX(Number(value)) : mapY(Number(value))))
      }
    }
  }
  for (const { element, text, axis } of state.ticks) {
    const pixel = Number(element.getAttribute(axis))
    element.textContent = state.zoom === 1 ? text : formatNumber(axis === 'x'
      ? invertScale(pixel, config.left, config.right, config.xMin, config.xMax, config.xScale)
      : invertScale(pixel, config.bottom, config.top, config.yMin, config.yMax, config.yScale))
  }
}

function setChartZoom(state, nextZoom) {
  if (!state.baseViewBox) return
  if (!state.baseConfig) initializeZoomGeometry(state)
  const anchor = {
    x: state.domainX,
    y: invertScale(state.cursorPixelY, state.config.bottom, state.config.top,
      state.config.yMin, state.config.yMax, state.config.yScale)
  }
  state.config = zoomedPlotConfig(state.baseConfig, state.config, nextZoom, anchor)
  state.zoom = Math.min(MAX_CHART_ZOOM, Math.max(MIN_CHART_ZOOM, nextZoom))
  for (const name of ['xMin', 'xMax', 'yMin', 'yMax']) state.root.dataset[name] = String(state.config[name])
  renderZoomGeometry(state)
  state.cursor?.group.setAttribute('clip-path', state.clipURL)
  clearInspection(state)
  syncZoomControls(state)
}

function closestControl(state, event, selector) {
  const control = event.target?.closest?.(selector)
  return control && state.root.contains(control) ? control : null
}

function activateChartControl(state, event) {
  const expandButton = closestControl(state, event, '[data-chart-expand]')
  if (expandButton) {
    const marker = document.createComment('expanded plot position')
    const dialog = document.createElement('dialog')
    dialog.className = 'plot-dialog'
    dialog.setAttribute('aria-label', 'Expanded plot')
    const close = document.createElement('button')
    close.type = 'button'
    close.className = 'plot-dialog-close'
    close.textContent = '×'
    close.setAttribute('aria-label', 'Close expanded view')
    close.title = 'Close expanded view (Esc)'
    state.root.before(marker)
    dialog.append(close, state.root)
    document.body.append(dialog)
    expandButton.hidden = true
    close.addEventListener('click', () => dialog.close())
    dialog.addEventListener('close', () => {
      marker.replaceWith(state.root)
      expandButton.hidden = false
      dialog.remove()
      expandButton.focus()
    }, { once: true })
    dialog.showModal()
    close.focus()
    return
  }
  const exportButton = closestControl(state, event, '[data-chart-png]')
  if (exportButton) {
    exportButton.disabled = true
    saveChartPNG(state.root, state.svg).catch(() => {
      const readout = state.root.querySelector('[data-chart-readout]')
      if (readout) readout.textContent = 'PNG export failed. Please try again.'
    }).finally(() => { exportButton.disabled = false })
    return
  }
  if (closestControl(state, event, '[data-chart-characteristics]')) {
    toggleCharacteristics(state)
    return
  }
  if (closestControl(state, event, '[data-chart-zoom-in]')) {
    setChartZoom(state, state.zoom * CHART_ZOOM_STEP)
    return
  }
  if (closestControl(state, event, '[data-chart-zoom-out]')) {
    setChartZoom(state, state.zoom / CHART_ZOOM_STEP)
    return
  }
  if (closestControl(state, event, '[data-chart-reset]')) {
    setChartZoom(state, MIN_CHART_ZOOM)
    inspectionCoordinator.clear(state.config.group)
    return
  }
  if (closestControl(state, event, '[data-chart-clear]')) {
    inspectionCoordinator.clear(state.config.group)
  }
}

function eventPoint(event, state) {
  if (Number.isFinite(event.plotX) && Number.isFinite(event.plotY)) {
    return { x: event.plotX, y: event.plotY }
  }
  // The SVG can be letterboxed inside its CSS box, particularly in the
  // expanded dialog. Its screen transform accounts for preserveAspectRatio.
  const matrix = state.svg.getScreenCTM?.()
  if (matrix && state.svg.createSVGPoint) {
    const point = state.svg.createSVGPoint()
    point.x = event.clientX
    point.y = event.clientY
    try {
      return point.matrixTransform(matrix.inverse())
    } catch {
      return null
    }
  }
  const bounds = state.svg.getBoundingClientRect?.()
  const viewBox = state.svg.viewBox?.baseVal
  if (!bounds || bounds.width === 0 || bounds.height === 0) return null
  const left = Number.isFinite(viewBox?.x) ? viewBox.x : 0
  const top = Number.isFinite(viewBox?.y) ? viewBox.y : 0
  const width = Number.isFinite(viewBox?.width) && viewBox.width > 0 ? viewBox.width : bounds.width
  const height = Number.isFinite(viewBox?.height) && viewBox.height > 0 ? viewBox.height : bounds.height
  return {
    x: left + ((event.clientX - bounds.left) / bounds.width) * width,
    y: top + ((event.clientY - bounds.top) / bounds.height) * height
  }
}

function visibleSeries(state) {
  return readSeries(state.root).map((series) => ({
    ...series,
    vertices: series.vertices.filter((point) => point.x >= state.config.left && point.x <= state.config.right)
  }))
}

function inspectPointer(state, event) {
  const point = eventPoint(event, state)
  if (!point || point.x < state.config.left || point.x > state.config.right ||
      point.y < state.config.top || point.y > state.config.bottom) return
  const visible = visibleSeries(state).filter((series) => !series.hidden)
  const candidates = visible.flatMap((series) =>
    series.vertices.map((vertex) => ({ ...vertex, key: series.key })))
  const nearest = nearestVertex(candidates, point.x, point.y)
  if (!nearest) return
  const domainX = invertScale(nearest.x, state.config.left, state.config.right,
    state.config.xMin, state.config.xMax, state.config.xScale)
  inspectionCoordinator.inspect(state.config.group, state, domainX, { activeKey: nearest.key, point: nearest })
}

function inspectKeyboard(state, event) {
  // Toolbar buttons and form controls keep their native keyboard behavior.
  if (event.target && event.target !== state.root && event.target.closest?.('button, input, select, textarea')) return
  if (event.key === 'Escape') {
    event.preventDefault()
    inspectionCoordinator.clear(state.config.group)
    return
  }
  const series = visibleSeries(state).filter((entry) => !entry.hidden && entry.vertices.length)
  const active = series.find((entry) => entry.key === state.activeKey) || series[0]
  if (!active) return
  // Preserve sample order, including repeated X positions on complex-plane
  // curves. Sorting/deduplicating X loses half of a closed Nyquist curve.
  const vertices = series.every((entry) => entry.key.startsWith('marker:'))
    ? series.flatMap((entry) => entry.vertices.map((point) => ({ ...point, key: entry.key })))
    : active.vertices
  const current = Number.isFinite(state.cursorPixelX)
    ? nearestVertex(vertices, state.cursorPixelX, state.cursorPixelY)?.index ?? -1
    : -1
  const next = inspectionIndexForKey(event.key, current, vertices.length)
  if (next === undefined || next === null) return
  event.preventDefault()
  const point = vertices[next]
  const domainX = invertScale(point.x, state.config.left, state.config.right,
    state.config.xMin, state.config.xMax, state.config.xScale)
  inspectionCoordinator.inspect(state.config.group, state, domainX, { activeKey: point.key || active.key, point })
}

function initializePlot(root) {
  const config = plotConfig(root)
  const svg = findSVG(root)
  if (!config || !svg) return null

  const existing = plotStates.get(root)
  if (existing) {
    if (existing.svg !== svg || (existing.baseConfig && !svg.contains(existing.clip))) {
      existing.cursor = null
      existing.baseViewBox = readViewBox(svg)
      existing.zoom = MIN_CHART_ZOOM
      existing.baseConfig = null
    }
    const readout = findReadout(root)
    if (existing.readout !== readout) existing.idleReadout = readout?.textContent || ''
    existing.config = config
    existing.svg = svg
    existing.readout = readout
    syncCharacteristicControls(existing)
    syncZoomControls(existing)
    return existing
  }

  const readout = findReadout(root)
  const state = {
    root,
    workspace: workbench(),
    svg,
    config,
    readout,
    idleReadout: readout?.textContent || '',
    cursor: null,
    cursorPixelX: Number.NaN,
    cursorPixelY: Number.NaN,
    domainX: Number.NaN,
    baseViewBox: readViewBox(svg),
    zoom: MIN_CHART_ZOOM,
    isConnected: () => root.isConnected !== false,
    show(domainX, detail) {
      showInspection(state, domainX, detail)
    },
    clear() {
      clearInspection(state)
    }
  }
  plotStates.set(root, state)
  inspectionCoordinator.register(config.group, state)

  if (!root.hasAttribute?.('tabindex')) root.setAttribute?.('tabindex', '0')
  root.addEventListener('pointermove', (event) => inspectPointer(state, event))
  root.addEventListener('pointerleave', () => inspectionCoordinator.clear(state.config.group))
  root.addEventListener('keydown', (event) => inspectKeyboard(state, event))
  root.addEventListener('click', (event) => activateChartControl(state, event))
  syncCharacteristicControls(state)
  syncZoomControls(state)
  return state
}

function plotRoots(scope) {
  if (!scope) return []
  const roots = []
  if (scope.matches?.('[data-engineering-plot]')) roots.push(scope)
  roots.push(...(scope.querySelectorAll?.('[data-engineering-plot]') || []))
  return [...new Set(roots)]
}

function trendWorkspaceRoots(scope) {
  if (!scope) return []
  const roots = []
  if (scope.matches?.('[data-trend-workspace]')) roots.push(scope)
  roots.push(...(scope.querySelectorAll?.('[data-trend-workspace]') || []))
  return [...new Set(roots)]
}

export function applyTrendLayout(scope = globalThis.document) {
  const layout = trendLayout()
  for (const root of trendWorkspaceRoots(scope)) {
    root.dataset.trendLayout = layout
    root.querySelectorAll('[data-trend-layout-value]').forEach((button) => {
      button.setAttribute('aria-pressed', String(button.dataset.trendLayoutValue === layout))
    })
    root.querySelectorAll('[data-trend-layout-panel]').forEach((panel) => {
      panel.toggleAttribute('hidden', panel.dataset.trendLayoutPanel !== layout)
    })
  }
  return layout
}

export function applyChartInspection(scope = globalThis.document) {
  return plotRoots(scope).map(initializePlot).filter(Boolean)
}

function workspaceChartRoots(root) {
  const detached = plotRoots(globalThis.document).filter((plot) =>
    plotStates.get(plot)?.workspace === root && !root.contains(plot))
  return [root, ...detached]
}

function workspaceContainsChartControl(root, control) {
  return root && workspaceChartRoots(root).some((scope) => scope.contains(control))
}

function workspaceChartElements(root, selector) {
  return workspaceChartRoots(root).flatMap((scope) => [...scope.querySelectorAll(selector)])
}

export function applySeriesVisibility() {
  const root = workbench()
  if (!root) return
  const hidden = hiddenSeries()
  const buttons = workspaceChartElements(root, '[data-series-toggle]')
  buttons.forEach((button) => {
    const isVisible = !hidden.has(button.dataset.seriesToggle)
    button.setAttribute('aria-pressed', String(isVisible))
  })
  workspaceChartElements(root, '[data-series-path]').forEach((path) => {
    path.toggleAttribute('hidden', hidden.has(path.dataset.seriesPath))
  })
  workspaceChartElements(root, '[data-series-panel]').forEach((panel) => {
    panel.toggleAttribute('hidden', hidden.has(panel.dataset.seriesPanel))
  })
  const keys = new Set(buttons.map((button) => button.dataset.seriesToggle))
  workspaceChartElements(root, '[data-series-show-all]').forEach((button) => {
    button.disabled = ![...keys].some((key) => hidden.has(key))
  })
  applyTrendLayout(root)
  for (const state of workspaceChartRoots(root).flatMap((scope) => applyChartInspection(scope))) {
    if (Number.isFinite(state.domainX)) state.show(state.domainX)
  }
}

function updateSeriesSelection(root, selectedKey, isolate) {
  const hidden = hiddenSeries()
  const keys = workspaceChartElements(root, '[data-series-toggle]')
    .map((button) => button.dataset.seriesToggle)
  if (isolate) {
    for (const key of keys) {
      if (key === selectedKey) hidden.delete(key)
      else hidden.add(key)
    }
  } else if (selectedKey === '') {
    for (const key of keys) hidden.delete(key)
  } else if (hidden.has(selectedKey)) {
    hidden.delete(selectedKey)
  } else {
    hidden.add(selectedKey)
  }
  saveHidden(hidden)
  applySeriesVisibility()
}

if (typeof document !== 'undefined') {
  document.addEventListener('click', (event) => {
    const layoutButton = event.target.closest('[data-trend-layout-value]')
    if (layoutButton && workbench()?.contains(layoutButton)) {
      saveTrendLayout(layoutButton.dataset.trendLayoutValue)
      applyTrendLayout(workbench())
      return
    }
    const showAllButton = event.target.closest('[data-series-show-all]')
    if (showAllButton && workbench()?.contains(showAllButton)) {
      updateSeriesSelection(workbench(), '', false)
      return
    }
    const button = event.target.closest('[data-series-toggle]')
    if (!button || !workspaceContainsChartControl(workbench(), button)) return
    updateSeriesSelection(workbench(), button.dataset.seriesToggle, false)
  })
  document.addEventListener('dblclick', (event) => {
    const button = event.target.closest('[data-series-toggle]')
    if (!button || !workspaceContainsChartControl(workbench(), button)) return
    event.preventDefault()
    updateSeriesSelection(workbench(), button.dataset.seriesToggle, true)
  })
}
