// APPSTORE-TC-162 from the window side: the confirmation preview is the plan
// request's answer, confirming submits the install through the control-plane
// API, the previewed object set is the created set, and the ownership entry is
// the session account — the surface has no field that names another owner
// (APPSTORE-AC-74, APPSTORE-C-76, KOSD-C-60, KOSD-C-62).

import assert from "node:assert/strict";
import test from "node:test";

import { createAppCenter } from "../../../assets/app.js";
import {
  appCenterPage,
  buttonWith,
  elementsOf,
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

function address() {
  return (
    "https://app-center.kos.tsic.top/app-assets/kos-appstore/app-center/1.0.0/sha256:abc/assets/index.html" +
    "?kos-console=" + encodeURIComponent(consoleOrigin) + "&kos-window=w-1"
  );
}

// redis is the shipped entry the member installs from.
const redis = {
  source: "kos-appstore",
  id: "redis",
  name: "Redis",
  version: "7.4.0",
  digest: "sha256:1111",
  definition: "redis",
  installable: true,
  dependencies: [],
  permissions: [],
  config: [{ name: "replicas", type: "integer", control: "number", default: "1", minimum: "1", maximum: "5" }],
};

// planned is the object reference the plan request answers with; it is the one
// the install creates.
const planned = { group: "kos.tsic.top", kind: "Application", namespace: "default", name: "cache" };

async function open() {
  const routes = {
    ["GET " + pagePath]: { status: 200, body: appCenterPage([redis]) },
    ["POST " + planPath]: {
      status: 200,
      body: {
        plan: {
          create: [{ ref: planned }],
          update: [],
          delete: [],
          summary: "1 个对象将被创建",
          warnings: [{ code: "retained", message: "卷数据将保留", object: planned }],
        },
      },
    },
    ["POST " + installPath + "?namespace=default"]: { status: 202, body: { accepted: true, ref: planned } },
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

// objectList returns the object references one rendered list carries.
function objectList(root, className) {
  const list = elementsWithClass(root, className)[0];
  if (!list) return [];
  return elementsOf(list, "li").map((item) => item.textContent.trim());
}

test("预览取计划请求的返回，确认提交后创建的对象与预览一致且所有者为会话账号", async () => {
  const { document, network } = await open();

  document.getElementById("entries").children[0].children[0].click();
  buttonWith(document.getElementById("detail"), "安装").click();
  const install = document.getElementById("install");

  // The install form names the session account as owner and offers no field for
  // another one (APPSTORE-C-76).
  assert.equal(fieldControl(install, "owner"), null);
  assert.equal(
    elementsOf(install, "input").filter((control) => control.dataset.field === "owner").length,
    0,
  );
  assert.match(install.textContent, /所有者：发起安装的会话账号/);

  // The operator names the application and fills the structure's field.
  fieldControl(install, "name").value = "cache";
  fieldControl(install, "replicas").value = "2";

  // The preview is the plan request's return: its summary and warning are shown,
  // and it writes nothing.
  buttonWith(install, "预览差异").click();
  await settle();
  assert.match(install.textContent, /1 个对象将被创建/);
  assert.match(install.textContent, /卷数据将保留/);
  assert.deepEqual(objectList(install, "preview-objects"), ["kos.tsic.top/Application/default/cache"]);
  assert.match(install.textContent, /预览不写入任何对象/);
  assert.equal(network.calls.filter((call) => call.url.startsWith(installPath)).length, 0);

  // The plan request carries the application the confirmation will submit, and
  // no owner: the API writes the session account (KOSD-C-60).
  const planCall = network.calls.find((call) => call.url === planPath);
  assert.deepEqual(planCall.body, {
    objects: [
      {
        apiVersion: "kos.tsic.top/v1beta1",
        kind: "Application",
        metadata: { name: "cache", namespace: "default" },
        spec: { definitionRef: "redis", namespace: "default", config: { replicas: 2 } },
      },
    ],
  });
  assert.doesNotMatch(JSON.stringify(planCall.body), /owner/);

  // Confirming submits through the control-plane API the page entry names, with
  // the manifest the preview planned.
  buttonWith(install, "确认安装").click();
  await settle();
  const submissions = network.calls.filter((call) => call.url.startsWith(installPath));
  assert.equal(submissions.length, 1);
  assert.equal(submissions[0].url, installPath + "?namespace=default");
  assert.deepEqual(submissions[0].body, planCall.body.objects[0]);

  // The created set is the previewed set, and the result names the session
  // account as owner without offering to change it.
  assert.deepEqual(objectList(install, "created-objects"), objectList(install, "preview-objects"));
  assert.match(install.textContent, /安装已提交/);
  assert.match(install.textContent, /所有者：发起安装的会话账号/);
  assert.equal(fieldControl(install, "owner"), null);
});
