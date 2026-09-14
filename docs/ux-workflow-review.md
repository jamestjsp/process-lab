# Model authoring walkthrough

Validated September 13–14, 2026, in the integrated Codex browser, starting from
`9eceab3`. This records the authoring checks performed before the plotting updates.

## Observations and changes

| Workflow | Observed friction | Updated behavior |
| --- | --- | --- |
| Find and add blocks | All 29 blocks required scanning or scrolling; there was no search. | Find a block matches words across names, categories, and descriptions, reports the result count, and explains empty results. Down focuses the first result; Enter adds it; Escape clears the filter. |
| Edit a block | Double-clicking a block left a collapsed inspector hidden. Context-menu Rename also tried to focus a hidden field. | Double-click and Rename open the inspector, select the requested block, and focus its name after the response renders. Single-click selection retains existing behavior. |
| Change parameters and rerun | Editing a previously simulated model displayed “No run yet,” although its previous run was visible below. | The empty plot says “Model changed — run again” and explains that previous runs remain available. |

The comparison was bounded to familiar authoring patterns. MathWorks documents
[searching for blocks](https://www.mathworks.com/help/simulink/ug/add-blocks-to-models.html),
[connecting blocks](https://www.mathworks.com/help/simulink/ug/connect-blocks.html),
and parameter editing in its
[interactive modeling overview](https://www.mathworks.com/help/simulink/modeling-basics.html).
This change adds library search and direct inspector access, not Simulink's
canvas quick-insert menu or subsystem feature set.

## Actual browser validation

- Created a disposable project and built Step → Gain → Scope through the UI.
- Ran the disconnected model, saw “Gain 1 is not connected,” then recovered by
  connecting the named ports and rerunning.
- Baseline exploration produced 1.000, then 2.000 after editing the gain.
- The rebuilt version produced 3.000, then 4.000 after another edit. The new
  rerun prompt appeared between those runs, with prior run history retained.
- Reload restored the gain, both wires, the 4.000 result, and run history.
- Search found `sources step`, `gain scale`, and `matrix gain`; a nonexistent
  name showed no matches. Keyboard insertion, Escape, and filter restoration
  after an add all worked. Switching sheets and reloading cleared the filter;
  browser Back restored an interactive catalogue.
- Double-click with the inspector collapsed revealed the correct Gain editor
  and focused Block name. Browser testing exposed pointer-capture retargeting
  to the card; the handler now accommodates that event target.
- Search remained reachable and functional at 800px width. The viewport was
  restored afterward. Final console inspection reported no errors.

Persistent checks: all 30 JavaScript tests in `internal/web/static/js/*_test.js`
and `go test ./internal/web` pass. New tests cover search/filter lifecycle,
keyboard recovery, editor focus ordering and superseded selection, and the
distinction between never-run and edited-model messages. `git diff --check`
passes. The actual UI checks above used the integrated browser; the repository's
separate headless Chrome suite was not run.

