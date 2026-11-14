// APPSTORE-TC-58 from the window side: a parameter the structure marks sensitive
// is a write-only field — the window masks it, reports whether it is set, and no
// rendered text ever carries the value (APPSTORE-AC-28, APPSTORE-C-31,
// KOSD-C-14).

import assert from "node:assert/strict";
import test from "node:test";

import { createAppCenter } from "../../../assets/app.js";
import {
  appCenterPage,
  buttonWith,
  elementsWithClass,
  fakeDocument,
  fakeElement,
  fakeFetch,
  fakeIntervals,
  fakeWindow,
  fieldControl,
  settle,
} from "../../shell/harness.js";

const consoleOrigin = "https://console.example.test";
const pagePath = consoleOrigin + "/api/v1/pages/app-center";
const planPath = consoleOrigin + "/api/v1/plan-requests";
const installPath = consoleOrigin + "/api/v1/resources/Application";
// secret is the submitted value no surface may echo.
const secret = "s3cr3t-Pa55phrase";

function address() {
  return (
    "https://app-center.kos.tsic.top/app-assets/kos-appstore/app-center/1.0.0/sha256:abc/assets/index.html" +
    "?kos-console=" + encodeURIComponent(consoleOrigin) + "&kos-window=w-1"
  );
}

// vault is one entry whose structure declares a required sensitive parameter.
// The structure also carries a default for it: the window still shows only
// whether it is set, never the default value.
const vault = {
  source: "kos-appstore",
  id: "vault",
  name: "Vault",
  version: "1.0.0",
  digest: "sha256:5555",
  definition: "vault",
  installable: true,
  dependencies: [],
  permissions: [],
  config: [
    { name: "endpoint", type: "string", control: "text", required: true, description: "上游地址" },
    { name: "adminPassword", type: "string", control: "text", required: true, sensitive: true, default: "seeded-default", description: "管理员口令" },
  ],
};

async function open() {
  const routes = {
    ["GET " + pagePath]: { status: 200, body: appCenterPage([vault]) },
    ["POST " + planPath]: {
      status: 200,
      body: {
        plan: {
          create: [{ ref: { group: "kos.tsic.top", kind: "Application", namespace: "default", name: "vault" } }],
          update: [],
          delete: [],
          summary: "1 个对象将被创建",
          warnings: [],
        },
      },
    },
    ["POST " + installPath + "?namespace=default"]: {
      status: 202,
      body: { accepted: true, ref: { group: "kos.tsic.top", kind: "Application", namespace: "default", name: "vault" } },
    },
  };
  const document = fakeDocument({
    status: fakeElement("p"),
    entries: fakeElement("ul"),
    "entries-empty": fakeElement("p"),
    detail: fakeElement("div"),
    install: fakeElement("div"),
  });
  const window = fakeWindow(address());
  const network = fakeFetch(routes);
  createAppCenter({ document, window, fetch: network.fetch, setInterval: fakeIntervals().setInterval }).start();
  window.deliver({
    origin: consoleOrigin,
    source: window.parent,
    data: { type: "init", token: "window-token", theme: "dark", capabilities: [], expiresAt: "2026-11-13T15:00:00Z" },
  });
  await settle();
  return { document, network };
}

// rendered returns every text the window shows, so a case can assert a value
// appears nowhere at all.
function rendered(document) {
  return ["status", "entries", "entries-empty", "detail", "install"]
    .map((id) => document.getElementById(id))
    .filter(Boolean)
    .map((element) => element.textContent)
    .join("\n");
}

test("敏感参数只显示是否已设置，界面不回显值", async () => {
  const { document, network } = await open();

  // The detail marks the parameter sensitive and shows no default for it.
  document.getElementById("entries").children[0].children[0].click();
  const detail = document.getElementById("detail");
  assert.match(detail.textContent, /adminPassword/);
  assert.match(detail.textContent, /敏感/);
  assert.doesNotMatch(detail.textContent, /seeded-default/);

  buttonWith(detail, "安装").click();
  const install = document.getElementById("install");
  const control = fieldControl(install, "adminPassword");
  assert.equal(control.type, "password");
  assert.equal(control.value, "");
  const state = elementsWithClass(install, "sensitive-state")[0];
  assert.equal(state.textContent, "未设置");
  assert.doesNotMatch(rendered(document), new RegExp("seeded-default"));

  // A typed value stays in the field and is reported as set, never as text.
  control.value = secret;
  control.fire("input");
  assert.equal(state.textContent, "已设置");
  assert.equal(control.value, secret);
  assert.doesNotMatch(rendered(document), new RegExp(secret));

  // The confirmation reports the same state and still no value.
  fieldControl(install, "endpoint").value = "https://upstream.example.test";
  buttonWith(install, "预览差异").click();
  await settle();
  assert.match(install.textContent, /确认安装/);
  const confirmation = elementsWithClass(install, "preview")[0];
  assert.ok(
    elementsWithClass(confirmation, "sensitive-state").some(
      (element) => element.textContent.includes("adminPassword") && element.textContent.includes("已设置"),
    ),
  );
  assert.doesNotMatch(rendered(document), new RegExp(secret));

  // The submission carries the value to the write path and the result surface
  // still reports only whether it is set.
  buttonWith(install, "确认安装").click();
  await settle();
  const submitted = network.calls.find((call) => call.url.startsWith(installPath));
  assert.equal(submitted.body.spec.config.adminPassword, secret);
  assert.match(install.textContent, /安装已提交/);
  const result = elementsWithClass(install, "result")[0];
  assert.ok(
    elementsWithClass(result, "sensitive-state").some(
      (element) => element.textContent.includes("adminPassword") && element.textContent.includes("已设置"),
    ),
  );
  assert.doesNotMatch(rendered(document), new RegExp(secret));
});
