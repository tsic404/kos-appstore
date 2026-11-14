// APPSTORE-TC-57 from the window side: the entry detail shows the version,
// dependencies, declared permissions and package digest, an entry whose
// capability is missing offers no install entry and lists the missing
// capability, and the install form is generated from the config structure — a
// submission that misses a required field or leaves the declared range is
// refused with the field and its constraint named, and no application object is
// produced (APPSTORE-AC-28, APPSTORE-C-31, KOSD-C-32).

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

// wiki is one installable entry whose form the config structure generates: an
// enum, a bounded integer and a switch, with names no shipped application uses.
const wiki = {
  source: "kos-appstore",
  id: "wiki",
  name: "Wiki",
  description: "团队文档",
  version: "1.4.0",
  digest: "sha256:3333",
  definition: "wiki",
  installable: true,
  dependencies: [{ name: "postgres", version: ">=14" }],
  permissions: [],
  config: [
    { name: "cacheMode", type: "enum", control: "select", required: true, enum: ["dev", "prod"], description: "缓存模式" },
    { name: "shardCount", type: "integer", control: "number", required: true, default: "2", minimum: "2", maximum: "4", description: "分片数" },
    { name: "warmup", type: "boolean", control: "switch", default: "true", description: "预热" },
  ],
};

// hub is one entry the catalogue refuses: its declared capability is absent.
const hub = {
  source: "kos-appstore",
  id: "notify-hub",
  name: "通知中心",
  version: "2.0.0",
  digest: "sha256:4444",
  definition: "notify-hub",
  installable: false,
  reason: "缺少必需能力 notify",
  missing: ["notify"],
  dependencies: [{ name: "notify", version: ">=1.0" }],
  permissions: [],
  config: [],
};

// open mounts the window over one page answer and completes the handshake. The
// routes object stays addressable, so a case rewrites the plan answer between
// attempts the way the write path answers each submission.
async function open(offers) {
  const routes = {
    ["GET " + pagePath]: { status: 200, body: appCenterPage(offers) },
    ["POST " + planPath]: {
      status: 400,
      body: {
        error: {
          code: "invalid_field",
          message: 'config field "cacheMode": the field is required',
          stage: "validate",
          sideEffects: "none",
        },
      },
    },
    ["POST " + installPath + "?namespace=default"]: {
      status: 202,
      body: { accepted: true, ref: { group: "kos.tsic.top", kind: "Application", namespace: "default", name: "wiki" } },
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
  return { document, window, network, routes };
}

// created returns the requests that submitted an install.
function created(network) {
  return network.calls.filter((call) => call.url.startsWith(installPath));
}

test("详情带版本、依赖、权限与摘要；不可安装条目没有安装入口", async () => {
  const { document } = await open([wiki, hub]);

  // The list is entries; the refused one carries the missing capability.
  const list = document.getElementById("entries");
  assert.match(list.textContent, /Wiki/);
  assert.match(list.textContent, /1\.4\.0/);
  assert.match(list.textContent, /不可安装：缺少必需能力 notify/);

  // The installable entry's detail shows the version, dependencies, the declared
  // permission set (none here) and the package digest.
  list.children[0].children[0].click();
  const detail = document.getElementById("detail");
  assert.match(detail.textContent, /1\.4\.0/);
  assert.match(detail.textContent, /sha256:3333/);
  assert.match(detail.textContent, /postgres · >=14/);
  assert.match(detail.textContent, /未声明权限/);
  assert.ok(buttonWith(detail, "安装"));

  // The refused entry lists what is missing and offers no install entry.
  list.children[1].children[0].click();
  assert.match(detail.textContent, /缺失能力/);
  assert.match(detail.textContent, /notify/);
  assert.equal(buttonWith(detail, "安装"), null);
});

test("安装表单由配置结构生成，拒绝在写入前指出字段与约束", async () => {
  const { document, network, routes } = await open([wiki]);

  document.getElementById("entries").children[0].children[0].click();
  buttonWith(document.getElementById("detail"), "安装").click();

  const install = document.getElementById("install");
  const mode = fieldControl(install, "cacheMode");
  assert.equal(mode.tagName, "SELECT");
  assert.deepEqual(
    elementsOf(mode, "option").map((option) => option.value),
    ["", "dev", "prod"],
  );
  const shards = fieldControl(install, "shardCount");
  assert.equal(shards.type, "number");
  assert.equal(shards.value, "2");
  assert.equal(shards.getAttribute("min"), "2");
  assert.equal(shards.getAttribute("max"), "4");
  const warmup = fieldControl(install, "warmup");
  assert.equal(warmup.type, "checkbox");
  assert.equal(warmup.checked, true);
  assert.match(install.textContent, /必填/);
  assert.match(install.textContent, /缓存模式/);
  assert.match(install.textContent, /分片数/);
  // The application envelope is generic; no application field is written down.
  assert.equal(fieldControl(install, "name").value, "wiki");
  assert.equal(fieldControl(install, "namespace").value, "default");

  // A submission that omits the required enum is refused by the write path with
  // the field and the constraint named, and nothing is created.
  mode.value = "";
  buttonWith(install, "预览差异").click();
  await settle();
  assert.match(install.textContent, /cacheMode/);
  assert.match(install.textContent, /required/);
  assert.equal(mode.getAttribute("aria-invalid"), "true");
  assert.doesNotMatch(install.textContent, /安装已提交/);
  assert.equal(created(network).length, 0);

  // An out-of-range value is refused the same way, naming the maximum.
  routes["POST " + planPath] = {
    status: 400,
    body: {
      error: {
        code: "invalid_field",
        message: 'config field "shardCount": the value is above the maximum 4',
        stage: "validate",
        sideEffects: "none",
      },
    },
  };
  mode.value = "prod";
  shards.value = "9";
  buttonWith(install, "预览差异").click();
  await settle();
  assert.match(install.textContent, /shardCount/);
  assert.match(install.textContent, /maximum 4/);
  assert.equal(shards.getAttribute("aria-invalid"), "true");
  assert.equal(mode.getAttribute("aria-invalid"), "false");
  assert.equal(created(network).length, 0);

  // A valid structure is previewed from the plan request; the preview alone
  // creates nothing.
  routes["POST " + planPath] = {
    status: 200,
    body: {
      plan: {
        create: [{ ref: { group: "kos.tsic.top", kind: "Application", namespace: "default", name: "wiki" } }],
        update: [],
        delete: [],
        summary: "1 个对象将被创建",
        warnings: [],
      },
    },
  };
  shards.value = "3";
  buttonWith(install, "预览差异").click();
  await settle();
  assert.match(install.textContent, /1 个对象将被创建/);
  assert.match(install.textContent, /kos\.tsic\.top\/Application\/default\/wiki/);
  assert.equal(created(network).length, 0);

  // The confirmation submits the manifest the preview planned, and the created
  // object is the one the preview listed.
  buttonWith(install, "确认安装").click();
  await settle();
  const submissions = created(network);
  assert.equal(submissions.length, 1);
  assert.equal(submissions[0].url, installPath + "?namespace=default");
  assert.deepEqual(submissions[0].body.spec, {
    definitionRef: "wiki",
    namespace: "default",
    config: { cacheMode: "prod", shardCount: 3, warmup: true },
  });
  assert.match(install.textContent, /安装已提交/);
  assert.match(install.textContent, /kos\.tsic\.top\/Application\/default\/wiki/);
});
