# Process Lab

Build block diagrams, simulate dynamic systems, and design controllers in your
browser. Use scalar or multichannel signals, continuous or discrete models, and
feedback loops. Inspect plots and save the current view as PNG.

Process Lab runs as one Go executable. HTML, CSS, JavaScript, and HTMX are
embedded. Projects are saved in SQLite. No Node runtime or CDN is required.

## Run

Install Go 1.27.1 or later, then run:

```sh
go run ./cmd/processlab serve
```

Open [localhost:8080](http://127.0.0.1:8080) in a current browser.
The first run creates `processlab.db` and an example project.

For Docker, run `docker compose up --build --detach`.

## Guides

- [CLI commands](docs/cli.md)
- [Workbench controls](docs/workbench-ergonomics.md)
- [Cascade reactor example](examples/README.md)
- [PID design](docs/pid-design.md) and [controller tuning](docs/generalized-tuning.md)
- [Model compatibility](docs/simulink-r2026a-compatibility.md)

## Tests

```sh
go test ./...
npm ci
node --test internal/web/static/js/*_test.js browser/*.test.mjs
```

Browser tests require Google Chrome. Rebuild the executable after code or web
asset changes.
