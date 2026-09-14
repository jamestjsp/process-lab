# Process Lab

- Performance is a priority; preserve numerical correctness and compatibility.
- Read and strictly apply **deep-go-design** for Go design changes and
  **fastweb** for HTTP, HTMX, and UI changes, and **go-optimisation** for Go
  performance regressions or optimization work. Report if a required skill is unavailable.
- Keep domain and simulation policy in `internal/studio`, HTTP and presentation
  in `internal/web`. Give each invariant one owner; hide internal sequencing.
- Use Go-rendered HTML and HTMX with real links/forms. Preserve URLs, history,
  focus, and safe initialization after swaps; keep JavaScript focused on interaction.
- Measure before optimizing. Use representative workloads, fair baseline
  comparisons, and profiles; report latency, allocations, and relevant tradeoffs.
  Never claim performance gains from functional tests alone.
- Add regression coverage and run relevant Go/JS tests. Verify UI changes in the
  browser and inspect actual exports; preserve user models with disposable test data.
- Preserve unrelated changes. Use cohesive commits and publish only as authorized.
