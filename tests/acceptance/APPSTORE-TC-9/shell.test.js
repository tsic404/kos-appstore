// APPSTORE-TC-9 from the window side: the app center lays every catalogue entry
// out as an entry — display information, version, dependencies, install
// parameter structure and package digest — and an entry that cannot be
// installed carries the reason and the missing capabilities it is refused with.
// The list is made of entries, never of installed applications
// (APPSTORE-AC-4, APPSTORE-C-31).

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

// address is the frame address the console builds: the console origin and the
// window instance travel as query values, the rest is the signed asset path.
function address(query = "") {
  return "https://app-center.kos.tsic.top/app-assets/kos-appstore/app-center/1.0.0/sha256:abc/assets/index.html?" + query;
}

// init is the console's answer to `ready`: one string token and the theme
// (KOSD-C-75).
function init(token) {
  return { type: "init", token, theme: "dark", capabilities: [], expiresAt: "2026-11-12T15:00:00Z" };
}

// redis is one installable entry with a range, a default and a sensitive field.
const redis = {
  source: "kos-appstore",
  id: "redis",
  name: "Redis",
  description: "内存键值存储",
  icon: "redis",
  version: "7.4.0",
  digest: "sha256:1111",
  definition: "redis",
  installable: true,
  dependencies: [],
  permissions: [],
  config: [
    { name: "replicas", type: "integer", control: "number", default: "1", minimum: "1", maximum: "5", description: "副本数" },
    { name: "password", type: "string", control: "text", required: true, sensitive: true },
  ],
};

// hub is one entry the catalogue refuses: its declared capability is missing.
const hub = {
  source: "kos-appstore",
  id: "notify-hub",
  name: "通知中心",
  description: "向订阅者投递通知",
  version: "2.0.0",
  digest: "sha256:2222",
  definition: "notify-hub",
  installable: false,
  reason: "缺少必需能力 notify",
  missing: ["notify"],
  dependencies: [{ name: "notify", version: ">=1.0" }],
  permissions: [],
  config: [],
};

// start mounts the window against one app-center page answer.
function start(entries) {
  const document = fakeDocument({
    status: fakeElement("p"),
    entries: fakeElement("ul"),
    "entries-empty": fakeElement("p"),
    detail: fakeElement("div"),
    install: fakeElement("div"),
  });
  const window = fakeWindow(address("kos-console=" + encodeURIComponent(consoleOrigin) + "&kos-window=w-1"));
  const network = fakeFetch({
    ["GET " + consoleOrigin + "/api/v1/pages/app-center"]: { status: 200, body: appCenterPage(entries) },
  });
  const clock = fakeIntervals();
  createAppCenter({
    document,
    window,
    fetch: network.fetch,
    setInterval: clock.setInterval,
  }).start();
  return { document, window, network, clock };
}

test("条目按显示信息、版本、依赖、参数结构与摘要呈现，且不是已安装应用", async () => {
  const { document, window, network, clock } = start([redis, hub]);

  // The app opens the handshake on the origin the frame address names, never on
  // a wildcard.
  assert.deepEqual(window.posted[0], {
    data: { type: "ready", window: "w-1" },
    targetOrigin: consoleOrigin,
  });

  // A message from another origin, or from a source that is not the embedding
  // console, is ignored and reads nothing.
  window.deliver({ origin: "https://elsewhere.example.test", source: window.parent, data: init("forged") });
  window.deliver({ origin: consoleOrigin, source: {}, data: init("forged") });
  assert.equal(network.calls.length, 0);

  window.deliver({ origin: consoleOrigin, source: window.parent, data: init("window-token") });
  await settle();

  // The catalogue read carries the window token and no console cookie.
  assert.equal(network.calls.length, 1);
  assert.equal(network.calls[0].headers.Authorization, "Bearer window-token");
  assert.equal(network.calls[0].credentials, "omit");

  const list = document.getElementById("entries");
  assert.match(list.textContent, /Redis/);
  assert.match(list.textContent, /内存键值存储/);
  assert.match(list.textContent, /7\.4\.0/);
  assert.match(list.textContent, /来源 kos-appstore/);
  assert.match(list.textContent, /不可安装：缺少必需能力 notify/);
  // The list is entries, not applications: no installed or running marker.
  assert.doesNotMatch(list.textContent, /已安装|运行中/);
  assert.equal(document.getElementById("entries-empty").hidden, true);

  // Selecting one entry shows its package digest, dependencies and the fields
  // its config structure generates.
  list.children[0].children[0].click();
  const detail = document.getElementById("detail");
  assert.match(detail.textContent, /Redis/);
  assert.match(detail.textContent, /sha256:1111/);
  assert.match(detail.textContent, /replicas/);
  assert.match(detail.textContent, /integer/);
  assert.match(detail.textContent, /默认 1/);
  assert.match(detail.textContent, /最小 1/);
  assert.match(detail.textContent, /最大 5/);
  assert.match(detail.textContent, /password/);
  assert.match(detail.textContent, /必填/);
  assert.match(detail.textContent, /敏感/);
  assert.match(detail.textContent, /条目是安装来源；安装后才会产生应用/);

  // A refused entry shows its dependency, the reason and the missing capability.
  list.children[1].children[0].click();
  assert.match(detail.textContent, /notify · >=1\.0/);
  assert.match(detail.textContent, /缺失能力/);
  assert.match(detail.textContent, /缺少必需能力 notify/);

  // The window reports liveness inside the console's silence window.
  const alive = clock.active();
  assert.equal(alive.length, 1);
  assert.ok(alive[0].milliseconds < 35000, "the liveness report must beat the console's silence window");
  clock.fire();
  assert.deepEqual(window.posted.at(-1), {
    data: { type: "alive", window: "w-1" },
    targetOrigin: consoleOrigin,
  });
});
