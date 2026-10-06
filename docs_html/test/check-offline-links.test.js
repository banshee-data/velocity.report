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
