// APPSTORE-TC-168 from the window side: after the catalogue sync the first page
// read carries the update available and its target version — no action, no
// "check for updates" entry. An application on the offered package carries
// neither (APPSTORE-AC-78, APPSTORE-C-80, KOSD-C-32, KOSD-C-13).

import assert from "node:assert/strict";
import test from "node:test";

import { createAppCenter } from "../../../assets/app.js";
import {
  appCenterPage,
  buttonWith,
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

const web = {
  source: "kos-appstore",
  id: "web",
  name: "Web",
  version: "2.0.0",
  digest: "sha256:2222",
  definition: "web",
  installable: true,
  dependencies: [],
  permissions: [],
  config: [],
};

// behind is installed from the previous package, so the offered 2.0.0 is an
// update; current is installed from the package the catalogue offers now.
const behind = {
  namespace: "default",
  name: "behind",
  definition: "web",
  phase: "Ready",
  health: "Healthy",
  conditions: [],
  update: { available: true, version: "2.0.0", digest: "sha256:2222", installedDigest: "sha256:1111" },
};
const current = {
  namespace: "default",
  name: "current",
  definition: "web",
  phase: "Ready",
  health: "Healthy",
  conditions: [],
  update: { available: false, installedDigest: "sha256:2222" },
};

async function open(installed) {
  const document = fakeDocument({
    status: fakeElement("p"),
    entries: fakeElement("ul"),
    "entries-empty": fakeElement("p"),
    installed: fakeElement("ul"),
    "installed-empty": fakeElement("p"),
    detail: fakeElement("div"),
    application: fakeElement("div"),
    install: fakeElement("div"),
  });
  const window = fakeWindow(address());
  const network = fakeFetch({
    ["GET " + pagePath]: { status: 200, body: appCenterPage([web], undefined, installed) },
  });
  createAppCenter({ document, window, fetch: network.fetch, setInterval: fakeIntervals().setInterval }).start();
  window.deliver({
    origin: consoleOrigin,
    source: window.parent,
    data: { type: "init", token: "window-token", theme: "dark", capabilities: [], expiresAt: "2026-11-13T15:00:00Z" },
  });
  await settle();
  return { document, network };
}

test("更新可用随页面读取出现，没有更高版本时不出现且没有检查更新入口", async () => {
  const { document, network } = await open([behind, current]);
  const list = document.getElementById("installed");

  // The first read already carries the update notice and the target version:
  // nothing was clicked and nothing was checked (APPSTORE-C-80).
  const behindRow = list.children[0].textContent;
  assert.match(behindRow, /更新可用/);
  assert.match(behindRow, /2\.0\.0/);
  // The control: an application on the offered package shows no notice.
  const currentRow = list.children[1].textContent;
  assert.doesNotMatch(currentRow, /更新可用/);
  assert.doesNotMatch(currentRow, /2\.0\.0/);

  // The page read is the only call, and no entry asks to check, refresh or sync
  // the catalogue (APPSTORE-C-80).
  assert.equal(network.calls.length, 1);
  assert.equal(network.calls[0].url, pagePath);
  for (const label of ["检查更新", "刷新", "同步"]) {
    assert.equal(buttonWith(document.documentElement, label), null, "the surface must not offer " + label);
  }

  // The detail repeats the same fact and still offers no check entry.
  list.children[1].children[0].click();
  const panel = document.getElementById("application");
  assert.match(panel.textContent, /无可用更新/);
  assert.equal(buttonWith(panel, "检查更新"), null);
});
