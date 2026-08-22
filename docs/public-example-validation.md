# Public control-example validation

This report validates Process Lab against publicly documented Simulink and
control-system examples. It tests the documented equations and block semantics;
it does not claim file-format, solver, code-generation, or complete product
equivalence with MATLAB or Simulink.

The validation date is 2026-08-22. All Process Lab time values are seconds.
Expected values are analytic calculations or independent equation integrations,
not MATLAB or Simulink output unless a source explicitly publishes the number.

## Validation matrix

| Case | Public authority | Process Lab mapping | Quantitative oracle | Coverage |
|---|---|---|---|---|
| CTMS cruise-control PI | [CTMS Simulink Controller Design](https://ctms.engin.umich.edu/CTMS/index.php?example=CruiseControl&section=SimulinkControl) | Step reference, `sum` with `+-`, continuous `pid` with P=800 and I=40, `transfer` 1/(1000s+50), Scope | The documented loop reduces to `10(1-exp(-0.8t))` for a 10 m/s step. Check representative samples and steady state. | Closed-loop feedback, PI, first-order plant |
| CTMS DC motor speed | [CTMS Motor Speed System Modeling](https://ctms.engin.umich.edu/CTMS/?example=MotorSpeed&section=SystemModeling) | Unit Step, `transfer` numerator 0.01 and denominator `[0.005, 0.06, 0.1001]`, Scope | Compare against the analytic two-pole step response derived from `K/((Js+b)(Ls+R)+K^2)` with J=0.01, b=0.1, K=0.01, R=1, L=0.5. | Continuous second-order response and DC gain |
| CTMS aircraft pitch | [CTMS Simulink Modeling](https://ctms.engin.umich.edu/CTMS/?example=AircraftPitch&section=SimulinkModeling) and [state-feedback design](https://ctms.engin.umich.edu/CTMS/index.php?example=AircraftPitch&section=ControlStateSpace) | 0.2-rad Step, precompensator gain 7.0711, `sum`, three-state `state_space` with identity output, `matrix_gain` K=[-0.6435,169.6950,7.0711], Demux, Scope | Integrate the published `xdot=(A-BK)x+B*Nbar*r` equations independently with RK4; check pitch samples, convergence to 0.2 rad, and closed-loop stability. | Continuous MIMO state-space, named states, matrix feedback |
| MathWorks first-order plus dead time | [Time Delays in Linear Systems](https://www.mathworks.com/help/control/ug/time-delays-in-linear-systems.html) | Unit Step, exact `delay` 2.1 s, `transfer` 1/(s+10), Scope | `y(t)=0` through the delay and `0.1*(1-exp(-10*(t-2.1)))` afterward on an aligned grid. | Exact continuous delay and delayed batch simulation |
| MathWorks Unit Delay | [Unit Delay](https://www.mathworks.com/help/simulink/slref/unitdelay.html) | Step or Constant, `unit_delay` with explicit sample time and authored initial condition, Scope | First sample equals the initial condition; every later sample equals the prior input sample. | Discrete execution, sample time, initial history |
| MathWorks delayed MIMO transfer | [Transport Delay in MIMO Transfer Function](https://www.mathworks.com/help/control/ug/time-delays-in-linear-systems.html) | Vector Constant, `mimo_transfer` with output-by-input delay matrix, Vector Scope | Reuse the versioned R2026a fixture and its independently derived shifted first-order responses for every named output. | Named 2x2 transfer matrix and four pairwise delays |

The University of Michigan [two-mass train Simulink workflow](https://ctms.engin.umich.edu/CTMS/index.php?example=Introduction&section=SimulinkControl)
is retained as a long-horizon browser regression. It was previously validated
with M1=1, M2=0.5, k=1, mu=0.02, g=9.8, a PI controller (P=0.05,
I=0.0075), a 150-second return step, and a 300-second run. This pass rechecks
that authored model and persistence behavior but does not count it as a new
numeric compatibility case.

## Mapping decisions

- CTMS examples are mapped from their published equations, not downloaded SLX
  files. Block placement and labels are Process Lab-native.
- The aircraft model exposes all three states from the State-Space block so the
  published full-state feedback gain can be applied. Only pitch is plotted.
- The delayed MIMO case reuses
  `internal/studio/testdata/simulink/r2026a/mimo_transfer_pairwise_delay.json`;
  duplicating that source or its expected vectors would create two authorities.
- The Unit Delay case uses Process Lab's explicit sample time. Simulink also
  supports inherited sample time (`-1`); Process Lab documents that difference
  in `simulink-r2026a-compatibility.md`.
- Source and mapping precision are bounded by double-precision arithmetic and
  the selected output grid. The CLI regression records tolerances alongside
  each oracle.

## CLI workflow

Each case is applied to a fresh project through the compiled CLI:

1. `project create` and `flow list --project` identify an isolated flow.
2. `flow apply --dry-run` validates the declarative document.
3. `flow apply`, `block list`, and `wire list` verify authored topology.
4. `analyze channels` verifies exposed named inputs and outputs.
5. `sim run` produces the response compared with the case oracle.

The block contract was checked from the running server with `block help` for
`source`, `sum`, `gain`, `matrix_gain`, `pid`, `transfer`, `state_space`,
`delay`, `unit_delay`, `mimo_transfer`, `scope`, and `vector_scope`.

## Results

All six numeric cases passed through the real HTTP-backed CLI. The first five
are table-driven by `cmd/processlab/public_examples_cli_test.go` and the JSON
fixtures in `cmd/processlab/testdata/public_examples`. The delayed MIMO case is
covered by `TestFlowsheetBuildingSkillRunsSimulinkR2026aMIMODelay` and the
versioned R2026a fixture.

| Case | Grid | Representative measured values | Absolute tolerance | Result |
|---|---:|---|---:|---|
| Cruise PI | 0.01 s to 10 s | y(1)=5.506710359, y(5)=9.816843611, y(10)=9.996645374 | 1e-9 | Pass |
| DC motor | 0.01 s to 3 s | y(0.1)=0.006855537, y(1)=0.083037111, y(3)=0.099592764 | 1e-10 | Pass |
| Aircraft pitch | 0.01 s to 10 s | theta(0.5)=0.101059541, theta(2)=0.204248597, theta(10)=0.199694534 | 1e-7 | Pass |
| First-order plus dead time | 0.01 s to 4 s | y(2.1)=0, y(2.2)=0.063212056, y(4)=0.099999999 | 1e-10 | Pass |
| Unit Delay | 0.1 s to 1 s | `[-1, 0, 0, 2, 2, 2, 2, 2, 2, 2, 2]` | 1e-12 | Pass |
| Delayed MIMO | 0.1 s to 1 s | temperature(1)=1.495807073, pressure(1)=2.726311523 | 1e-10 | Pass |

The fixture test also verifies dry-run/apply counts, persisted blocks and
wires, analysis-channel names, simulation driver selection, sample count, and
series identity. The aircraft oracle is an independent fixed-step RK4
integration at 1e-4 s; the remaining scalar cases use closed-form equations.

## Intentional limitations

- Exact delays must align with the requested run grid. A 2.1 s delay on a
  0.04 s grid is rejected with the nearest aligned delay and Padé/Thiran
  alternatives instead of silently changing the model.
- A Unit Delay sample time must be an integer multiple of the run grid. A
  0.1 s Unit Delay on a 0.06 s grid is rejected with aligned 0.06 s and 0.12 s
  suggestions.
- The validation covers equations and observable block behavior, not SLX
  import, variable-step solver parity, inherited Unit Delay timing, code
  generation, or the complete MATLAB/Simulink block catalog.

## Integrated browser evidence

- The delayed 2x2 MIMO run rendered two named trends together and separately.
  The separate plots retained independent y domains, shared a linked cursor,
  supported double-click isolation and Show all, and preserved the selected
  layout after reload.
- The same MIMO model rendered linked Bode magnitude and phase plots plus
  singular-value plots after CLI analysis. Both Bode plots reported the same
  inspected frequency.
- The cruise model rendered its five-block feedback topology, trend cursor,
  metrics, and Dynamics step/pole-zero plots; its canonical Dynamics URL
  survived reload.
- The CTMS two-mass train regression rendered seven blocks and seven signals,
  remained solved after a 300 s run, peaked at 0.968719, and decayed to
  0.023706 after the 150 s return step. The run, duration, trend, and metrics
  survived reload.
- Browser console warning/error logs were empty for the inspected pages. The
  persistent browser suite separately verifies mode history, HTMX replacement,
  linked inspection controls, comparison baselines, and multi-trend
  together/separate behavior.

## Reproduction

Run the fixture matrix and the existing public MIMO skill regression:

```sh
GOCACHE=/tmp/qlx-go-build-cache go test ./cmd/processlab -run '^TestPublicControlExamplesThroughCLI$' -count=1
GOCACHE=/tmp/qlx-go-build-cache go test ./cmd/processlab -run '^TestFlowsheetBuildingSkillRunsSimulinkR2026aMIMODelay$' -count=1
```

Run the complete automated gates:

```sh
GOCACHE=/tmp/qlx-go-build-cache go test ./...
node --test internal/web/static/js/*_test.js
node --test browser/*.test.mjs
git diff --check
```
