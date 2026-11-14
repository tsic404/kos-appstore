// APPSTORE-TC-163 from the window side: the entry detail and the install
// confirmation list exactly the permission set the package declares, an
// undeclared permission name never appears, and neither surface offers a
// grant or revoke entry (APPSTORE-AC-75, APPSTORE-C-77, KOSD-C-73).

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

// undeclared are permission names in the fixed set that neither application
// declares; none of them may be offered anywhere.
const undeclared = ["clipboard-read", "clipboard-write", "file-selection", "external-link"];

function address() {
  return (
    "https://app-center.kos.tsic.top/app-assets/kos-appstore/app-center/1.0.0/sha256:abc/assets/index.html" +
    "?kos-console=" + encodeURIComponent(consoleOrigin) + "&kos-window=w-1"
  );
}

// notifier declares the notification permission; plain declares none.
const notifier = {
  source: "kos-appstore",
  id: "notifier",
  name: "通知中心",
  version: "1.0.0",
  digest: "sha256:6666",
  definition: "notifier",
  installable: true,
  dependencies: [],
  permissions: ["notifications"],
  config: [],
};

const plain = {
  source: "kos-appstore",
  id: "plain",
  name: "纯静态",
  version: "1.0.0",
  digest: "sha256:7777",
  definition: "plain",
  installable: true,
  dependencies: [],
  permissions: [],
  config: [],
};

async function open() {
  const routes = {
    ["GET " + pagePath]: { status: 200, body: appCenterPage([notifier, plain]) },
    ["POST " + planPath]: {
      status: 200,
      body: {
        plan: {
          create: [{ ref: { group: "kos.tsic.top", kind: "Application", namespace: "default", name: "app" } }],
          update: [],
          delete: [],
          summary: "1 个对象将被创建",
          warnings: [],
        },
      },
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
  createAppCenter({
    document,
    window,
    fetch: fakeFetch(routes).fetch,
    setInterval: fakeIntervals().setInterval,
  }).start();
  window.deliver({
    origin: consoleOrigin,
    source: window.parent,
    data: { type: "init", token: "window-token", theme: "dark", capabilities: [], expiresAt: "2026-11-13T15:00:00Z" },
  });
  await settle();
  return { document };
}

// permissionNames returns the names one rendered permission list carries.
function permissionNames(root) {
  const list = elementsWithClass(root, "permission-names")[0];
  if (!list) return [];
  return elementsOf(list, "li").map((item) => item.textContent.trim());
}

// assertNoGrantEntry checks that the surface offers neither a grant nor a
// revoke entry, and no control for a permission name (APPSTORE-C-77).
function assertNoGrantEntry(root) {
  for (const word of ["授予", "撤销", "添加权限"]) {
    assert.equal(buttonWith(root, word), null, "the surface offers no " + word + " entry");
  }
  for (const control of elementsOf(root, "input")) {
    for (const name of [notifier.permissions, undeclared].flat()) {
      assert.notEqual(control.dataset.field, name, "no control names the permission " + name);
    }
  }
}

test("详情与安装确认的权限集合等于包声明，未声明项不出现且没有授予入口", async () => {
  const { document } = await open();
  const install = document.getElementById("install");

  // The declaring entry: detail and confirmation carry exactly the declared
  // permission.
  document.getElementById("entries").children[0].children[0].click();
  const detail = document.getElementById("detail");
  assert.deepEqual(permissionNames(detail), ["notifications"]);
  for (const name of undeclared) {
    assert.doesNotMatch(detail.textContent, new RegExp(name));
  }
  assertNoGrantEntry(detail);

  buttonWith(detail, "安装").click();
  assert.deepEqual(permissionNames(install), ["notifications"]);
  for (const name of undeclared) {
    assert.doesNotMatch(install.textContent, new RegExp(name));
  }
  assertNoGrantEntry(install);

  fieldControl(install, "name").value = "app";
  buttonWith(install, "预览差异").click();
  await settle();
  assert.match(install.textContent, /确认安装/);
  // The confirmation surface itself lists the declared set, not the form above it.
  const confirmation = elementsWithClass(install, "preview")[0];
  assert.deepEqual(permissionNames(confirmation), ["notifications"]);
  for (const name of undeclared) {
    assert.doesNotMatch(confirmation.textContent, new RegExp(name));
  }
  assertNoGrantEntry(confirmation);

  // The application that declares none: both surfaces list no permission at all
  // and still offer no grant entry.
  buttonWith(install, "取消").click();
  document.getElementById("entries").children[1].children[0].click();
  assert.deepEqual(permissionNames(detail), []);
  assert.match(detail.textContent, /未声明权限/);
  for (const name of [...notifier.permissions, ...undeclared]) {
    assert.doesNotMatch(detail.textContent, new RegExp(name));
  }
  assertNoGrantEntry(detail);

  buttonWith(detail, "安装").click();
  assert.deepEqual(permissionNames(install), []);
  assert.match(install.textContent, /未声明权限/);
  for (const name of [...notifier.permissions, ...undeclared]) {
    assert.doesNotMatch(install.textContent, new RegExp(name));
  }
  assertNoGrantEntry(install);

  fieldControl(install, "name").value = "app";
  buttonWith(install, "预览差异").click();
  await settle();
  assert.match(install.textContent, /确认安装/);
  const emptyConfirmation = elementsWithClass(install, "preview")[0];
  assert.deepEqual(permissionNames(emptyConfirmation), []);
  assert.match(emptyConfirmation.textContent, /未声明权限/);
  for (const name of [...notifier.permissions, ...undeclared]) {
    assert.doesNotMatch(emptyConfirmation.textContent, new RegExp(name));
  }
  assertNoGrantEntry(emptyConfirmation);
});
