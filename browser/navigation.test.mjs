// The interaction gate: swap, history, and restore behaviour that no handler
// test can reach. The tab strip swaps a fragment while pushing the canonical
// document URL, so the failure this guards is a back button that leaves the
// page without its swap target — after which every later tab click silently
// does nothing.

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

async function openWorkbenchWithTwoSheets() {
  const page = await browser.newPage();
  const problems = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      problems.push(`console: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => problems.push(`pageerror: ${error.message}`));
  page.on("requestfailed", (request) =>
    problems.push(`requestfailed: ${request.url()}`));
  page.on("response", (response) => {
    if (response.status() >= 400) {
      problems.push(`http ${response.status()}: ${response.url()}`);
    }
  });

  await page.goto(`${server.url}/`);
  await page.click(".project-open");
  await page.waitForSelector("#workbench");

  // The tab strip needs a second sheet to switch between. This "+" is a real
  // form, so it also proves the no-JavaScript create path still redirects.
  if (await page.locator(".flow-tab").count() < 2) {
    await page.click(".tab-add");
    await page.waitForFunction(
      () => document.querySelectorAll(".flow-tab").length >= 2,
    );
  }
  return {page, problems};
}

describe("tab navigation", () => {
  test("back after a swap restores a working page", async () => {
    const {page, problems} = await openWorkbenchWithTwoSheets();
    try {
      assert.equal(await page.evaluate(() => htmx.version), "4.0.0");
      const firstURL = page.url();
      const firstTitle = await page.title();
      const originalWorkbench = await page.locator("#workbench").elementHandle();
      const originalCanvas = await page.locator("#flow-canvas").elementHandle();

      const otherTab = page.locator(".flow-tab:not([aria-current])").first();
      const otherFlowID = await otherTab.getAttribute("data-flow-tab");
      assert.equal(await otherTab.getAttribute("hx-swap"), "outerMorph");
      const partialRequest = page.waitForRequest((request) =>
        request.headers()["hx-request-type"] === "partial");
      await otherTab.click();
      const partialHeaders = (await partialRequest).headers();
      assert.equal(partialHeaders["hx-request"], "true");
      assert.equal(partialHeaders["hx-request-type"], "partial");
      assert.equal(partialHeaders.accept, "text/html");
      await page.waitForFunction(
        (id) => document.querySelector("#workbench")?.dataset.flowId === id,
        otherFlowID,
      );
      assert.equal(
        await page.evaluate((node) => node === document.querySelector("#workbench"), originalWorkbench),
        true,
        "morph navigation replaced the stable workbench node",
      );
      assert.equal(
        await page.evaluate((node) => node === document.querySelector("#flow-canvas"), originalCanvas),
        true,
        "morph navigation replaced the stable canvas node",
      );

      const secondURL = page.url();
      assert.notEqual(secondURL, firstURL, "swap did not push a new URL");
      assert.notEqual(
        await page.title(), firstTitle,
        "the document title did not move with the pushed URL",
      );

      await page.goBack();
      await page.waitForFunction(
        (url) => location.pathname + location.search === url,
        new URL(firstURL).pathname + new URL(firstURL).search,
      );

      const restored = await page.evaluate(() => ({
        workbench: Boolean(document.querySelector("#workbench")),
        tabs: document.querySelectorAll(".flow-tab").length,
        topbar: Boolean(document.querySelector(".topbar")),
      }));
      assert.ok(restored.workbench, "back navigation lost the #workbench target");
      assert.ok(restored.topbar, "back navigation lost the page chrome");
      assert.ok(restored.tabs >= 2, "back navigation lost the tab strip");
      assert.equal(
        await page.title(), firstTitle,
        "the restored page kept the title of the sheet it navigated away from",
      );

      // The real regression: the swap target has to still work afterwards.
      await page.locator(".flow-tab:not([aria-current])").first().click();
      await page.waitForFunction(
        (id) => document.querySelector("#workbench")?.dataset.flowId === id,
        otherFlowID,
      );

      assert.deepEqual(problems, [], "browser reported failures");
    } finally {
      await page.close();
    }
  });

  test("bounded edits route morphing partials without replacing the workbench", async () => {
    const {page, problems} = await openWorkbenchWithTwoSheets();
    try {
      await page.locator(".block-body").first().click();
      await page.locator("form.property-form").waitFor();

      const selectors = [
        "#workbench",
        "#flow-canvas",
        ".block-card.selected",
        "#inspector-rail",
        "#simulation-results",
        "#flow-tabs",
        "#project-facts",
      ];
      const originals = await Promise.all(
        selectors.map((selector) => page.locator(selector).elementHandle()),
      );
      const form = page.locator("form.property-form");
      const name = form.locator('input[name="name"]');
      const updatedName = `Morph ${Date.now()}`;
      await name.fill(updatedName);
      const responsePromise = page.waitForResponse((response) =>
        response.request().method() === "PUT" && /\/blocks\/\d+/.test(response.url()));
      await form.locator('button[type="submit"]').click();
      const response = await responsePromise;
      assert.equal(response.status(), 200);
      const body = await response.text();
      assert.equal((body.match(/<hx-partial\b/g) || []).length, 5);
      assert.equal(
        (body.match(/<hx-partial\b[^>]*hx-swap="outerMorph"/g) || []).length,
        5,
      );
      assert.doesNotMatch(body, /hx-swap-oob/);
      await page.waitForFunction(
        (value) => document.querySelector(".block-card.selected strong")?.textContent === value,
        updatedName,
      );

      for (let index = 0; index < selectors.length; index += 1) {
        assert.equal(
          await page.evaluate(
            ([node, selector]) => node === document.querySelector(selector),
            [originals[index], selectors[index]],
          ),
          true,
          `bounded partial replaced ${selectors[index]}`,
        );
      }
      assert.deepEqual(problems, [], "browser reported failures");
    } finally {
      await page.close();
    }
  });

  test("history restore through the server rebuilds the page", async () => {
    const {page, problems} = await openWorkbenchWithTwoSheets();
    try {
      const firstURL = page.url();
      const otherTab = page.locator(".flow-tab:not([aria-current])").first();
      const otherFlowID = await otherTab.getAttribute("data-flow-tab");
      await otherTab.click();
      await page.waitForFunction(
        (id) => document.querySelector("#workbench")?.dataset.flowId === id,
        otherFlowID,
      );

      assert.equal(
        await page.evaluate(() => sessionStorage.getItem("htmx-history-cache")),
        null,
        "htmx 4 unexpectedly wrote a history snapshot cache",
      );

      // Assert the contract, not just the visible result. Here the workbench
      // fragment happens to be the whole body — the topbar lives inside
      // #workbench — so answering a restore with a fragment would still leave
      // a page that looks and works about right, and a symptom-only check
      // would wave it through. What must hold is that the restore was
      // answered with a complete document.
      // Collected as promises registered synchronously in the listener, so the
      // assertion below cannot run before the bodies have been read.
      const restores = [];
      page.on("response", (response) => {
        if (response.request().headers()["hx-history-restore-request"]) {
          restores.push(
            response.text().then((body) => ({
              url: response.url(),
              body,
              headers: response.request().headers(),
            })),
          );
        }
      });

      await page.goBack();
      await page.waitForFunction(
        (url) => location.pathname + location.search === url,
        new URL(firstURL).pathname + new URL(firstURL).search,
      );

      const restoredResponses = await Promise.all(restores);
      assert.equal(restoredResponses.length, 1, "the server restore path was not used");
      assert.equal(
        restoredResponses[0].headers["hx-history-restore-request"],
        "true",
      );
      assert.equal(restoredResponses[0].headers["hx-request-type"], "full");
      assert.match(
        restoredResponses[0].body.trimStart().slice(0, 40).toLowerCase(),
        /^<!doctype html>/,
        "the history restore was answered with a fragment instead of a document",
      );
      const restored = await page.evaluate(() => ({
        workbench: Boolean(document.querySelector("#workbench")),
        topbar: Boolean(document.querySelector(".topbar")),
        stylesheets: document.querySelectorAll('link[rel="stylesheet"]').length,
      }));
      assert.ok(restored.workbench, "server restore lost the #workbench target");
      assert.ok(restored.topbar, "server restore lost the page chrome");
      assert.ok(restored.stylesheets > 0, "server restore lost the stylesheets");

      await page.locator(".flow-tab:not([aria-current])").first().click();
      await page.waitForFunction(
        (id) => document.querySelector("#workbench")?.dataset.flowId === id,
        otherFlowID,
      );

      assert.deepEqual(problems, [], "browser reported failures");
    } finally {
      await page.close();
    }
  });
});
