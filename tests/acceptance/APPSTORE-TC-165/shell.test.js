// APPSTORE-TC-165 from the window side: the page read gives the target version
// with no user action, and the upgrade panel presents the framework's preflight
// report — the target digest beside the applied one and the config difference,
// including a changed default. Before the upgrade is confirmed the phase the
// window shows is still the application object's, and the panel does not claim
// the target as applied (APPSTORE-AC-77, APPSTORE-C-79, KOSD-C-32, KOSD-C-34).

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
  elementsWithClass,
  rowValue,
  settle,
} from "../../shell/harness.js";

const consoleOrigin = "https://console.example.test";
const pagePath = consoleOrigin + "/api/v1/pages/app-center";
const applicationPath = consoleOrigin + "/api/v1/resources/Application/site?namespace=default";

function address() {
  return (
    "https://app-center.kos.tsic.top/app-assets/kos-appstore/app-center/1.0.0/sha256:abc/assets/index.html" +
    "?kos-console=" + encodeURIComponent(consoleOrigin) + "&kos-window=w-1"
  );
}

// web declares the config field whose default the offered version changes.
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
  config: [{ name: "mode", type: "string", control: "text", default: "two" }],
};

// site is the installed application; the catalogue offers 2.0.0 for the
// installed 1.0.0 package.
const site = {
  namespace: "default",
  name: "site",
  definition: "web",
  phase: "Ready",
  health: "Healthy",
  conditions: [{ type: "Reconciled", status: "True" }],
  update: {
    available: true,
    version: "2.0.0",
    digest: "sha256:2222",
    installedDigest: "sha256:1111",
    preview: {
      target: "sha256:2222",
      specDigest: "sha256:spec",
      renderDigest: "sha256:render",
      configChanges: [{ field: "mode", from: '"one"', to: '"two"' }],
      renderChanges: [
        {
          object: { group: "", kind: "ConfigMap", namespace: "apps", name: "site" },
          operation: "update",
          fields: ["data.mode"],
        },
      ],
      irreversible: [],
      migrations: [],
      rolledBack: false,
    },
  },
};

// stored is the application projection the upgrade write is built from: the
// write carries the version the API checks against.
const stored = {
  object: {
    apiVersion: "kos.tsic.top/v1beta1",
    kind: "Application",
    metadata: { name: "site", namespace: "default", resourceVersion: "7" },
    spec: { definitionRef: "web", namespace: "default" },
  },
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
    ["GET " + applicationPath]: { status: 200, body: stored },
    ["PUT " + applicationPath]: { status: 202, body: { accepted: true } },
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

test("页面给出目标版本与框架预检，确认前阶段与已应用版本不变", async () => {
  const { document, network } = await open();

  // The update available and its target version appear with the page read, with
  // no action of any kind (APPSTORE-C-80).
  const list = document.getElementById("installed");
  assert.match(list.textContent, /更新可用/);
  assert.match(list.textContent, /2\.0\.0/);

  list.children[0].children[0].click();
  const panel = document.getElementById("application");

  // The panel presents the target and the applied package beside it, and the
  // phase is the application object's own (KOSD-C-33).
  assert.equal(rowValue(panel, "目标版本"), "2.0.0");
  assert.equal(rowValue(panel, "目标摘要"), "sha256:2222");
  assert.equal(rowValue(panel, "当前摘要"), "sha256:1111");
  assert.equal(rowValue(panel, "阶段"), "Ready");
  assert.doesNotMatch(panel.textContent, /已升级/);

  // The preview is the framework's report: the changed default appears as the
  // value the baseline held and the value the upgrade writes (KOSD-C-34).
  assert.match(panel.textContent, /升级预览/);
  const preview = elementsWithClass(panel, "preview")[0];
  assert.ok(preview, "the panel presents the preflight report");
  assert.equal(rowValue(preview, "目标摘要"), "sha256:2222");
  assert.match(panel.textContent, /mode: "one" → "two"/);
  assert.match(panel.textContent, /update · ConfigMap\/apps\/site/);
  assert.match(panel.textContent, /data\.mode/);
  assert.equal(rowValue(preview, "已回滚"), "否");

  // Confirming submits the page's upgrade write with the target config and the
  // version the API checks against (KOSD-C-32).
  buttonWith(panel, "升级").click();
  await settle();
  const submissions = network.calls.filter((call) => call.method === "PUT");
  assert.equal(submissions.length, 1);
  assert.equal(submissions[0].url, applicationPath);
  assert.equal(submissions[0].body.spec.definitionRef, "web");
  assert.deepEqual(submissions[0].body.spec.config, { mode: "two" });
  assert.equal(submissions[0].body.metadata.resourceVersion, "7");

  // The progress the window then shows is a fresh page read, never a value it
  // made up: the phase is the reader's (KOSD-C-33).
  const reads = network.calls.filter((call) => call.method === "GET" && call.url === pagePath);
  assert.equal(reads.length, 2);
  assert.equal(rowValue(panel, "阶段"), "Ready");
});
