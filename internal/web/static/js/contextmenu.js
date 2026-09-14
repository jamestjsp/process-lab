// =====================================================================
// Canvas context menu.
//
// menu.js owns construction, placement, dismissal, and the keyboard;
// this file only says where the menu applies and what is in it. The
// native menu is suppressed only over the sheet; over the rails and
// the dock the browser's own menu stays available, where it is useful.
// =====================================================================
import { setStatus, workbench } from './dom.js'
import { geometry } from './geometry.js'
import { fitView, resetZoom, screenToSheet } from './viewport.js'
import {
  deleteSelection, duplicateSelection, fitSelection, isSelected,
  selectAll, editBlock, selectionSize, setSelection
} from './selection.js'

function menuItems(node, point) {
  if (node) {
    const plural = selectionSize() > 1 ? ` ${selectionSize()} blocks` : ''
    return [
      { label: 'Rename', run: () => editBlock(node) },
      { label: 'Flip horizontally', run: () => flipBlock(node) },
      { label: `Duplicate${plural}`, run: duplicateSelection },
      { label: 'Disconnect all wires', run: () => disconnectBlock(node) },
      { label: `Fit to${plural || ' this block'}`, run: fitSelection },
      { label: `Delete${plural}`, run: deleteSelection, danger: true }
    ]
  }
  return [
    { label: 'Add block here', submenu: paletteChoices(point) },
    { label: 'Select all', run: selectAll },
    { label: 'Fit to contents', run: fitView },
    { label: 'Reset zoom', run: resetZoom }
  ]
}

// Read the block catalogue off the palette rather than duplicating it,
// so a new block kind on the server appears here with no client change.
function paletteChoices(point) {
  const { grid } = geometry()
  return Array.from(document.querySelectorAll('.palette-list form')).map((form) => {
    const kind = form.querySelector('[name="kind"]').value
    const label = form.querySelector('strong').textContent
    return {
      label,
      run: () => {
        const root = workbench()
        if (!root) return
        htmx.ajax('POST', `/flows/${root.dataset.flowId}/blocks`, {
          target: '#workbench',
          swap: 'outerMorph',
          values: {
            kind,
            x: String(Math.round(point.x / grid) * grid),
            y: String(Math.round(point.y / grid) * grid)
          }
        })
      }
    }
  })
}

async function flipBlock(node) {
  const id = node.dataset.blockId
  await htmx.ajax('POST', `/blocks/${id}/flip`, { target: '#workbench', swap: 'outerMorph' })
  const block = document.getElementById(`block-card-${id}`)
  if (block) {
    block.tabIndex = -1
    block.focus({ preventScroll: true })
  }
}

function disconnectBlock(node) {
  htmx.ajax('DELETE', `/blocks/${node.dataset.blockId}/connections`, {
    target: '#workbench',
    swap: 'outerMorph'
  })
  setStatus('Signal wires removed')
}

// No restoreFocus: Escape returning focus to the region it was raised
// from is what the menu does by default, and for this region the host is
// the canvas itself.
window.ProcessLab.menu.register({
  selector: '#flow-canvas',
  items: (event) => {
    const node = event.target.closest('.block-card')
    // Right-clicking outside the current selection re-targets it; inside
    // it, the existing selection and its plural actions are kept.
    if (node && !isSelected(node.dataset.blockId)) setSelection([node.dataset.blockId])
    if (!node) setSelection([])
    return menuItems(node, screenToSheet(event.clientX, event.clientY))
  }
})
