import { workbench } from './dom.js'

let query = ''
let flowID = ''

export function matchesBlock(text, search) {
  const haystack = text.toLocaleLowerCase()
  return search.trim().toLocaleLowerCase().split(/\s+/).every((word) => haystack.includes(word))
}

export function applyLibrarySearch() {
  const root = workbench()
  const field = document.querySelector('#block-search')
  if (!root || !field) return
  if (flowID !== root.dataset.flowId) query = ''
  flowID = root.dataset.flowId
  field.value = query
  const forms = Array.from(document.querySelectorAll('.palette-list form'))
  let count = 0
  forms.forEach((form) => {
    form.hidden = !matchesBlock(form.querySelector('.palette-text').textContent, query)
    if (!form.hidden) count++
  })
  document.querySelector('#block-search-status').textContent = count
    ? `${count} block${count === 1 ? '' : 's'}${query.trim() ? ' found' : ' available'}`
    : 'No matching blocks. Try another name or category.'
}

document.addEventListener('input', (event) => {
  if (event.target.id !== 'block-search') return
  query = event.target.value
  applyLibrarySearch()
})

document.addEventListener('keydown', (event) => {
  if (event.target.id !== 'block-search') return
  if (event.key === 'Escape') {
    event.preventDefault()
    query = ''
    applyLibrarySearch()
  } else if (event.key === 'ArrowDown') {
    event.preventDefault()
    document.querySelector('.palette-list form:not([hidden]) button')?.focus()
  }
})
