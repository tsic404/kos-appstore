// App-center window: the application side of the KOSD-C-75 handshake and the
// catalogue entries the app center shows (APPSTORE-C-31). An entry is the
// install source, not an installed application: the list and the detail show
// the entry's display information, version, dependencies, install parameter
// structure, declared permission set and package digest, and installing it goes
// through the page's install entry.
//
// The install form is laid out from the entry's config structure — the shell
// never writes down an application field and never judges a value; the write
// path refuses a submission and names the field and its constraint (KOSD-C-32).
// The confirmation preview is the plan request's answer (KOSD-C-62), and the
// confirmed install is the control-plane write the page item names
// (APPSTORE-C-76). The application belongs to the installing session account:
// the surface has no owner entry (KOSD-C-60).
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

// PAGE_PATH is the app-center page the window hosts: its entries item carries
// the offer list the reader generated, and the item's entry names the install
// write (APPSTORE-C-31, KOSD-C-62).
const PAGE_PATH = "/api/v1/pages/app-center";
// PLAN_PATH is the framework plan request the confirmation preview reads.
const PLAN_PATH = "/api/v1/plan-requests";
// INSTALL_ACTION is the page action the entries item is installed through.
const INSTALL_ACTION = "install";

// The Application envelope every install writes. The config keys inside spec
// come from the entry's structure; the envelope itself is the same for every
// entry, so no application field is written down here.
const APPLICATION_API_VERSION = "kos.tsic.top/v1beta1";
const APPLICATION_KIND = "Application";
// DEFAULT_NAMESPACE is the namespace an install names until the operator picks
// another one.
const DEFAULT_NAMESPACE = "default";

