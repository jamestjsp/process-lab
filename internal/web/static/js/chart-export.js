// Inline the rendered styles so the downloaded image is independent of page CSS.
export async function saveChartPNG(root, svg) {
  const copy = svg.cloneNode(true)
  const originals = [svg, ...svg.querySelectorAll('*')]
  const copies = [copy, ...copy.querySelectorAll('*')]
  originals.forEach((element, index) => {
    const style = getComputedStyle(element)
    for (const property of ['fill', 'stroke', 'stroke-width', 'stroke-dasharray', 'opacity', 'display', 'visibility', 'font-family', 'font-size', 'font-weight', 'text-anchor', 'dominant-baseline']) {
      copies[index].style.setProperty(property, style.getPropertyValue(property))
    }
  })
  const width = 1920
  const height = 1080
  const padding = 40
  const title = root.querySelector('figcaption strong, figcaption > span')?.textContent?.trim() || 'Process Lab plot'
  const legend = [...svg.querySelectorAll('[data-series-path]')]
    .filter(path => getComputedStyle(path).display !== 'none' && !path.hasAttribute('hidden'))
    .map(path => ({ name: path.dataset.seriesName || path.dataset.seriesPath, color: getComputedStyle(path).stroke }))
  const headerHeight = 70 + legend.length * 30
  const viewBox = svg.viewBox.baseVal
  const scale = Math.min((width - padding * 2) / viewBox.width,
    (height - headerHeight - padding * 2) / viewBox.height)
  const plotWidth = Math.round(viewBox.width * scale)
  const plotHeight = Math.round(viewBox.height * scale)
  copy.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  copy.setAttribute('width', plotWidth)
  copy.setAttribute('height', plotHeight)
  // Data images are permitted by the app policy; blob images are not.
  const source = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(new XMLSerializer().serializeToString(copy))
  let download
  try {
    const image = new Image()
    image.src = source
    await image.decode()
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    context.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--housing-raised').trim() || '#fff'
    context.fillRect(0, 0, width, canvas.height)
    context.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--ink').trim() || '#17212f'
    context.font = '600 26px sans-serif'
    context.fillText(title, 24, 38)
    context.font = '20px sans-serif'
    legend.forEach((series, index) => {
      const y = 68 + index * 30
      context.strokeStyle = series.color
      context.lineWidth = 3
      context.beginPath()
      context.moveTo(24, y)
      context.lineTo(50, y)
      context.stroke()
      context.fillText(series.name, 62, y + 6)
    })
    context.drawImage(image, (width - plotWidth) / 2,
      headerHeight + (height - headerHeight - plotHeight) / 2, plotWidth, plotHeight)
    const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/png'))
    if (!blob) throw new Error('PNG encoding failed')
    download = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = download
    link.download = `${title.trim().replace(/[^a-z0-9_-]+/gi, '-')}.png`
    document.body.append(link)
    link.click()
    link.remove()
    const readout = root.querySelector('[data-chart-readout]')
    if (readout) {
      readout.textContent = 'PNG prepared. '
      const retry = document.createElement('a')
      retry.href = canvas.toDataURL('image/png')
      retry.download = link.download
      retry.textContent = 'Download again'
      readout.append(retry)
    }
  } finally {
    if (download) setTimeout(() => URL.revokeObjectURL(download), 10000)
  }
}
