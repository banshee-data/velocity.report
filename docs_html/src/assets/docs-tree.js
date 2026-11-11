// Loads the documentation tree into the sidebar.
//
// The tree is rendered once into assets/docs-tree.html rather than into every
// page. Each page carries an empty <nav data-doc-tree-src="..."> whose value is
// the tree file's URL relative to that page. This script fetches the file,
// rewrites its links (which are relative to the tree file) to absolute URLs,
// marks the current page, opens its ancestor folders, and restores and
// persists the reader's expand/collapse state.
//
// The DOM logic is exported for docs_html/test/docs-tree.test.js; in the
// browser the script runs itself.
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  } else {
    api.start(root);
  }
})(typeof window === "undefined" ? undefined : window, function () {
  const TREE_KEY = "docs-tree-open";

  // `/docs/a/index.html` and `/docs/a/` name the same page.
  function normalisePath(pathname) {
    return String(pathname || "/").replace(/\/index\.html$/, "/");
  }

  function readSavedState(storage) {
    try {
      const saved = JSON.parse(storage.getItem(TREE_KEY) || "{}");
      return saved && typeof saved === "object" ? saved : {};
    } catch (e) {
      return {};
    }
  }

  // Rewrite tree links against the tree file's URL and mark the current page.
  // Returns the current link (null when the page is not in the tree) and the
  // folders that contain it. Folder open state is applied separately.
  function markTree(tree, treeURL, currentPath) {
    const current = normalisePath(currentPath);
    const ancestors = new Set();
    let currentLink = null;
    tree.querySelectorAll("a[href]").forEach((link) => {
      const target = new URL(link.getAttribute("href"), treeURL);
      link.setAttribute("href", target.href);
      if (currentLink || normalisePath(target.pathname) !== current) return;
      currentLink = link;
      link.setAttribute("aria-current", "page");
      let itemMarked = false;
      for (let node = link.parentElement; node && node !== tree; node = node.parentElement) {
        const tag = String(node.tagName).toLowerCase();
        if (tag === "li" && !itemMarked) {
          node.classList.add("is-current");
          itemMarked = true;
        }
        if (tag === "details") ancestors.add(node);
      }
    });
    return { currentLink, ancestors };
  }

  // Open the current page's ancestor folders, then apply the reader's saved
  // choices, which win, as they did when the tree was inline. Each folder is
  // set once, to its final state. Returns a map of the folders this changed to
  // the state it set, so the toggle events they raise are not mistaken for the
  // reader's.
  function applyOpenState(tree, ancestors, saved) {
    const changed = new Map();
    tree.querySelectorAll("details[data-tree-path]").forEach((details) => {
      const treePath = details.getAttribute("data-tree-path");
      let open = ancestors.has(details);
      if (Object.prototype.hasOwnProperty.call(saved, treePath)) {
        open = !!saved[treePath];
      }
      if (details.open !== open) {
        details.open = open;
        changed.set(details, open);
      }
    });
    return changed;
  }

  // Persist a folder the reader opened or closed. Toggle events raised by
  // applyOpenState are skipped: a folder opened because the reader is on a
  // page inside it should not stay open on every later page. An event is ours
  // only while the folder still holds the state we set; if the reader flips it
  // first, or the browser never raises ours, their toggle is still recorded.
  // Returns whether the toggle was recorded.
  function onToggle(storage, changed, target) {
    if (!target || String(target.tagName).toLowerCase() !== "details") return false;
    if (changed.has(target)) {
      const ours = changed.get(target) === target.open;
      changed.delete(target);
      if (ours) return false;
    }
    const treePath = target.getAttribute("data-tree-path");
    if (!treePath) return false;
    try {
      const map = readSavedState(storage);
      map[treePath] = target.open;
      storage.setItem(TREE_KEY, JSON.stringify(map));
      return true;
    } catch (e) {
      // ignore — quota or private mode
      return false;
    }
  }

  function showStatus(doc, nav, message) {
    const status = doc.createElement("p");
    status.className = "tree-status";
    status.setAttribute("role", "status");
    status.textContent = message;
    nav.appendChild(status);
  }

  function safeStorage(win) {
    try {
      return win.localStorage;
    } catch (e) {
      return { getItem: () => null, setItem: () => {} };
    }
  }

  // Fetch the tree file, install it in `nav`, and wire persistence. Resolves
  // to the installed tree, or null when loading failed (a status is shown).
  async function loadTree(win, nav) {
    const doc = win.document;
    const treeURL = new URL(nav.getAttribute("data-doc-tree-src"), doc.baseURI).href;
    const storage = safeStorage(win);
    let tree;
    try {
      const response = await win.fetch(treeURL);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const parsed = new win.DOMParser().parseFromString(await response.text(), "text/html");
      tree = parsed.querySelector("[data-doc-tree]");
      if (!tree) throw new Error("no [data-doc-tree] element");
    } catch (error) {
      win.console.warn(`docs tree: could not load ${treeURL}:`, error);
      nav.setAttribute("data-doc-tree-state", "error");
      showStatus(
        doc,
        nav,
        "The documentation tree could not be loaded. Use the breadcrumbs above the page to navigate.",
      );
      return null;
    }
    tree = doc.importNode(tree, true);
    const { ancestors } = markTree(tree, treeURL, win.location.pathname);
    const changed = applyOpenState(tree, ancestors, readSavedState(storage));
    tree.addEventListener("toggle", (event) => onToggle(storage, changed, event.target), true);
    nav.appendChild(tree);
    nav.setAttribute("data-doc-tree-state", "loaded");
    return tree;
  }

  function start(win) {
    const nav = win.document.querySelector("nav[data-doc-tree-src]");
    if (!nav) return null;
    return loadTree(win, nav);
  }

  return {
    TREE_KEY,
    applyOpenState,
    loadTree,
    markTree,
    normalisePath,
    onToggle,
    readSavedState,
    start,
  };
});
