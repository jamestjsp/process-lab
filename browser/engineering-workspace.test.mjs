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

      const seriesButton = plot.locator("[data-series-toggle]").first();
      const seriesKey = await seriesButton.getAttribute("data-series-toggle");
      await seriesButton.click();
      assert.equal(await plot.locator(`[data-series-path="${seriesKey}"]`).getAttribute("hidden"), "");

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
});
