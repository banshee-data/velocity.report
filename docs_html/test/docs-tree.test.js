const assert = require("node:assert/strict");
const test = require("node:test");
const cheerio = require("cheerio");

const docsTree = require("../src/assets/docs-tree.js");

// A minimal DOM, enough for docs-tree.js: built from cheerio's parse so the
// fixtures below are real HTML in the shape src/docs-tree.njk renders.
class FakeElement {
  constructor(tagName, attributes = {}) {
    this.tagName = tagName.toUpperCase();
    this.attributes = new Map(Object.entries(attributes));
    this.children = [];
    this.parentElement = null;
    this.listeners = [];
    this.textContent = "";
    this.open = this.attributes.has("open");
    const classes = new Set(String(attributes.class || "").split(/\s+/).filter(Boolean));
    this.classList = {
      add: (name) => classes.add(name),
      contains: (name) => classes.has(name),
    };
  }

  set className(value) {
    this.attributes.set("class", value);
  }

  getAttribute(name) {
    return this.attributes.has(name) ? this.attributes.get(name) : null;
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value));
  }

  appendChild(child) {
    child.parentElement = this;
    this.children.push(child);
    return child;
  }

  addEventListener(type, listener, capture) {
    this.listeners.push({ type, listener, capture });
  }

  dispatch(type, target) {
    for (const entry of this.listeners) {
      if (entry.type === type) entry.listener({ target });
    }
  }

  *descendants() {
    for (const child of this.children) {
      yield child;
      yield* child.descendants();
    }
  }

  // Supports the `tag[attr]` and `[attr]` selectors the script uses.
  matches(selector) {
    const match = selector.match(/^([a-z]*)\[([a-z-]+)\]$/);
    if (!match) throw new Error(`unsupported selector ${selector}`);
    const [, tag, attribute] = match;
    return (!tag || this.tagName === tag.toUpperCase()) && this.attributes.has(attribute);
  }

  querySelectorAll(selector) {
    return [...this.descendants()].filter((node) => node.matches(selector));
  }

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] || null;
  }
}

function fromHTML(html) {
  const $ = cheerio.load(html);
  const convert = (node) => {
    const element = new FakeElement(node.name, node.attribs);
    for (const child of node.children || []) {
      if (child.type === "tag") element.appendChild(convert(child));
    }
    return element;
  };
  const root = new FakeElement("#document");
  root.appendChild(convert($.root().children("html")[0]));
  return root;
}

// Paths in the built site are relative to /assets/docs-tree.html.
const treeHTML = `<!doctype html><html><body><ul class="tree-root" data-doc-tree>
<li class="tree-item"><details class="tree-folder" data-tree-path="/docs"><summary class="tree-summary">
<a class="tree-link tree-folder-link" href="../docs/" data-docs-internal>Docs</a></summary>
<ul class="tree-children">
<li class="tree-item"><details class="tree-folder" data-tree-path="/docs/lidar"><summary class="tree-summary">
<span class="tree-folder-name">Lidar</span></summary>
<ul class="tree-children">
<li class="tree-item"><a class="tree-link tree-leaf" href="../docs/lidar/guide/" data-docs-internal>Guide</a></li>
</ul></details></li>
<li class="tree-item"><a class="tree-link tree-leaf" href="../docs/other/" data-docs-internal>Other</a></li>
</ul></details></li>
<li class="tree-item"><details class="tree-folder" data-tree-path="/data"><summary class="tree-summary">
<span class="tree-folder-name">Data</span></summary>
<ul class="tree-children">
<li class="tree-item"><a class="tree-link tree-leaf" href="../data/x/" data-docs-internal>X</a></li>
</ul></details></li>
</ul></body></html>`;

// The Go server mounts the site at /docs/, so site paths gain that prefix.
const treeURL = "http://127.0.0.1:8080/docs/assets/docs-tree.html";

function memoryStorage(initial) {
  const values = new Map(initial ? [[docsTree.TREE_KEY, initial]] : []);
  return {
    values,
    getItem: (key) => (values.has(key) ? values.get(key) : null),
    setItem: (key, value) => values.set(key, value),
  };
}

function folders(tree) {
  return Object.fromEntries(
    tree
      .querySelectorAll("details[data-tree-path]")
      .map((details) => [details.getAttribute("data-tree-path"), details.open]),
  );
}

