const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const test = require("node:test");

const checker = path.resolve(__dirname, "../../scripts/check-docs-offline-links.js");

function writePage(root, relative, body) {
  const target = path.join(root, relative, "index.html");
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, `<html><body>${body}</body></html>`);
}

test("offline link checker accepts GitHub and the deliberate homepage surface", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "offline-links-"));
  try {
    writePage(
      root,
      "",
      '<a href="/guide/" data-docs-internal>guide</a><a href="https://github.com/banshee-data/velocity.report/blob/abc/file.go">source</a><a href="/public_html/" data-docs-app-surface>home</a>',
    );
    writePage(root, "guide", '<a id="top"></a>');
    const result = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /Offline docs links OK/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("offline link checker rejects an unmarked same-origin dead link", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "offline-links-"));
  try {
    writePage(root, "", '<a href="missing/">missing</a>');
    const result = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(result.status, 1);
    assert.match(result.stderr, /unresolved link missing/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

function writeFile(root, relative, contents) {
  const target = path.join(root, relative);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, contents);
}

test("offline link checker follows the sidebar tree source and checks its links", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "offline-links-"));
  try {
    writePage(root, "", '<nav data-doc-tree-src="./assets/docs-tree.html"></nav>');
    writePage(root, "guide/deep", '<nav data-doc-tree-src="../../assets/docs-tree.html"></nav>');
    writeFile(
      root,
      "assets/docs-tree.html",
      '<ul data-doc-tree><li><a href="../guide/deep/" data-docs-internal>Deep</a></li></ul>',
    );
    const ok = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(ok.status, 0, ok.stderr);

    // A dead link inside the tree file is reported against that file.
    writeFile(
      root,
      "assets/docs-tree.html",
      '<ul data-doc-tree><li><a href="../gone/" data-docs-internal>Gone</a></li></ul>',
    );
    const dead = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(dead.status, 1);
    assert.match(dead.stderr, /assets\/docs-tree\.html: unresolved link \.\.\/gone\//);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("offline link checker rejects a missing or empty sidebar tree source", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "offline-links-"));
  try {
    writePage(root, "", '<nav data-doc-tree-src="./assets/docs-tree.html"></nav>');
    const missing = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(missing.status, 1);
    assert.match(missing.stderr, /docs tree source \.\/assets\/docs-tree\.html .* does not exist/);

    writeFile(root, "assets/docs-tree.html", "<p>not a tree</p>");
    const empty = spawnSync(process.execPath, [checker, root], { encoding: "utf8" });
    assert.equal(empty.status, 1);
    assert.match(empty.stderr, /has no \[data-doc-tree\] element/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
