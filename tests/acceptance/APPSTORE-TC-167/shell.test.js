// APPSTORE-TC-167 from the window side: the phase and the conditions the window
// shows are the application object's own reading at page-read time. During the
// upgrade, after it settles and after an automatic rollback, the panel repeats
// the served values — it keeps no progress state of its own and fabricates none
// (APPSTORE-AC-77, APPSTORE-C-79, KOSD-C-33, KOSD-C-34).

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
  rowValue,
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

// site builds one served application with the phase and conditions the object
// carries at that read.
function site(phase, conditions, preview) {
  return {
    namespace: "default",
    name: "site",
    definition: "web",
    phase,
    health: "Healthy",
    conditions,
    update: preview
      ? { available: true, version: "3.0.0", digest: "sha256:3333", installedDigest: "sha256:2222", preview }
      : { available: false },
  };
}

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
  const clock = fakeIntervals();
  const network = fakeFetch({
    ["GET " + pagePath]: { status: 200, body: appCenterPage([web], undefined, [installed]) },
  });
  createAppCenter({ document, window, fetch: network.fetch, setInterval: clock.setInterval }).start();
  window.deliver({
    origin: consoleOrigin,
    source: window.parent,
    data: { type: "init", token: "window-token", theme: "dark", capabilities: [], expiresAt: "2026-11-13T15:00:00Z" },
  });
  await settle();
  document.getElementById("installed").children[0].children[0].click();
  return { document, clock, network };
}

test("升级中与结束后的阶段和条件都是应用对象的那一份", async () => {
  // The preflight round: the object is Upgrading and the downstream object still
  // renders the installed version.
  const upgrading = await open(
    site("Upgrading", [
      { type: "Reconciled", status: "False", reason: "Upgrading", message: "升级预检进行中" },
    ]),
  );
  const during = upgrading.document.getElementById("application");
  assert.equal(rowValue(during, "阶段"), "Upgrading");
  assert.match(during.textContent, /Reconciled: False/);
  assert.match(during.textContent, /升级预检进行中/);
  assert.doesNotMatch(during.textContent, /\d+ ?%/);
  assert.doesNotMatch(during.textContent, /进度 [0-9]/);
  // The window arms no timer of its own for progress: the liveness report is
  // the only interval, and nothing re-reads the page behind the operator.
  assert.equal(upgrading.clock.active().length, 1);
  assert.equal(upgrading.network.calls.length, 1);

  // The apply round finished: the object is Ready and reconciled.
  const settled = await open(site("Ready", [{ type: "Reconciled", status: "True", reason: "Reconciled" }]));
  const after = settled.document.getElementById("application");
  assert.equal(rowValue(after, "阶段"), "Ready");
  assert.match(after.textContent, /Reconciled: True/);

  // The upgrade hook failed and the framework rolled the target back: the panel
  // repeats the result the object recorded, including the reverted preview.
  const rolled = await open(
    site(
      "Ready",
      [
        { type: "UpgradeFailed", status: "True", reason: "HookFailed", message: "升级钩子失败" },
        { type: "Reconciled", status: "True" },
      ],
      {
        target: "sha256:3333",
        configChanges: [],
        renderChanges: [],
        irreversible: [],
        migrations: [],
        rolledBack: true,
      },
    ),
  );
  const reverted = rolled.document.getElementById("application");
  assert.equal(rowValue(reverted, "阶段"), "Ready");
  assert.match(reverted.textContent, /UpgradeFailed: True/);
  assert.match(reverted.textContent, /升级钩子失败/);
  assert.equal(rowValue(reverted, "已回滚"), "是");
});