test("normalisePath treats index.html and its directory as one page", () => {
  assert.equal(docsTree.normalisePath("/docs/a/index.html"), "/docs/a/");
  assert.equal(docsTree.normalisePath("/docs/a/"), "/docs/a/");
  assert.equal(docsTree.normalisePath(""), "/");
});

test("readSavedState tolerates missing, malformed, and non-object state", () => {
  assert.deepEqual(docsTree.readSavedState(memoryStorage()), {});
  assert.deepEqual(docsTree.readSavedState(memoryStorage("{not json")), {});
  assert.deepEqual(docsTree.readSavedState(memoryStorage("null")), {});
  assert.deepEqual(docsTree.readSavedState(memoryStorage('{"/docs":true}')), { "/docs": true });
});

test("markTree resolves links against the tree file and marks a nested current page", () => {
  const tree = fromHTML(treeHTML).querySelector("[data-doc-tree]");
  const { currentLink, ancestors } = docsTree.markTree(
    tree,
    treeURL,
    "/docs/docs/lidar/guide/index.html",
  );
  assert.equal(currentLink.getAttribute("href"), "http://127.0.0.1:8080/docs/docs/lidar/guide/");
  assert.equal(currentLink.getAttribute("aria-current"), "page");
  assert.equal(currentLink.parentElement.classList.contains("is-current"), true);
  // Only the nearest item is current, not the folders above it.
  const docsItem = tree.querySelector("details[data-tree-path]").parentElement;
  assert.equal(docsItem.classList.contains("is-current"), false);
  assert.deepEqual(
    [...ancestors].map((details) => details.getAttribute("data-tree-path")),
    ["/docs/lidar", "/docs"],
  );
  assert.equal(
    tree.querySelectorAll("a[href]").filter((a) => a.getAttribute("aria-current")).length,
    1,
  );
  assert.equal(
    tree.querySelectorAll("a[href]")[3].getAttribute("href"),
    "http://127.0.0.1:8080/docs/data/x/",
  );
});

test("markTree marks a folder page and leaves an unknown page unmarked", () => {
  const tree = fromHTML(treeHTML).querySelector("[data-doc-tree]");
  const folder = docsTree.markTree(tree, treeURL, "/docs/docs/");
  assert.equal(folder.currentLink.getAttribute("href"), "http://127.0.0.1:8080/docs/docs/");
  assert.deepEqual(
    [...folder.ancestors].map((details) => details.getAttribute("data-tree-path")),
    ["/docs"],
  );

  const other = fromHTML(treeHTML).querySelector("[data-doc-tree]");
  const missing = docsTree.markTree(other, treeURL, "/docs/");
  assert.equal(missing.currentLink, null);
  assert.equal(missing.ancestors.size, 0);
});

test("applyOpenState opens ancestors, lets saved choices win, and reports its changes", () => {
  const tree = fromHTML(treeHTML).querySelector("[data-doc-tree]");
  const { ancestors } = docsTree.markTree(tree, treeURL, "/docs/docs/lidar/guide/");
  const changed = docsTree.applyOpenState(tree, ancestors, {
    "/docs/lidar": false,
    "/data": true,
  });
  assert.deepEqual(folders(tree), { "/docs": true, "/docs/lidar": false, "/data": true });
  // /docs/lidar was closed and stays closed, so it is not a change.
  assert.deepEqual(
    [...changed].map(([details, open]) => [details.getAttribute("data-tree-path"), open]),
    [
      ["/docs", true],
      ["/data", true],
    ],
  );
});

test("onToggle records the reader's toggles and skips the script's own", () => {
  const tree = fromHTML(treeHTML).querySelector("[data-doc-tree]");
  const storage = memoryStorage();
  const { ancestors } = docsTree.markTree(tree, treeURL, "/docs/docs/lidar/guide/");
  const changed = docsTree.applyOpenState(tree, ancestors, {});
  const [docs, lidar, data] = tree.querySelectorAll("details[data-tree-path]");

  // The browser's toggle event for a programmatic open is not persisted.
  assert.equal(docsTree.onToggle(storage, changed, docs), false);
  assert.equal(storage.getItem(docsTree.TREE_KEY), null);

  // The reader closes /docs/lidar before its programmatic event arrives.
  lidar.open = false;
  assert.equal(docsTree.onToggle(storage, changed, lidar), true);
  assert.deepEqual(JSON.parse(storage.getItem(docsTree.TREE_KEY)), { "/docs/lidar": false });

  data.open = true;
  assert.equal(docsTree.onToggle(storage, changed, data), true);
  assert.deepEqual(JSON.parse(storage.getItem(docsTree.TREE_KEY)), {
    "/docs/lidar": false,
    "/data": true,
  });

  assert.equal(docsTree.onToggle(storage, changed, null), false);
  assert.equal(docsTree.onToggle(storage, changed, tree), false);
  assert.equal(docsTree.onToggle(storage, changed, new FakeElement("details")), false);
  const full = {
    getItem: () => "{}",
    setItem: () => {
      throw new Error("quota");
    },
  };
  assert.equal(docsTree.onToggle(full, new Map(), data), false);
});

