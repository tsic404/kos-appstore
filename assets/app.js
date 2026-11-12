// App-center window: the application side of the KOSD-C-75 handshake and the
// catalogue entries the app center shows (APPSTORE-C-31). An entry is the
// install source, not an installed application: the list and the detail show
// the entry's display information, version, dependencies, install parameter
// structure and package digest, and installing it is the next step.
//
// The console origin and the window instance arrive in the frame address, so
// every report goes to one concrete origin and never to a wildcard; the
// application token stays in this closure and is never persisted.

// The message names the console channel uses (KOSD-C-75).
const READY_MESSAGE = "ready";
const ALIVE_MESSAGE = "alive";
const INIT_MESSAGE = "init";

// ALIVE_INTERVAL_MS stays well under the console's silence window, so a running
// window never looks stopped (DASH-C-74).
const ALIVE_INTERVAL_MS = 15000;

// createAppCenter binds the window to one document, one frame window and one
// fetch. Every dependency is injected so the handshake, the read and the view
// can be exercised without a browser.
export function createAppCenter({
  document: doc,
  window: frame,
  fetch: read = globalThis.fetch,
  setInterval: startInterval = globalThis.setInterval,
}) {
  const state = {
    consoleOrigin: "",
    windowId: "",
    token: "",
    entries: [],
    selectedId: "",
  };

  const elements = {
    status: doc.getElementById("status"),
    entries: doc.getElementById("entries"),
    entriesEmpty: doc.getElementById("entries-empty"),
    detail: doc.getElementById("detail"),
  };

  const buttons = new Map();
  let alive = 0;

  // start resolves the console from the frame address and opens the handshake.
  // An address without the console origin names no permissible target, so the
  // window reports why it cannot start instead of posting into the dark.
  function start() {
    const search = addressSearch(frame.location && frame.location.href);
    if (!search) {
      setStatus("窗口地址无效，应用不能启动", true);
      return;
    }
    state.consoleOrigin = originOf(search.get("kos-console"));
    state.windowId = (search.get("kos-window") || "").trim();
    if (!state.consoleOrigin) {
      setStatus("窗口地址没有控制台来源，应用不能启动", true);
      return;
    }
    frame.addEventListener("message", receive);
    frame.parent.postMessage(envelope(READY_MESSAGE), state.consoleOrigin);
  }

  // receive judges one inbound message: only an init from the console that
  // embedded this window is accepted, and only once (KOSD-C-75).
  function receive(event) {
    if (!event || event.origin !== state.consoleOrigin || event.source !== frame.parent) {
      return;
    }
    const data = event.data;
    if (!data || data.type !== INIT_MESSAGE || state.token) {
      return;
    }
    if (typeof data.token !== "string" || data.token === "") {
      setStatus("控制台返回的令牌无效", true);
      return;
    }
    state.token = data.token;
    if (typeof data.theme === "string" && data.theme !== "") {
      doc.documentElement.dataset.theme = data.theme;
    }
    armAlive();
    void loadEntries();
  }

  // armAlive reports the window as running for as long as the console listens;
  // the console ends the frame on its own when the token expires.
  function armAlive() {
    if (alive) {
      return;
    }
    alive = startInterval(() => frame.parent.postMessage(envelope(ALIVE_MESSAGE), state.consoleOrigin), ALIVE_INTERVAL_MS);
  }

  function envelope(type) {
    const message = { type };
    if (state.windowId) {
      message.window = state.windowId;
    }
    return message;
  }

  // loadEntries reads the catalogue the active sources offer. The read carries
  // the window token and no console cookie: the console session never reaches
  // this origin (KOSD-C-75).
  async function loadEntries() {
    setStatus("正在读取目录…");
    let response;
    try {
      response = await read(`${state.consoleOrigin}/api/v1/catalogue/entries`, {
        headers: { Authorization: `Bearer ${state.token}` },
        credentials: "omit",
      });
    } catch {
      setStatus("读取目录失败，请稍后重试", true);
      return;
    }
    if (response.status === 401) {
      setStatus("窗口令牌已失效，请重新打开应用", true);
      return;
    }
    if (!response.ok) {
      setStatus(await failureText(response), true);
      return;
    }
    let body;
    try {
      body = await response.json();
    } catch {
      setStatus("读取目录失败：应答无法解析", true);
      return;
    }
    state.entries = body && Array.isArray(body.items) ? body.items : [];
    renderEntries();
    setStatus(`${state.entries.length} 个条目`);
  }

  // failureText keeps the control plane's own reason readable instead of
  // replacing it with a status code (APPSTORE-C-24).
  async function failureText(response) {
    try {
      const body = await response.json();
      const message = body && body.error && body.error.message;
      if (typeof message === "string" && message !== "") {
        return `读取目录失败：${message}`;
      }
    } catch {
      // The status code below is the fallback when the body is not readable.
    }
    return `读取目录失败（${response.status}）`;
  }

  // renderEntries lays the entries out as install sources; an entry nothing
  // offers any more is dropped with the read that returned it, so a removed
  // source leaves no stale row (APPSTORE-C-24).
  function renderEntries() {
    elements.entries.replaceChildren();
    buttons.clear();
    elements.entriesEmpty.hidden = state.entries.length !== 0;
    for (const entry of state.entries) {
      const item = doc.createElement("li");
      const button = doc.createElement("button");
      button.type = "button";
      button.className = "entry";
      button.dataset.id = entry.id;

      const name = doc.createElement("h3");
      name.textContent = entry.name || entry.id;
      button.append(name);

      if (entry.description) {
        const description = doc.createElement("p");
        description.className = "description";
        description.textContent = entry.description;
        button.append(description);
      }

      const meta = doc.createElement("p");
      meta.textContent = `${entry.version || "—"} · 来源 ${entry.source || "—"}`;
      button.append(meta);

      if (!entry.installable) {
        const blocked = doc.createElement("p");
        blocked.className = "blocked";
        blocked.textContent = `不可安装：${entry.reason || "原因未知"}`;
        button.append(blocked);
      }

      button.addEventListener("click", () => selectEntry(entry.id));
      item.append(button);
      elements.entries.append(item);
      buttons.set(entry.id, button);
    }
    showSelection();
  }

  function selectEntry(id) {
    state.selectedId = id;
    showSelection();
  }

  // showSelection keeps the detail on the selected entry and drops it when the
  // read no longer carries that entry.
  function showSelection() {
    const entry = state.entries.find((candidate) => candidate.id === state.selectedId);
    if (!entry) {
      state.selectedId = "";
      elements.detail.replaceChildren(placeholder());
      return;
    }
    for (const [id, button] of buttons) {
      button.setAttribute("aria-current", String(id === entry.id));
    }
    elements.detail.replaceChildren(...renderDetail(entry));
  }

  function placeholder() {
    const empty = doc.createElement("p");
    empty.className = "empty";
    empty.textContent = "选择一个条目查看版本、依赖、参数结构与摘要。";
    return empty;
  }

  // renderDetail shows one entry from the catalogue read model: display
  // information, version, dependencies, the declared install parameter
  // structure and the package digest (APPSTORE-C-24, APPSTORE-C-31). The
  // structure generates the fields, so no application field is written down
  // here.
  function renderDetail(entry) {
    const list = doc.createElement("dl");
    const row = (label, value) => {
      const term = doc.createElement("dt");
      term.textContent = label;
      const description = doc.createElement("dd");
      description.append(value);
      list.append(term, description);
    };

    row("显示名称", text(entry.name || entry.id));
    if (entry.description) {
      row("说明", text(entry.description));
    }
    row("标识", text(entry.id));
    row("版本", text(entry.version || "—"));
    row("来源", text(entry.source || "—"));
    row("包摘要", text(entry.digest || "—"));
    row("依赖", dependencyList(entry.dependencies));
    row("安装参数", configList(entry.config));
    if (!entry.installable) {
      row("不可安装", text(entry.reason || "原因未知"));
      if (Array.isArray(entry.missing) && entry.missing.length > 0) {
        row("缺失能力", stringList(entry.missing));
      }
    }

    const note = doc.createElement("p");
    note.className = "note";
    note.textContent = "条目是安装来源；安装后才会产生应用。";
    return [list, note];
  }

  function text(value) {
    return doc.createTextNode(value);
  }

  function dependencyList(dependencies) {
    if (!Array.isArray(dependencies) || dependencies.length === 0) {
      return text("无");
    }
    const list = doc.createElement("ul");
    list.className = "fields";
    for (const dependency of dependencies) {
      const item = doc.createElement("li");
      const parts = [dependency.name];
      if (dependency.version) {
        parts.push(dependency.version);
      }
      if (dependency.scope === "cluster") {
        parts.push("集群级");
      }
      if (dependency.optional) {
        parts.push("可选");
      }
      item.textContent = parts.join(" · ");
      list.append(item);
    }
    return list;
  }

  function configList(fields) {
    if (!Array.isArray(fields) || fields.length === 0) {
      return text("无");
    }
    const list = doc.createElement("ul");
    list.className = "fields";
    for (const field of fields) {
      const item = doc.createElement("li");
      const parts = [field.name, field.type];
      if (field.required) {
        parts.push("必填");
      }
      if (field.default !== undefined && field.default !== null) {
        parts.push(`默认 ${field.default}`);
      }
      if (field.minimum !== undefined && field.minimum !== null) {
        parts.push(`最小 ${field.minimum}`);
      }
      if (field.maximum !== undefined && field.maximum !== null) {
        parts.push(`最大 ${field.maximum}`);
      }
      if (Array.isArray(field.enum) && field.enum.length > 0) {
        parts.push(`可选值 ${field.enum.join("/")}`);
      }
      if (field.sensitive) {
        parts.push("敏感");
      }
      if (field.description) {
        parts.push(field.description);
      }
      item.textContent = parts.filter((part) => part !== "" && part !== undefined && part !== null).join(" · ");
      list.append(item);
    }
    return list;
  }

  function stringList(values) {
    const list = doc.createElement("ul");
    list.className = "fields";
    for (const value of values) {
      const item = doc.createElement("li");
      item.textContent = value;
      list.append(item);
    }
    return list;
  }

  function setStatus(message, isBlocked = false) {
    elements.status.textContent = message;
    elements.status.classList.toggle("blocked", isBlocked);
  }

  return { start };
}

// addressSearch reads the query of one frame address; an address that does not
// parse names no console.
function addressSearch(href) {
  try {
    return new URL(href).searchParams;
  } catch {
    return null;
  }
}

// originOf returns the origin of one absolute address, or "" when the value is
// not one; the handshake compares whole origins, never path prefixes.
function originOf(value) {
  try {
    const parsed = new URL(value);
    return parsed.origin === "null" ? "" : parsed.origin;
  } catch {
    return "";
  }
}

if (typeof window !== "undefined" && typeof document !== "undefined") {
  createAppCenter({ document, window }).start();
}
