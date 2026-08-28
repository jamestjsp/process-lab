import assert from "node:assert/strict";
import test, {after, before, describe} from "node:test";

import {chromium} from "playwright-core";

import {chromePath, startServer} from "./server.mjs";

let server;
let browser;

before(async () => {
  server = await startServer();
  browser = await chromium.launch({executablePath: chromePath, headless: true});
}, {timeout: 120_000});

after(async () => {
  await browser?.close();
  await server?.stop();
});

async function openWorkbench() {
  const page = await browser.newPage();
  const problems = [];
  page.on("pageerror", (error) => problems.push(`pageerror: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") problems.push(`console: ${message.text()}`);
  });
  await page.goto(`${server.url}/`);
  await page.click(".project-open");
  await page.waitForSelector('[data-workbench-mode="simulation"]');
  return {page, problems};
}

async function chooseMode(page, mode) {
  await page.locator(`.workspace-modes a[href*="view=${mode}"]`).click();
  await page.waitForSelector(`[data-workbench-mode="${mode}"]`);
}

async function installMultiTrendFlow(page) {
  const document = {
    version: 1,
    blocks: [
      {kind: "source", name: "Setpoint", position: {x: 80, y: 80}, parameters: {
        amplitude: "2", initial_value: "0", step_time: "0.5",
      }},
      {kind: "sine", name: "Oscillation", position: {x: 80, y: 260}, parameters: {
        amplitude: "0.8", bias: "0.5", frequency: "1.2", phase: "0",
      }},
      {kind: "constant", name: "Load", position: {x: 80, y: 440}, parameters: {value: "-1"}},
      {kind: "mux", name: "Signals", position: {x: 380, y: 260}, parameters: {
        output_names: "setpoint, oscillation, load",
      }},
      {kind: "vector_scope", name: "Trend viewer", position: {x: 700, y: 260}, parameters: {
        input_names: "setpoint, oscillation, load",
      }},
    ],
    wires: [
      {source: "Setpoint", sourcePort: 0, target: "Signals", targetPort: 0},
      {source: "Oscillation", sourcePort: 0, target: "Signals", targetPort: 1},
      {source: "Load", sourcePort: 0, target: "Signals", targetPort: 2},
      {source: "Signals", sourcePort: 0, target: "Trend viewer", targetPort: 0},
    ],
  };
  const applied = await page.request.put(`${server.url}/api/v1/flows/1/document`, {data: document});
  assert.equal(applied.status(), 200, await applied.text());
  const simulated = await page.request.post(`${server.url}/api/v1/flows/1/simulations`, {
    data: {duration: 2, sampleTime: 0.1},
  });
  assert.equal(simulated.status(), 201, await simulated.text());
  await page.goto(`${server.url}/projects/1/flows/1?view=simulation`);
  await page.evaluate(() => localStorage.clear());
  await page.reload();
}

describe("engineering workspace", () => {
  test("mode URLs survive reload, back, and forward while Design stays reachable", async () => {
    const {page, problems} = await openWorkbench();
    try {
      assert.equal(new URL(page.url()).searchParams.get("view"), "simulation");
      await chooseMode(page, "frequency");
      assert.equal(new URL(page.url()).searchParams.get("view"), "frequency");

      await chooseMode(page, "design");
      assert.equal(await page.locator("#pid-controller-design-form").count(), 1);
      assert.equal(await page.locator("#state-controller-design-form").count(), 1);
      assert.equal(await page.locator("#robust-controller-design-form").count(), 1);

      await page.goBack();
      await page.waitForSelector('[data-workbench-mode="frequency"]');
      assert.equal(new URL(page.url()).searchParams.get("view"), "frequency");
      await page.goForward();
      await page.waitForSelector('[data-workbench-mode="design"]');
      await page.reload();
      await page.waitForSelector('[data-workbench-mode="design"]');
      assert.equal(await page.locator("#controller-design-heading").count(), 1);
      assert.deepEqual(problems, []);
    } finally {
      await page.close();
    }
  });

  test("inspection and plot controls work after HTMX mode replacement", async () => {
    const {page, problems} = await openWorkbench();
    try {
      await page.locator('#run-form input[name="duration"]').fill("2");
      await page.locator('#run-form input[name="sample_time"]').fill("0.1");
      await page.locator("#run-form .run-button").click();
      const plot = page.locator('[data-plot-id="simulation-trend"]');
      await plot.waitFor();

      await chooseMode(page, "design");
      await chooseMode(page, "simulation");
      await plot.waitFor();

      await plot.focus();
      await page.keyboard.press("ArrowRight");
      await assert.doesNotReject(async () => {
        await page.waitForFunction(() =>
          !document.querySelector('[data-plot-id="simulation-trend"] [data-chart-readout]')
            ?.textContent.includes("Focus the plot"));
      });

      const seriesButton = page.locator("[data-trend-workspace] [data-series-toggle]").first();
      const seriesKey = await seriesButton.getAttribute("data-series-toggle");
      await seriesButton.click();
      assert.equal(await plot.locator(`[data-series-path="${seriesKey}"]`).getAttribute("hidden"), "");
      await seriesButton.click();
      assert.equal(await plot.locator(`[data-series-path="${seriesKey}"]`).getAttribute("hidden"), null);

      const svg = plot.locator("svg");
      const originalViewBox = await svg.getAttribute("viewBox");
      await plot.locator("[data-chart-zoom-in]").click();
      assert.notEqual(await svg.getAttribute("viewBox"), originalViewBox);
      await plot.locator("[data-chart-characteristics]").click();
      assert.equal(await plot.locator("[data-chart-characteristic-lines]").getAttribute("hidden"), "");
      await plot.locator("[data-chart-reset]").click();
      assert.equal(await svg.getAttribute("viewBox"), originalViewBox);
      assert.deepEqual(problems, []);
    } finally {
      await page.close();
    }
  });

  test("controller validation swaps a semantic 400 into its result region", async () => {
    const {page, problems} = await openWorkbench();
    try {
      await chooseMode(page, "design");
      const form = page.locator("#state-controller-design-form");
      await form.locator('textarea[name="q"]').fill("not a matrix");
      const responsePromise = page.waitForResponse((response) =>
        response.url().includes("/controller-candidates/state-space"));
      await form.locator("button[type=submit]").click();
      const response = await responsePromise;

      assert.equal(response.status(), 400);
      assert.equal(response.request().headers()["hx-request-type"], "partial");
      await page.locator("#controller-candidate .controller-candidate-error").waitFor();
      assert.match(
        await page.locator("#controller-candidate").textContent(),
        /Q:|matrix/i,
      );
      assert.equal(await page.locator("#workbench").count(), 1);
      assert.equal(await page.locator("#state-controller-design-form").count(), 1);
      assert.equal(problems.length, 1);
      assert.match(problems[0], /console: .*status of 400 \(Bad Request\)$/);
    } finally {
      await page.close();
    }
  });

  test("dynamics and frequency modes render inspectable engineering axes", async () => {
    const {page, problems} = await openWorkbench();
    try {
      await page.locator('#run-form input[name="duration"]').fill("2");
      await page.locator('#run-form input[name="sample_time"]').fill("0.1");
      await page.locator("#run-form .run-button").click();
      await page.waitForSelector('[data-plot-id="simulation-trend"]');
      assert.ok(await page.locator('.run-history a[href$=".csv"]').count() >= 1);

      await chooseMode(page, "dynamics");
      await page.locator('#analysis-form input[name="analysis_horizon"]').fill("4");
      await page.locator("#analysis-form .analysis-run-button").click();
      const step = page.locator('[data-plot-id="analysis-dynamics-step"]');
      await step.waitFor();
      assert.ok(await step.locator(".chart-label").count() >= 4);
      assert.ok(await step.locator(".chart-grid").count() >= 4);
      assert.equal(await step.locator(".chart-label").first().isVisible(), true);

      await chooseMode(page, "frequency");
      await page.locator('#analysis-form input[name="analysis_points"]').fill("40");
      await page.locator("#analysis-form .analysis-run-button").click();
      const magnitude = page.locator('[data-plot-id="analysis-frequency-bode-magnitude"]');
      const phase = page.locator('[data-plot-id="analysis-frequency-bode-phase"]');
      await magnitude.waitFor();
      await phase.waitFor();
      assert.equal(await magnitude.getAttribute("data-x-scale"), "log10");

      const decadeLabels = await magnitude.locator(".x-label").allTextContents();
      const decadeValues = decadeLabels.map(Number).filter(Number.isFinite);
      assert.ok(decadeValues.length >= 3, `Bode tick labels: ${decadeLabels.join(", ")}`);
      for (const value of decadeValues) {
        assert.ok(value > 0, `non-positive log tick: ${value}`);
        assert.ok(
          Math.abs(Math.log10(value) - Math.round(Math.log10(value))) < 1e-9,
          `non-decade Bode tick: ${value}`,
        );
      }

      const lowestYLabelBox = await magnitude.locator(".y-label").first().boundingBox();
      const firstXLabelBox = await magnitude.locator(".x-label").first().boundingBox();
      assert.ok(lowestYLabelBox);
      assert.ok(firstXLabelBox);
      assert.ok(
        lowestYLabelBox.x + lowestYLabelBox.width <= firstXLabelBox.x ||
          firstXLabelBox.x + firstXLabelBox.width <= lowestYLabelBox.x ||
          lowestYLabelBox.y + lowestYLabelBox.height <= firstXLabelBox.y ||
          firstXLabelBox.y + firstXLabelBox.height <= lowestYLabelBox.y,
        `Bode axis labels overlap: ${JSON.stringify({lowestYLabelBox, firstXLabelBox})}`,
      );

      await magnitude.focus();
      await page.keyboard.press("ArrowRight");
      await page.waitForFunction(() =>
        document.querySelectorAll(
          '[data-plot-group="analysis-frequency-bode"] [data-chart-cursor]:not([hidden])',
        ).length === 2,
      );
      assert.doesNotMatch(
        await phase.locator("[data-chart-readout]").textContent(),
        /Focus the plot/,
      );
      assert.deepEqual(problems, []);
    } finally {
      await page.close();
    }
  });

  test("an older run becomes a reloadable comparison baseline", async () => {
    const {page, problems} = await openWorkbench();
    try {
      for (const duration of ["1", "2"]) {
        await page.locator('#run-form input[name="duration"]').fill(duration);
        await page.locator('#run-form input[name="sample_time"]').fill("0.1");
        await page.locator("#run-form .run-button").click();
        await page.waitForSelector('[data-plot-id="simulation-trend"]');
      }
      const baselineLinks = page.locator(".run-history-actions a", {hasText: "Use as baseline"});
      assert.ok(await baselineLinks.count());
      assert.equal(await page.locator(".run-history li").first().locator("a", {hasText: "Use as baseline"}).count(), 0);
      await baselineLinks.first().click();
      await page.waitForSelector('[data-workbench-mode="compare"]');
      assert.equal(new URL(page.url()).searchParams.get("view"), "compare");
      assert.ok(new URL(page.url()).searchParams.get("baseline"));
      assert.equal(await page.locator('[data-plot-id="simulation-compare-overlay"]').count(), 1);
      assert.equal(await page.locator('[data-plot-id="simulation-compare-difference"]').count(), 1);
      assert.ok(await page.locator('path[stroke-dasharray="6 4"]').count());

      await page.reload();
      await page.waitForSelector('[data-plot-id="simulation-compare-difference"]');
      assert.equal(new URL(page.url()).searchParams.get("view"), "compare");
      assert.deepEqual(problems, []);
    } finally {
      await page.close();
    }
  });

  test("multi-trend signals switch between linked separate and together views", async () => {
    const {page, problems} = await openWorkbench();
    try {
      await installMultiTrendFlow(page);
      const workspace = page.locator("[data-trend-workspace]");
      await workspace.waitFor();
      const overlayPanel = workspace.locator('[data-trend-layout-panel="overlay"]');
      const splitPanel = workspace.locator('[data-trend-layout-panel="split"]');
      const together = workspace.getByRole("button", {name: "Together", exact: true});
      const separate = workspace.getByRole("button", {name: "Separate", exact: true});

      assert.equal(await overlayPanel.locator("[data-series-path]").count(), 3);
      assert.equal(await together.getAttribute("aria-pressed"), "true");
      assert.equal(await overlayPanel.getAttribute("hidden"), null);
      assert.equal(await splitPanel.getAttribute("hidden"), "");

      await separate.click();
      assert.equal(await separate.getAttribute("aria-pressed"), "true");
      assert.equal(await overlayPanel.getAttribute("hidden"), "");
      assert.equal(await splitPanel.getAttribute("hidden"), null);
      assert.equal(await splitPanel.locator("[data-series-panel]:not([hidden])").count(), 3);

      const splitPlots = splitPanel.locator("[data-series-panel]");
      await splitPlots.first().focus();
      await page.keyboard.press("ArrowRight");
      await page.waitForFunction(() =>
        document.querySelectorAll(
          '[data-trend-layout-panel="split"] [data-chart-cursor]:not([hidden])',
        ).length === 3,
      );

      const signals = workspace.locator("[data-series-toggle]");
      await signals.nth(1).dblclick();
      assert.equal(await splitPanel.locator("[data-series-panel]:not([hidden])").count(), 1);
      assert.deepEqual(await signals.evaluateAll((buttons) =>
        buttons.map((button) => button.getAttribute("aria-pressed"))), ["false", "true", "false"]);

      await workspace.getByRole("button", {name: "Show all", exact: true}).click();
      assert.equal(await splitPanel.locator("[data-series-panel]:not([hidden])").count(), 3);
      assert.deepEqual(await signals.evaluateAll((buttons) =>
        buttons.map((button) => button.getAttribute("aria-pressed"))), ["true", "true", "true"]);

      await together.click();
      assert.equal(await overlayPanel.getAttribute("hidden"), null);
      assert.equal(await splitPanel.getAttribute("hidden"), "");
      await separate.click();
      await page.reload();
      await page.waitForSelector('[data-trend-layout-panel="split"]:not([hidden])');
      assert.equal(await page.getByRole("button", {name: "Separate", exact: true}).getAttribute("aria-pressed"), "true");
      assert.deepEqual(problems, []);
    } finally {
      await page.close();
    }
  });
});
