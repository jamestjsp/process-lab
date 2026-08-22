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

export function zoomedViewBox(base, zoom, anchor = {}) {
  const values = [base?.x, base?.y, base?.width, base?.height]
  if (!values.every(Number.isFinite) || base.width <= 0 || base.height <= 0) return null
  const boundedZoom = Math.min(MAX_CHART_ZOOM, Math.max(MIN_CHART_ZOOM, zoom))
  const anchorX = Number.isFinite(anchor.x) ? anchor.x : base.x + base.width / 2
  const anchorY = Number.isFinite(anchor.y) ? anchor.y : base.y + base.height / 2
  const width = base.width / boundedZoom
  const height = base.height / boundedZoom
  const xFraction = Math.min(1, Math.max(0, (anchorX - base.x) / base.width))
  const yFraction = Math.min(1, Math.max(0, (anchorY - base.y) / base.height))
  return {
    x: Math.min(base.x + base.width - width, Math.max(base.x, anchorX - xFraction * width)),
    y: Math.min(base.y + base.height - height, Math.max(base.y, anchorY - yFraction * height)),
    width,
    height,
    zoom: boundedZoom
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
  return [...root.querySelectorAll('[data-series-path]')]
    .map((path) => ({
      key: path.dataset?.seriesPath || '',
      name: path.dataset?.seriesName || path.dataset?.seriesPath || 'series',
      hidden: isHidden(path),
      vertices: parsePathVertices(path.getAttribute('d') || '')
    }))
    .filter((series) => series.vertices.length > 0)
}

function findSVG(root) {
  return root.tagName?.toLowerCase() === 'svg' ? root : root.querySelector('svg')
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

  const values = seriesValuesAtX(readSeries(state.root), targetX)
  if (values.length === 0) {
    state.clear()
    return
  }
  const active = values.find((entry) => entry.key === detail.activeKey) || values[0]
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

function setChartZoom(state, nextZoom) {
  if (!state.baseViewBox) return
  const viewBox = zoomedViewBox(state.baseViewBox, nextZoom, {
    x: state.cursorPixelX,
    y: state.cursorPixelY
  })
  if (!viewBox) return
  state.zoom = viewBox.zoom
  state.svg.setAttribute('viewBox', `${viewBox.x} ${viewBox.y} ${viewBox.width} ${viewBox.height}`)
  syncZoomControls(state)
}

function closestControl(state, event, selector) {
  const control = event.target?.closest?.(selector)
  return control && state.root.contains(control) ? control : null
}

function activateChartControl(state, event) {
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

function inspectPointer(state, event) {
  const point = eventPoint(event, state)
  if (!point || point.x < state.config.left || point.x > state.config.right ||
      point.y < state.config.top || point.y > state.config.bottom) return
  const visible = readSeries(state.root).filter((series) => !series.hidden)
  const candidates = visible.flatMap((series) =>
    series.vertices.map((vertex) => ({ ...vertex, key: series.key })))
  const nearest = nearestVertex(candidates, point.x, point.y)
  if (!nearest) return
  const domainX = invertScale(nearest.x, state.config.left, state.config.right,
    state.config.xMin, state.config.xMax, state.config.xScale)
  inspectionCoordinator.inspect(state.config.group, state, domainX, { activeKey: nearest.key })
}

function keyboardVertices(state) {
  return [...new Set(readSeries(state.root)
    .filter((series) => !series.hidden)
    .flatMap((series) => series.vertices.map((point) => point.x)))]
    .sort((left, right) => left - right)
}

function inspectKeyboard(state, event) {
  if (event.key === 'Escape') {
    event.preventDefault()
    inspectionCoordinator.clear(state.config.group)
    return
  }
  const vertices = keyboardVertices(state)
  const current = Number.isFinite(state.cursorPixelX)
    ? nearestVertex(vertices.map((x) => ({ x, y: 0 })), state.cursorPixelX)?.index ?? -1
    : -1
  const next = inspectionIndexForKey(event.key, current, vertices.length)
  if (next === undefined || next === null) return
  event.preventDefault()
  const domainX = invertScale(vertices[next], state.config.left, state.config.right,
    state.config.xMin, state.config.xMax, state.config.xScale)
  inspectionCoordinator.inspect(state.config.group, state, domainX)
}

function initializePlot(root) {
  const config = plotConfig(root)
  const svg = findSVG(root)
  if (!config || !svg) return null

  const existing = plotStates.get(root)
  if (existing) {
    if (existing.svg !== svg) {
      existing.cursor = null
      existing.baseViewBox = readViewBox(svg)
      existing.zoom = MIN_CHART_ZOOM
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

export function applyChartInspection(scope = globalThis.document) {
  return plotRoots(scope).map(initializePlot).filter(Boolean)
}

export function applySeriesVisibility() {
  const root = workbench()
  if (!root) return
  const hidden = hiddenSeries()
  root.querySelectorAll('[data-series-toggle]').forEach((button) => {
    const isVisible = !hidden.has(button.dataset.seriesToggle)
    button.setAttribute('aria-pressed', String(isVisible))
  })
  root.querySelectorAll('[data-series-path]').forEach((path) => {
    path.toggleAttribute('hidden', hidden.has(path.dataset.seriesPath))
  })
  for (const state of applyChartInspection(root)) {
    if (Number.isFinite(state.domainX)) state.show(state.domainX)
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('click', (event) => {
    const button = event.target.closest('[data-series-toggle]')
    if (!button || !workbench()?.contains(button)) return
    const hidden = hiddenSeries()
    const key = button.dataset.seriesToggle
    if (hidden.has(key)) hidden.delete(key)
    else hidden.add(key)
    saveHidden(hidden)
    applySeriesVisibility()
  })
}
