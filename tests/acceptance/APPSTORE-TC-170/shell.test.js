// APPSTORE-TC-170 from the window side: an application whose stored config
// carries a sensitive value is served that value masked, so a spec rebuilt from
// the read cannot carry it. The window refuses the upgrade instead of
// resubmitting the mask or inventing a value, and the refusal names the
// parameters it is about — for a required sensitive parameter and for one the
// target structure declares a default for alike (APPSTORE-AC-77,
// APPSTORE-C-81, KOSD-C-14, KOSD-C-32).

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
const applicationPath = consoleOrigin + "/api/v1/resources/Application/site?namespace=default";

function address() {
  return (
    "https://app-center.kos.tsic.top/app-assets/kos-appstore/app-center/1.0.0/sha256:abc/assets/index.html" +
    "?kos-console=" + encodeURIComponent(consoleOrigin) + "&kos-window=w-1"
  );
}

// stored renders the application projection the upgrade write would be built
// from: the config values the read path serves, with every sensitive value
// replaced by the mask that says "set" (KOSD-C-14).
function stored(config) {
  return {
    object: {
      apiVersion: "kos.tsic.top/v1beta1",
      kind: "Application",
      metadata: { name: "site", namespace: "default", resourceVersion: "7" },
      spec: { definitionRef: "web", namespace: "default", config },
    },
  };
}

// entry declares the target structure: the field itself, whether it is required
// and whether the new version gives it a default.
function entry(config) {
  return {
    source: "kos-appstore",
    id: "web",
    name: "Web",
    version: "2.0.0",
    digest: "sha256:2222",
    definition: "web",
    installable: true,
    dependencies: [],
    permissions: [],
    config,
  };
}

// site is the installed application the catalogue offers 2.0.0 for.
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
      configChanges: [],
      renderChanges: [],
      irreversible: [],
      migrations: [],
      rolledBack: false,
    },
  },
};

async function open(offer, projection) {
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
    ["GET " + pagePath]: { status: 200, body: appCenterPage([offer], undefined, [site]) },
    ["GET " + applicationPath]: { status: 200, body: projection },
    ["PUT " + applicationPath]: { status: 202, body: { accepted: true } },
  });
  createAppCenter({ document, window, fetch: network.fetch, setInterval: fakeIntervals().setInterval }).start();
  window.deliver({
    origin: consoleOrigin,
    source: window.parent,
    data: { type: "init", token: "window-token", theme: "dark", capabilities: [], expiresAt: "2026-11-13T15:00:00Z" },
  });
  await settle();
  document.getElementById("installed").children[0].children[0].click();
  return { document, network };
}

// assertRefused checks the surface and the network against one refused upgrade.
async function assertRefused({ document, network }, parameter) {
  const panel = document.getElementById("application");

  // Confirming is the attempt the window judges: the page value carries no
  // stored config, so the application object read is where a set sensitive
  // value becomes knowable. The click below reads it and stops there.
  const confirm = buttonWith(panel, "升级");
  assert.ok(confirm, "the surface offers the upgrade the page declared");
  confirm.click();
  await settle();

  assert.match(panel.textContent, /升级被拒绝/);
  assert.match(panel.textContent, new RegExp(parameter));
  assert.equal(rowValue(panel, "阶段"), "Ready");

  // No mask and no invented value reaches any submission: the write is not
  // issued at all.
  assert.deepEqual(
    network.calls.map((call) => call.method + " " + call.url),
    ["GET " + pagePath, "GET " + applicationPath],
  );
  assert.ok(!JSON.stringify(network.calls).includes("***"), "no request may carry the mask");
  assert.ok(!JSON.stringify(network.calls).includes("from-structure"), "no value is guessed");

  // The confirmation entry is gone once the refusal is known, and no other
  // control submits the upgrade on a later click.
  for (const label of ["升级", "确认", "强制", "绕过"]) {
    assert.equal(buttonWith(panel, label), null, "the surface must not offer " + label);
  }
  for (const button of elementsOf(panel, "button")) {
    button.click();
  }
  await settle();
  assert.deepEqual(
    network.calls.map((call) => call.method + " " + call.url),
    ["GET " + pagePath, "GET " + applicationPath],
  );
}

test("必填敏感项已设置时升级不提交，原因指向该参数", async () => {
  const offer = entry([{ name: "password", type: "string", control: "text", required: true, sensitive: true }]);
  // The value the read serves is the mask, never the parameter itself.
  const opened = await open(offer, stored({ password: "***", user: "svc" }));
  await assertRefused(opened, "password");
});

test("带默认值的敏感项已设置时升级不提交，既不回传掩码也不用默认值替代", async () => {
  const offer = entry([
    { name: "token", type: "string", control: "text", sensitive: true, default: "from-structure" },
  ]);
  const opened = await open(offer, stored({ token: "***" }));
  await assertRefused(opened, "token");
});

test("没有已设置的敏感项时升级照常提交，默认值仍来自目标结构", async () => {
  const offer = entry([
    { name: "password", type: "string", control: "text", required: true, sensitive: true },
    { name: "user", type: "string", control: "text" },
    { name: "mode", type: "string", control: "text", default: "two" },
  ]);
  const opened = await open(offer, stored({ user: "svc" }));
  const panel = opened.document.getElementById("application");
  buttonWith(panel, "升级").click();
  await settle();
  const submissions = opened.network.calls.filter((call) => call.method === "PUT");
  assert.equal(submissions.length, 1);
  assert.deepEqual(submissions[0].body.spec.config, { user: "svc", mode: "two" });
});
