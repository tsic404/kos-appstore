// APPSTORE-TC-166 from the window side: the preflight reports a data format
// change as an irreversible item, so the upgrade is refused with the reason
// naming that item — and the surface offers no confirmation entry that would
// bypass the preflight (APPSTORE-AC-77, APPSTORE-C-79, KOSD-C-34).

import assert from "node:assert/strict";
import test from "node:test";

import { createAppCenter } from "../../../assets/app.js";
import {
  appCenterPage,
  buttonWith,
  elementsOf,
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

// detail is the reason the framework refuses the upgrade.
const detail = "数据格式由 v1 变为 v2，需要在本产品之外完成迁移";

// site carries the refused preflight: the target declares a data format change
// and the phase stayed on the installed version.
const site = {
  namespace: "default",
  name: "site",
  definition: "web",
  phase: "Ready",
  health: "Healthy",
  conditions: [
    { type: "IrreversibleUpgrade", status: "True", reason: "Refused", message: "dataFormat" },
    { type: "Reconciled", status: "True" },
  ],
  update: {
    available: true,
    version: "2.0.0",
    digest: "sha256:2222",
    installedDigest: "sha256:1111",
    preview: {
      target: "sha256:2222",
      configChanges: [],
      renderChanges: [],
      irreversible: [{ kind: "dataFormat", detail }],
      migrations: [],
      rolledBack: false,
    },
  },
};

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

async function open() {
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
    ["GET " + pagePath]: { status: 200, body: appCenterPage([web], undefined, [site]) },
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

test("不可逆项拒绝原因指向该项本身，界面没有绕过预检的确认入口", async () => {
  const { document, network } = await open();
  document.getElementById("installed").children[0].children[0].click();
  const panel = document.getElementById("application");

  // The refusal names the irreversible item itself (KOSD-C-34).
  assert.match(panel.textContent, /升级被拒绝/);
  assert.match(panel.textContent, /dataFormat/);
  assert.match(panel.textContent, new RegExp(detail));

  // The confirmation entry is unavailable, and no other control submits the
  // upgrade: the preflight cannot be bypassed from this surface.
  for (const label of ["升级", "确认", "强制", "绕过"]) {
    assert.equal(buttonWith(panel, label), null, "the surface must not offer " + label);
  }
  for (const button of elementsOf(panel, "button")) {
    button.click();
  }
  await settle();
  assert.deepEqual(network.calls.map((call) => call.method + " " + call.url), ["GET " + pagePath]);

  // The phase and the downstream version are the application object's: the
  // refusal changed neither.
  assert.equal(rowValue(panel, "阶段"), "Ready");
  assert.match(panel.textContent, /IrreversibleUpgrade: True/);
});