// createAppCenter binds the window to one document, one frame window and one
// fetch. Every dependency is injected so the handshake, the reads and the views
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
    action: null,
    install: null,
  };

  const elements = {
    status: doc.getElementById("status"),
    entries: doc.getElementById("entries"),
    entriesEmpty: doc.getElementById("entries-empty"),
    detail: doc.getElementById("detail"),
    install: doc.getElementById("install"),
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

  // loadEntries reads the app-center page and takes the entries item's value:
  // the entries as the reader generated them — the install form fields and the
  // declared permission set — never a shape this module assembles itself
  // (APPSTORE-C-31). The read carries the window token and no console cookie:
  // the console session never reaches this origin (KOSD-C-75).
  async function loadEntries() {
    setStatus("正在读取目录…");
    let response;
    try {
      response = await read(`${state.consoleOrigin}${PAGE_PATH}`, {
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
    const items = body && Array.isArray(body.items) ? body.items : [];
    const item = items.find((candidate) => candidate && candidate.action === INSTALL_ACTION);
    if (!item) {
      state.entries = [];
      state.action = null;
      renderEntries();
      setStatus("应用中心页没有安装入口", true);
      return;
    }
    state.entries = Array.isArray(item.value) ? item.value : [];
    state.action = item.entry && item.entry.path ? item.entry : null;
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

  // answerText reads the failure envelope of a write: its message already names
  // the field and the constraint the write path refused (KOSD-C-32).
  async function answerText(response) {
    try {
      const body = await response.json();
      const error = body && body.error;
      if (error && typeof error.message === "string" && error.message !== "") {
        return error.message;
      }
    } catch {
      // The status code below is the fallback when the body is not readable.
    }
    return `请求失败（${response.status}）`;
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
    closeInstall();
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

  // renderDetail shows one entry from the app-center page value: display
  // information, version, dependencies, the declared permission set, the
  // install parameter structure and the package digest (APPSTORE-C-24,
  // APPSTORE-C-31, APPSTORE-C-77). The structure generates the fields, so no
  // application field is written down here.
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
    row("权限", permissionList(entry.permissions));
    row("安装参数", configList(entry.config));
    if (!entry.installable) {
      row("不可安装", text(entry.reason || "原因未知"));
      if (Array.isArray(entry.missing) && entry.missing.length > 0) {
        row("缺失能力", stringList(entry.missing));
      }
    }

    const nodes = [list];
    const note = doc.createElement("p");
    note.className = "note";
    note.textContent = "条目是安装来源；安装后才会产生应用。";
    nodes.push(note);

    // An entry the catalogue refuses offers no install entry: the refusal is
    // shown, not a path around it (APPSTORE-C-24).
    if (entry.installable && state.action) {
      const install = doc.createElement("button");
      install.type = "button";
      install.className = "primary";
      install.textContent = "安装";
      install.addEventListener("click", () => openInstall(entry));
      nodes.push(install);
    }
    return nodes;
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

  // permissionList shows the capability names the package declares for the
  // entry's application. The set is the declaration's, so a name it omits never
  // appears and there is no entry to grant one (APPSTORE-C-77, KOSD-C-73).
  function permissionList(permissions) {
    if (!Array.isArray(permissions) || permissions.length === 0) {
      return text("未声明权限");
    }
    const list = doc.createElement("ul");
    list.className = "permission-names";
    for (const permission of permissions) {
      const item = doc.createElement("li");
      item.textContent = permission;
      list.append(item);
    }
    return list;
  }

  // configList shows one field per declared structure entry. A sensitive field
  // is marked and carries no value or default: the surface reports only whether
  // it is set (KOSD-C-14, APPSTORE-C-28).
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
      if (field.sensitive) {
        parts.push("敏感（不回显）");
        if (field.description) {
          parts.push(field.description);
        }
        item.textContent = parts.join(" · ");
        list.append(item);
        continue;
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

  // openInstall starts the install wizard on one entry.
  function openInstall(entry) {
    state.install = { entry, controls: new Map(), sensitive: new Map(), manifest: null };
    renderInstallForm();
  }

  function closeInstall() {
    state.install = null;
    if (elements.install) {
      elements.install.replaceChildren();
    }
  }

  // renderInstallForm lays the submission form out of the entry's config
  // structure (KOSD-C-32). The name and namespace are the Application envelope
  // every install carries; the fields below them are the structure's, in its
  // order, with its type, default, required mark, range, accepted values and
  // sensitive mark.
  function renderInstallForm() {
    const install = state.install;
    const entry = install.entry;
    const form = doc.createElement("div");
    form.className = "install-form";

    const heading = doc.createElement("h3");
    heading.textContent = `安装 ${entry.name || entry.id}`;
    form.append(heading);

    const owner = doc.createElement("p");
    owner.className = "note";
    owner.textContent = "所有者：发起安装的会话账号；本窗口不提供指定其他所有者的入口。";
    form.append(owner);

    const envelope = doc.createElement("div");
    envelope.className = "envelope";
    envelope.append(
      fieldRow("name", "名称", entry.id, { required: true, hint: "应用名称" }),
      fieldRow("namespace", "命名空间", DEFAULT_NAMESPACE, { required: true, hint: "应用渲染所在的命名空间" }),
    );
    form.append(envelope);

    const fields = Array.isArray(entry.config) ? entry.config : [];
    const structure = doc.createElement("div");
    structure.className = "structure";
    for (const field of fields) {
      structure.append(configRow(field));
    }
    if (fields.length === 0) {
      const empty = doc.createElement("p");
      empty.className = "empty";
      empty.textContent = "这个条目的配置结构没有字段。";
      structure.append(empty);
    }
    form.append(structure);

    form.append(permissionsSection(entry.permissions));

    const error = doc.createElement("p");
    error.className = "error";
    error.hidden = true;
    error.setAttribute("role", "alert");
    form.append(error);

    const actions = doc.createElement("div");
    actions.className = "actions";
    const preview = doc.createElement("button");
    preview.type = "button";
    preview.className = "primary";
    preview.textContent = "预览差异";
    preview.addEventListener("click", () => void previewInstall());
    const cancel = doc.createElement("button");
    cancel.type = "button";
    cancel.textContent = "取消";
    cancel.addEventListener("click", () => closeInstall());
    actions.append(preview, cancel);
    form.append(actions);

    elements.install.replaceChildren(form);
  }

  // fieldRow builds one envelope control: the same input shape for every
  // install, labelled and registered under its field name.
  function fieldRow(name, label, value, { required = false, hint = "" } = {}) {
    const row = doc.createElement("div");
    row.className = "field";
    const caption = doc.createElement("label");
    caption.textContent = required ? label + "（必填）" : label;
    const control = doc.createElement("input");
    control.type = "text";
    control.name = name;
    control.dataset.field = name;
    control.value = value;
    control.required = required;
    control.setAttribute("aria-invalid", "false");
    if (hint) {
      control.placeholder = hint;
    }
    state.install.controls.set(name, control);
    row.append(caption, control);
    return row;
  }

  // configRow builds the control the structure's type picks, carrying the
  // constraints as attributes and text. The shell presents them; it never
  // judges a value, so a bad submission still reaches the write path (KOSD-C-32).
  function configRow(field) {
    const row = doc.createElement("div");
    row.className = "field";

    const caption = doc.createElement("label");
    caption.textContent = field.name;
    const hints = [field.type];
    if (field.required) {
      hints.push("必填");
    }
    if (field.sensitive) {
      hints.push("敏感（不回显）");
    }
    const hint = doc.createElement("span");
    hint.className = "hint";
    hint.textContent = hints.join(" · ");
    caption.append(hint);

    let control;
    if (field.control === "select") {
      control = doc.createElement("select");
      const none = doc.createElement("option");
      none.value = "";
      none.textContent = "（未选择）";
      control.append(none);
      for (const accepted of Array.isArray(field.enum) ? field.enum : []) {
        const option = doc.createElement("option");
        option.value = accepted;
        option.textContent = accepted;
        control.append(option);
      }
      control.value = field.default === undefined || field.default === null ? "" : String(field.default);
    } else if (field.control === "switch") {
      control = doc.createElement("input");
      control.type = "checkbox";
      control.checked = String(field.default) === "true";
    } else {
      control = doc.createElement("input");
      control.type = field.sensitive ? "password" : "text";
      control.value = field.sensitive || field.default === undefined || field.default === null ? "" : String(field.default);
      if (field.control === "number" && !field.sensitive) {
        control.type = "number";
        if (field.minimum !== undefined && field.minimum !== null) {
          control.setAttribute("min", String(field.minimum));
        }
        if (field.maximum !== undefined && field.maximum !== null) {
          control.setAttribute("max", String(field.maximum));
        }
      }
    }
    control.name = field.name;
    control.dataset.field = field.name;
    control.required = Boolean(field.required);
    control.setAttribute("aria-invalid", "false");
    if (field.description) {
      control.placeholder = field.description;
    }
    state.install.controls.set(field.name, control);

    row.append(caption, control);

    if (field.sensitive) {
      // The surface reports whether a sensitive parameter is set and never its
      // value (KOSD-C-14, APPSTORE-C-28).
      const stateLine = doc.createElement("p");
      stateLine.className = "sensitive-state";
      stateLine.textContent = "未设置";
      const update = () => {
        stateLine.textContent = control.value === "" ? "未设置" : "已设置";
      };
      control.addEventListener("input", update);
      control.addEventListener("change", update);
      row.append(stateLine);
      state.install.sensitive.set(field.name, stateLine);
    }

    const constraints = constraintText(field);
    if (constraints) {
      const note = doc.createElement("p");
      note.className = "note";
      note.textContent = constraints;
      row.append(note);
    }
    return row;
  }

  // constraintText renders the structure's declared bounds as text, so the
  // operator sees what the write path enforces before submitting.
  function constraintText(field) {
    const parts = [];
    if (!field.sensitive && field.default !== undefined && field.default !== null) {
      parts.push(`默认 ${field.default}`);
    }
    if (field.minimum !== undefined && field.minimum !== null && field.maximum !== undefined && field.maximum !== null) {
      parts.push(`范围 ${field.minimum}–${field.maximum}`);
    } else if (field.minimum !== undefined && field.minimum !== null) {
      parts.push(`最小 ${field.minimum}`);
    } else if (field.maximum !== undefined && field.maximum !== null) {
      parts.push(`最大 ${field.maximum}`);
    }
    if (Array.isArray(field.enum) && field.enum.length > 0) {
      parts.push(`可选值 ${field.enum.join("/")}`);
    }
    if (field.description) {
      parts.push(field.description);
    }
    return parts.join(" · ");
  }

  // permissionsSection shows the declared permission set on both surfaces; it
  // has no entry that grants or revokes one (APPSTORE-C-77, KOSD-C-75).
  function permissionsSection(permissions) {
    const section = doc.createElement("section");
    section.className = "permissions";
    const heading = doc.createElement("h3");
    heading.textContent = "权限";
    section.append(heading);
    section.append(permissionList(permissions));
    const note = doc.createElement("p");
    note.className = "note";
    note.textContent = "权限取自包声明；未声明的权限不会出现，本窗口不提供授予或撤销入口。";
    section.append(note);
    return section;
  }

  // previewInstall submits the form's manifest to the plan request and renders
  // its answer: the confirmation preview is the framework's own difference list
  // and writes nothing (KOSD-C-62, APPSTORE-C-76).
  async function previewInstall() {
    const install = state.install;
    if (!install) {
      return;
    }
    const manifest = manifestOf(install);
    if (!manifest) {
      return;
    }
    install.manifest = manifest;
    const error = formError();
    if (error) {
      error.hidden = true;
      error.textContent = "";
    }
    let response;
    try {
      response = await read(`${state.consoleOrigin}${PLAN_PATH}`, {
        method: "POST",
        headers: { Authorization: `Bearer ${state.token}`, "Content-Type": "application/json" },
        credentials: "omit",
        body: JSON.stringify({ objects: [manifest] }),
      });
    } catch {
      showFormError("无法连接控制面，请稍后重试");
      return;
    }
    if (response.status === 401) {
      setStatus("窗口令牌已失效，请重新打开应用", true);
      return;
    }
    if (!response.ok) {
      showFormError(await answerText(response));
      return;
    }
    let body;
    try {
      body = await response.json();
    } catch {
      showFormError("预览应答无法解析");
      return;
    }
    install.plan = body && body.plan ? body.plan : emptyPlan();
    renderPreview();
  }

  // manifestOf builds the Application the form submits: the envelope is the
  // same for every entry and the config carries only what the structure
  // declares. No owner is written: the API sets the session account (KOSD-C-60).
  function manifestOf(install) {
    const name = String(controlValue(install, "name") || "").trim();
    const namespace = String(controlValue(install, "namespace") || "").trim() || DEFAULT_NAMESPACE;
    if (name === "") {
      showFormError("请填写应用名称");
      return null;
    }
    const fields = Array.isArray(install.entry.config) ? install.entry.config : [];
    const config = {};
    for (const field of fields) {
      const control = install.controls.get(field.name);
      if (!control) {
        continue;
      }
      if (field.control === "switch") {
        config[field.name] = Boolean(control.checked);
        continue;
      }
      const raw = control.value;
      if (raw === "" || raw === undefined || raw === null) {
        continue;
      }
      if (field.control === "number") {
        const number = Number(raw);
        if (!Number.isNaN(number)) {
          config[field.name] = number;
        }
        continue;
      }
      config[field.name] = raw;
    }
    const spec = { definitionRef: install.entry.definition, namespace };
    if (Object.keys(config).length > 0) {
      spec.config = config;
    }
    return {
      apiVersion: APPLICATION_API_VERSION,
      kind: APPLICATION_KIND,
      metadata: { name, namespace },
      spec,
    };
  }

  function controlValue(install, name) {
    const control = install.controls.get(name);
    return control ? control.value : "";
  }

  function formError() {
    if (!elements.install) {
      return null;
    }
    return findClassName(elements.install, "error");
  }

  // showFormError presents the write path's own refusal: its message names the
  // field and the constraint, and the matching field is marked (KOSD-C-32).
  function showFormError(message) {
    const error = formError();
    if (error) {
      error.hidden = false;
      error.textContent = `安装被拒绝：${message}`;
    }
    for (const [name, control] of state.install.controls) {
      const marked = message.includes(`"${name}"`);
      control.setAttribute("aria-invalid", String(marked));
    }
  }

  // renderPreview shows the plan request's answer: the three difference
  // classes, the rendered summary and the warnings, plus the declared
  // permissions and the session owner the confirmation carries (KOSD-C-62,
  // APPSTORE-C-77).
  function renderPreview() {
    const install = state.install;
    const plan = install.plan;
    const box = doc.createElement("section");
    box.className = "preview";

    const heading = doc.createElement("h3");
    heading.textContent = "确认安装";
    box.append(heading);

    if (plan.summary) {
      const summary = doc.createElement("p");
      summary.className = "plan-summary";
      summary.textContent = plan.summary;
      box.append(summary);
    }

    box.append(planSection("将创建", plan.create, "preview-objects"));
    box.append(planSection("将更新", plan.update, "preview-updates"));
    box.append(planSection("将删除", plan.delete, "preview-deletes"));
    box.append(warningSection(plan.warnings));
    box.append(permissionsSection(install.entry.permissions));

    const sensitive = sensitiveStates(install);
    if (sensitive) {
      box.append(sensitive);
    }

    const owner = doc.createElement("p");
    owner.className = "note";
    owner.textContent = "所有者：发起安装的会话账号。";
    box.append(owner);

    const note = doc.createElement("p");
    note.className = "note";
    note.textContent = "预览不写入任何对象；差异取自计划请求的返回。";
    box.append(note);

    const actions = doc.createElement("div");
    actions.className = "actions";
    const confirm = doc.createElement("button");
    confirm.type = "button";
    confirm.className = "primary";
    confirm.textContent = "确认安装";
    confirm.addEventListener("click", () => void confirmInstall());
    const back = doc.createElement("button");
    back.type = "button";
    back.textContent = "返回修改";
    back.addEventListener("click", () => {
      box.remove();
      setFormDisabled(install, false);
    });
    actions.append(confirm, back);
    box.append(actions);

    install.previewActions = actions;
    setFormDisabled(install, true);
    elements.install.append(box);
  }

  // planSection renders one difference class of the plan: its objects, or the
  // changed fields of an update. An empty class is omitted.
  function planSection(title, entries, className) {
    const section = doc.createElement("section");
    section.className = "plan-class";
    const heading = doc.createElement("h4");
    heading.textContent = title;
    section.append(heading);
    if (!Array.isArray(entries) || entries.length === 0) {
      const empty = doc.createElement("p");
      empty.className = "empty";
      empty.textContent = "无";
      section.append(empty);
      return section;
    }
    const list = doc.createElement("ul");
    list.className = className;
    for (const entry of entries) {
      const item = doc.createElement("li");
      item.textContent = referenceText(entry && entry.ref);
      if (Array.isArray(entry && entry.changes) && entry.changes.length > 0) {
        const changes = doc.createElement("ul");
        changes.className = "changes";
        for (const change of entry.changes) {
          const line = doc.createElement("li");
          line.textContent = `${change.path}: ${printable(change.from)} → ${printable(change.to)}`;
          changes.append(line);
        }
        item.append(changes);
      }
      list.append(item);
    }
    section.append(list);
    return section;
  }

  function warningSection(warnings) {
    const section = doc.createElement("section");
    section.className = "plan-class";
    const heading = doc.createElement("h4");
    heading.textContent = "警告";
    section.append(heading);
    if (!Array.isArray(warnings) || warnings.length === 0) {
      const empty = doc.createElement("p");
      empty.className = "empty";
      empty.textContent = "无";
      section.append(empty);
      return section;
    }
    const list = doc.createElement("ul");
    list.className = "plan-warnings";
    for (const warning of warnings) {
      const item = doc.createElement("li");
      item.textContent = warning && warning.message ? warning.message : String(warning && warning.code || "");
      list.append(item);
    }
    section.append(list);
    return section;
  }

  // sensitiveStates lists the sensitive parameters the submission carries as set
  // or unset — never their values (KOSD-C-14).
  function sensitiveStates(install) {
    const lines = [];
    for (const [name, stateLine] of install.sensitive) {
      const line = doc.createElement("p");
      line.className = "sensitive-state";
      line.textContent = `${name}：${stateLine.textContent}`;
      lines.push(line);
    }
    if (lines.length === 0) {
      return null;
    }
    const box = doc.createElement("section");
    box.className = "sensitive";
    box.append(...lines);
    return box;
  }

  // confirmInstall submits the previewed manifest through the control-plane
  // write the page item names. The answer's object is the created object; the
  // result repeats the previewed set so the two stay comparable
  // (APPSTORE-C-76).
  async function confirmInstall() {
    const install = state.install;
    if (!install || !install.manifest || !state.action) {
      return;
    }
    const namespace = install.manifest.metadata.namespace;
    const address = `${state.consoleOrigin}${state.action.path}?namespace=${encodeURIComponent(namespace)}`;
    let response;
    try {
      response = await read(address, {
        method: state.action.method || "POST",
        headers: { Authorization: `Bearer ${state.token}`, "Content-Type": "application/json" },
        credentials: "omit",
        body: JSON.stringify(install.manifest),
      });
    } catch {
      showFormError("无法连接控制面，请稍后重试");
      return;
    }
    if (response.status === 401) {
      setStatus("窗口令牌已失效，请重新打开应用", true);
      return;
    }
    if (!response.ok) {
      showFormError(await answerText(response));
      return;
    }
    let body;
    try {
      body = await response.json();
    } catch {
      body = {};
    }
    renderResult(body && body.ref ? body.ref : createdOf(install.plan));
  }

  // renderResult shows the created object set and the owner the session
  // supplies; the surface offers no owner entry to change it (KOSD-C-60).
  function renderResult(created) {
    const install = state.install;
    const box = doc.createElement("section");
    box.className = "result";

    const heading = doc.createElement("h3");
    heading.textContent = "安装已提交";
    box.append(heading);

    const label = doc.createElement("p");
    label.className = "note";
    label.textContent = "创建的对象";
    box.append(label);

    const list = doc.createElement("ul");
    list.className = "created-objects";
    for (const ref of Array.isArray(created) ? created : [created]) {
      const item = doc.createElement("li");
      item.textContent = referenceText(ref);
      list.append(item);
    }
    box.append(list);

    const sensitive = sensitiveStates(install);
    if (sensitive) {
      box.append(sensitive);
    }

    const owner = doc.createElement("p");
    owner.className = "note";
    owner.textContent = "所有者：发起安装的会话账号。";
    box.append(owner);

    const actions = doc.createElement("div");
    actions.className = "actions";
    const done = doc.createElement("button");
    done.type = "button";
    done.textContent = "完成";
    done.addEventListener("click", () => {
      closeInstall();
      showSelection();
    });
    actions.append(done);
    box.append(actions);

    // The preview stays readable next to the result: the created set can be
    // compared with the differences the plan listed (APPSTORE-C-76).
    if (install.previewActions) {
      install.previewActions.hidden = true;
    }
    elements.install.append(box);
  }

  // createdOf reads the created objects from the previewed plan; it is the set
  // the confirmation submits.
  function createdOf(plan) {
    const creates = plan && Array.isArray(plan.create) ? plan.create : [];
    return creates.map((entry) => entry && entry.ref).filter(Boolean);
  }

  function emptyPlan() {
    return { create: [], update: [], delete: [], warnings: [] };
  }

  function referenceText(ref) {
    if (!ref) {
      return "";
    }
    return [ref.group, ref.kind, ref.namespace, ref.name].filter((part) => part).join("/");
  }

  function printable(value) {
    if (value === undefined || value === null) {
      return "（无）";
    }
    return String(value);
  }

  // setFormDisabled freezes the form while its preview stands, so the
  // confirmation submits exactly the manifest the preview listed.
  function setFormDisabled(install, disabled) {
    for (const control of install.controls.values()) {
      control.disabled = disabled;
    }
  }

  function findClassName(root, className) {
    const walk = (node) => {
      if (!node || node.nodeType !== 1) {
        return null;
      }
      if (String(node.className || "").split(/\s+/).includes(className)) {
        return node;
      }
      for (const child of node.children || []) {
        const found = walk(child);
        if (found) {
          return found;
        }
      }
      return null;
    };
    return walk(root);
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
