// APPSTORE-TC-164 from the window side: the installed list and the upgrade
// entry render exactly the applications the reader served for the reading
// session — the window neither adds nor filters one — while the catalogue entry
// list is the same for every identity, because an entry is not an application
// (APPSTORE-AC-76, APPSTORE-C-78, KOSD-C-59).

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

// web is the entry every identity reads; an entry is not an application, so
// the entries list does not depend on the session (APPSTORE-C-78).
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

// latest is the update an application on the offered package carries.
const latest = { available: false };
// offered is the update an application behind the catalogue carries.
const offered = { available: true, version: "2.0.0", digest: "sha256:2222", installedDigest: "sha256:1111" };

// application builds one served installed item the way the reader renders it.
function application(name, update) {
  return {
    namespace: "default",
    name,
    definition: "web",
    phase: "Ready",
    health: "Healthy",
    conditions: [{ type: "Reconciled", status: "True" }],
    update,
  };
}

// open mounts the window against one page answer: what the reader serves for
// one identity is the whole input, so the rendered list is a fact about the
// reader, not a second judgement in the window.
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

// installedNames returns the application names one rendered list carries.
function installedNames(document) {
  return document
    .getElementById("installed")
    .children.map((item) => item.children[0].children[0].textContent.trim());
}

test("已安装应用与升级入口只渲染读者给的应用，四种身份的条目列表相同", async () => {
  // The owner sees every application; the unlisted member only its own; the
  // listed member two; the guest the application open to guests (KOSD-C-59).
  const owner = await open([application("mine", offered), application("shared", latest), application("open", latest)]);
  const unlisted = await open([application("mine", offered)]);
  const listed = await open([application("mine", offered), application("shared", latest)]);
  const guest = await open([application("open", latest)]);

  assert.deepEqual(installedNames(owner.document), ["mine", "shared", "open"]);
  assert.deepEqual(installedNames(unlisted.document), ["mine"]);
  assert.deepEqual(installedNames(listed.document), ["mine", "shared"]);
  assert.deepEqual(installedNames(guest.document), ["open"]);

  // The entry list is not filtered by application visibility: every identity
  // reads the same catalogue entries (APPSTORE-C-78).
  const entries = [owner, unlisted, listed, guest].map(({ document }) =>
    document.getElementById("entries").textContent,
  );
  for (const text of entries) {
    assert.equal(text, entries[0]);
  }

  // Visibility is the reader's judgement: the window reads the page once and
  // asks for no application list of its own.
  for (const { network } of [owner, unlisted, listed, guest]) {
    assert.deepEqual(network.calls.map((call) => call.method + " " + call.url), ["GET " + pagePath]);
  }

  // The upgrade entry appears exactly where the reader offered an update.
  const applicationPanel = owner.document.getElementById("application");
  owner.document.getElementById("installed").children[0].children[0].click();
  assert.ok(buttonWith(applicationPanel, "升级"));
  owner.document.getElementById("installed").children[1].children[0].click();
  assert.equal(buttonWith(applicationPanel, "升级"), null);
});
