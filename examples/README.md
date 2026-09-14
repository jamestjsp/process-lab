# Cascade reactor example

`cascade-reactor.json` is an illustrative linear model with 21 blocks and 23
wires: an outer temperature PI loop, an inner flow PI loop, actuator and sensor
lags, a two-second exact recycle delay, and a heat-load disturbance at 80 seconds.
Four mirrored blocks carry feedback and recycle signals from right to left.
Values are normalized; this is a software demonstration, not a calibrated plant.

With Process Lab running, create a separate project and use its initial empty
flowsheet ID below:

```sh
processlab project create "Cascade reactor with recycle" --json
processlab flow list --project <project-id> --json
processlab flow apply --flow <flow-id> --dry-run --json < examples/cascade-reactor.json
processlab flow apply --flow <flow-id> --json < examples/cascade-reactor.json
processlab sim run --flow <flow-id> --duration 200 --sample-time 0.1 --json
```

The temperature setpoint changes to 1 at five seconds. The heat-load disturbance
changes to -0.2 at 80 seconds. The checked run produces finite values on all five
scope signals and a final reactor temperature of approximately 0.9995.
