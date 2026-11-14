// APPSTORE-TC-10 from the window side: after the source is removed the window
// reads an empty catalogue and shows no entry and no detail, so the entries a
// removed source offered do not outlive it on the surface
// (APPSTORE-AC-4, APPSTORE-C-24, APPSTORE-C-31).

import assert from "node:assert/strict";
import test from "node:test";

import { createAppCenter } from "../../../assets/app.js";
import {
  appCenterPage,
  fakeDocument,
  fakeElement,
  fakeFetch,
  fakeIntervals,
  fakeWindow,
  settle,
} from "../../shell/harness.js";

const consoleOrigin = "https://console.example.test";
const pagePath = consoleOrigin + "/api/v1/pages/app-center";

function address() {
  return (
    "https://app-center.kos.tsic.top/app-assets/kos-appstore/app-center/1.0.0/sha256:abc/assets/index.html" +
    "?kos-console=" + encodeURIComponent(consoleOrigin) + "&kos-window=w-1"
  );
}

function open(network) {
  const document = fakeDocument({
    status: fakeElement("p"),
    entries: fakeElement("ul"),
    "entries-empty": fakeElement("p"),
    detail: fakeElement("div"),
    install: fakeElement("div"),
  });
  const window = fakeWindow(address());
  const clock = fakeIntervals();
  createAppCenter({
    document,
    window,
    fetch: network.fetch,
    setInterval: clock.setInterval,
  }).start();
  window.deliver({
    origin: consoleOrigin,
    source: window.parent,
    data: { type: "init", token: "window-token", theme: "system", capabilities: [], expiresAt: "2026-11-12T15:00:00Z" },
  });
  return { document, window };
}

test("来源移除后窗口不残留条目，空目录给出空状态", async () => {
  const network = fakeFetch({ ["GET " + pagePath]: { status: 200, body: appCenterPage([]) } });
  const { document } = open(network);
  await settle();

  assert.equal(document.getElementById("entries").children.length, 0);
  assert.equal(document.getElementById("entries-empty").hidden, false);
  assert.match(document.getElementById("status").textContent, /0 个条目/);
  assert.match(document.getElementById("detail").textContent, /选择一个条目/);
  // No entry survives the removal: the window renders no shipped application.
  assert.doesNotMatch(document.getElementById("detail").textContent, /Redis|sha256:/);
});
