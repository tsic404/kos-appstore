// Shared browser stand-ins for the app-center window: the minimal document the
// view builds with, a frame window that records what the application posts and
// delivers console messages, a fetch that answers by route, and the interval
// clock the liveness report runs on. The window injects every one of them, so
// the cases exercise the module the browser runs.

// fakeDocument builds the document the window reads its ids from.
export function fakeDocument(ids = {}) {
  return {
    documentElement: fakeElement("html"),
    createElement: (tag) => fakeElement(tag),
    createTextNode: (value) => ({ nodeType: 3, textContent: String(value) }),
    getElementById: (id) => ids[id] || null,
  };
}

// fakeElement is the element API the view builds nodes with. textContent reads
// the own text and every descendant's, which is what a rendered document shows;
// an input's value is not text, so a masked parameter never leaks into it. The
// form fields the view reads behave as the browser's: a select resolves value
// through its selected option, and a typed value arrives through fire("input").
export function fakeElement(tag) {
  let ownText = "";
  const classes = new Set();
  const element = {
    tagName: String(tag).toUpperCase(),
    nodeType: 1,
    children: [],
    hidden: false,
    id: "",
    name: "",
    type: "",
    checked: false,
    disabled: false,
    selected: false,
    required: false,
    min: "",
    max: "",
    placeholder: "",
    title: "",
    dataset: {},
    attributes: new Map(),
    listeners: new Map(),
    className: "",
    classList: {
      add(name) {
        classes.add(name);
        element.className = [...classes].join(" ");
      },
      toggle(name, force) {
        const on = force === undefined ? !classes.has(name) : Boolean(force);
        if (on) element.classList.add(name);
        else {
          classes.delete(name);
          element.className = [...classes].join(" ");
        }
        return on;
      },
    },
    append(...nodes) {
      for (const node of nodes) {
        node.parentNode = element;
        element.children.push(node);
      }
    },
    appendChild(node) {
      element.append(node);
      return node;
    },
    replaceChildren(...nodes) {
      element.children = [];
      element.append(...nodes);
    },
    setAttribute(name, value) {
      element.attributes.set(String(name), String(value));
    },
    getAttribute(name) {
      return element.attributes.has(String(name)) ? element.attributes.get(String(name)) : null;
    },
    addEventListener(name, handler) {
      const handlers = element.listeners.get(name) || [];
      handlers.push(handler);
      element.listeners.set(name, handlers);
    },
    click() {
      for (const handler of element.listeners.get("click") || []) handler({ target: element });
    },
    fire(type) {
      for (const handler of element.listeners.get(type) || []) handler({ target: element });
    },
    remove() {
      const siblings = element.parentNode ? element.parentNode.children : [];
      const at = siblings.indexOf(element);
      if (at >= 0) siblings.splice(at, 1);
    },
  };
  // A select reads its value from the chosen option, as the browser does; the
  // first option stands in when none was chosen yet.
  if (element.tagName === "SELECT") {
    Object.defineProperty(element, "value", {
      get() {
        const options = element.children.filter((child) => child.nodeType === 1);
        const chosen = options.find((child) => child.selected) || options[0];
        return chosen ? String(chosen.value ?? "") : "";
      },
      set(next) {
        for (const child of element.children) {
          if (child.nodeType === 1) child.selected = String(child.value ?? "") === String(next ?? "");
        }
      },
    });
  } else {
    element.value = "";
  }
  Object.defineProperty(element, "textContent", {
    get() {
      if (element.children.length === 0) return ownText;
      return ownText + element.children.map((child) => child.textContent).join("");
    },
    set(value) {
      ownText = value === undefined || value === null ? "" : String(value);
      element.children = [];
    },
  });
  return element;
}

// fakeWindow is the frame's window: its location carries the address the host
// loaded it from, parent.postMessage records every report, and deliver hands a
// console message to the registered listener.
export function fakeWindow(address) {
  const posted = [];
  const listeners = new Map();
  const parent = {
    postMessage(data, targetOrigin) {
      posted.push({ data, targetOrigin });
    },
  };
  return {
    location: { href: address, ancestorOrigins: [] },
    parent,
    posted,
    addEventListener(name, handler) {
      const handlers = listeners.get(name) || [];
      handlers.push(handler);
      listeners.set(name, handlers);
    },
    deliver(event) {
      for (const handler of listeners.get("message") || []) handler(event);
    },
  };
}