function fakeWindow({ response, storage = memoryStorage(), pathname, storageThrows = false }) {
  const page = fromHTML(
    '<html><body><nav class="nav" data-doc-tree-src="../../../assets/docs-tree.html"></nav></body></html>',
  );
  const warnings = [];
  const fetched = [];
  const document = {
    baseURI: `http://127.0.0.1:8080${pathname}`,
    querySelector: (selector) => page.querySelector(selector),
    createElement: (tag) => new FakeElement(tag),
    importNode: (node) => node,
  };
  const win = {
    document,
    location: { pathname },
    console: { warn: (...args) => warnings.push(args) },
    fetch: async (url) => {
      fetched.push(url);
      if (response instanceof Error) throw response;
      return response;
    },
    DOMParser: class {
      parseFromString(text) {
        return fromHTML(text);
      }
    },
  };
  Object.defineProperty(win, "localStorage", {
    get() {
      if (storageThrows) throw new Error("denied");
      return storage;
    },
  });
  return { win, nav: page.querySelector("nav[data-doc-tree-src]"), warnings, fetched };
}

test("start loads the tree into the placeholder and wires persistence", async () => {
  const storage = memoryStorage('{"/data":true}');
  const { win, nav, fetched } = fakeWindow({
    response: { ok: true, status: 200, text: async () => treeHTML },
    storage,
    pathname: "/docs/docs/lidar/guide/",
  });
  const tree = await docsTree.start(win);
  assert.deepEqual(fetched, [treeURL]);
  assert.equal(nav.getAttribute("data-doc-tree-state"), "loaded");
  assert.equal(nav.children[0], tree);
  assert.deepEqual(folders(tree), { "/docs": true, "/docs/lidar": true, "/data": true });
  assert.equal(tree.listeners[0].capture, true);

  const lidar = tree.querySelectorAll("details[data-tree-path]")[1];
  tree.dispatch("toggle", lidar);
  assert.equal(storage.getItem(docsTree.TREE_KEY), '{"/data":true}');
  lidar.open = false;
  tree.dispatch("toggle", lidar);
  assert.deepEqual(JSON.parse(storage.getItem(docsTree.TREE_KEY)), {
    "/data": true,
    "/docs/lidar": false,
  });
});

test("start shows a visible status when the tree cannot be loaded", async () => {
  for (const response of [
    new Error("offline"),
    { ok: false, status: 404, text: async () => "" },
    { ok: true, status: 200, text: async () => "<html><body><p>wrong file</p></body></html>" },
  ]) {
    const { win, nav, warnings } = fakeWindow({
      response,
      pathname: "/docs/docs/lidar/guide/",
      storageThrows: true,
    });
    assert.equal(await docsTree.start(win), null);
    assert.equal(nav.getAttribute("data-doc-tree-state"), "error");
    assert.equal(nav.children[0].getAttribute("role"), "status");
    assert.match(nav.children[0].textContent, /breadcrumbs/);
    assert.equal(warnings.length, 1);
  }
});

test("start does nothing on a page without a tree placeholder", () => {
  const win = { document: { querySelector: () => null } };
  assert.equal(docsTree.start(win), null);
});

test("storage that refuses access falls back to a no-op store", async () => {
  const { win } = fakeWindow({
    response: { ok: true, status: 200, text: async () => treeHTML },
    pathname: "/docs/docs/lidar/guide/",
    storageThrows: true,
  });
  const tree = await docsTree.start(win);
  const data = tree.querySelectorAll("details[data-tree-path]")[2];
  data.open = true;
  tree.dispatch("toggle", data);
  assert.deepEqual(folders(tree), { "/docs": true, "/docs/lidar": true, "/data": true });
});
