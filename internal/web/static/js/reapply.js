// =====================================================================
// The htmx re-apply contract.
//
// Nearly every action swaps the whole #workbench fragment away, taking
// the pan and zoom transform, the selected classes, the drawn wires and
// the rail and dock attributes with it. All of that is client state, so
// the only way it survives an edit is to be put back afterwards.
//
// One entry point, one ordered list of steps. Modules register what they
// need to rebuild instead of each adding its own htmx:after:swap
// listener, so the order the state is rebuilt in is written down in one
// place (main.js) rather than emerging from script order.
//
// htmx 4 completes every main and out-of-band task before after:swap, so this
// is the first single event where every step can measure the final live DOM.
// History restoration re-fetches the page and follows the same swap path.
// =====================================================================

const steps = []
const beforeSwapSteps = []

export function onReapply(step) {
  steps.push(step)
}

export function onBeforeSwap(step) {
  beforeSwapSteps.push(step)
}

function reapply(event) {
  steps.forEach((step) => step(event))
}

document.addEventListener('htmx:before:swap', (event) => beforeSwapSteps.forEach((step) => step(event)))
document.addEventListener('htmx:after:swap', reapply)