// reply builds the one response shape the window reads.
export function reply(status, body) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}

// fakeFetch answers by "METHOD URL" and records what the window sent, body
// included: a case asserts the submission it carries, not just its route.
export function fakeFetch(routes) {
  const calls = [];
  const fetch = async (url, init = {}) => {
    const call = {
      method: init.method || "GET",
      url,
      headers: init.headers || {},
      credentials: init.credentials,
      body: typeof init.body === "string" ? JSON.parse(init.body) : undefined,
    };
    calls.push(call);
    const route = routes[`${call.method} ${url}`];
    if (!route) {
      return reply(404, { error: { code: "not_found", message: "missing", stage: "read" } });
    }
    return reply(route.status, route.body);
  };
  return { fetch, calls };
}

// fakeIntervals is the timer the liveness report runs on: nothing fires until a
// case fires it.
export function fakeIntervals() {
  const armed = [];
  return {
    setInterval(handler, milliseconds) {
      armed.push({ handler, milliseconds, cleared: false });
      return armed.length;
    },
    clearInterval(id) {
      const slot = armed[id - 1];
      if (slot) slot.cleared = true;
    },
    active() {
      return armed.filter((slot) => !slot.cleared);
    },
    fire() {
      for (const slot of armed) {
        if (!slot.cleared) slot.handler();
      }
    },
  };
}

// settle lets the window's read chain finish before a case asserts on it.
export async function settle() {
  await new Promise((resolve) => setTimeout(resolve, 5));
}

// elementsOf collects the elements of one tag below a root in document order, so
// a case reads what the view rendered instead of its internal state.
export function elementsOf(root, tagName) {
  const found = [];
  const walk = (node) => {
    if (!node || node.nodeType !== 1) return;
    if (node.tagName === String(tagName).toUpperCase()) found.push(node);
    for (const child of node.children || []) walk(child);
  };
  walk(root);
  return found;
}

// buttonWith returns the first button whose label carries text, or null.
export function buttonWith(root, text) {
  return elementsOf(root, "button").find((button) => button.textContent.includes(text)) || null;
}

// elementsWithClass collects every element carrying one class name.
export function elementsWithClass(root, className) {
  const found = [];
  const walk = (node) => {
    if (!node || node.nodeType !== 1) return;
    if (String(node.className || "").split(/\s+/).includes(className)) found.push(node);
    for (const child of node.children || []) walk(child);
  };
  walk(root);
  return found;
}

// fieldControl returns the control the install form generated for one field
// name, whichever control the structure picked.
export function fieldControl(root, name) {
  return elementsOf(root, "input").find((control) => control.dataset.field === name) ||
    elementsOf(root, "select").find((control) => control.dataset.field === name) ||
    null;
}

// appCenterPage builds one rendered app-center page: the entries item carries
// the offers the reader generated (form fields and declared permissions), and
// the item's entry names the install write the confirmation submits
// (APPSTORE-C-31, KOSD-C-62).
export function appCenterPage(offers, install = { method: "POST", path: "/api/v1/resources/Application" }) {
  const observedAt = "2026-11-13T10:00:00Z";
  return {
    id: "app-center",
    title: "应用中心",
    location: "panel",
    requiredRole: "member",
    readOnly: false,
    sources: ["computed"],
    items: [
      {
        id: "entries",
        label: "可安装应用",
        source: "computed",
        value: offers,
        observedAt,
        stale: false,
        action: "install",
        entry: { method: install.method, path: install.path },
      },
      {
        id: "upgrades",
        label: "可升级应用",
        source: "computed",
        value: [],
        observedAt,
        stale: false,
        action: "upgrade",
        entry: { method: "PUT", path: "/api/v1/resources/Application" },
      },
    ],
    actions: [
      { id: "install", entry: { method: install.method, path: install.path } },
      { id: "upgrade", entry: { method: "PUT", path: "/api/v1/resources/Application" } },
    ],
  };
}
