import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as THREE from "three";
import * as VendoredTHREE from "../../vendor/three/three.module.js";
import { moduleVersion } from "../../../module-version.js";

const VENDOR_SPECIFIER = "../vendor/three/three.module.js";
const fromSite = (path) => new URL(`../../../${path}`, import.meta.url);
const read = (path) => readFileSync(fromSite(path), "utf8");
const modules = readdirSync(fromSite("src/js")).filter((f) =>
  f.endsWith(".js"),
);
const templates = readdirSync(fromSite("src"), { recursive: true }).filter(
  (f) => f.endsWith(".njk"),
);

test("the vendored path resolves to the same module the tests import", () => {
  assert.equal(VendoredTHREE, THREE);
});

test("the build copies three.js to where the scene modules import it from", () => {
  const served = new URL(VENDOR_SPECIFIER, "https://example.test/js/a.js");
  const config = read(".eleventy.js");
  assert.ok(config.includes("`${modulesDir}" + served.pathname + "`"));
  assert.ok(config.includes('"src/js": `${modulesDir}/js`'));
});

// The Wayback Machine rewrites a module's URL so that its relative imports stay
// in module mode, but it never rewrites an import map. A mapped root path
// reaches it as a classic script, and the module stops parsing at its first
// import: the archived page reports "Unexpected token '{'" and shows no scene.
test("scene modules import three.js by relative path", () => {
  const importers = modules.filter((file) => {
    const source = read(`src/js/${file}`);
    assert.doesNotMatch(source, /from\s+["']three["']/, file);
    return source.includes(`from "${VENDOR_SPECIFIER}"`);
  });
  assert.ok(importers.includes("scene-player.js"));
});

test("no template carries an import map", () => {
  for (const file of templates) {
    assert.doesNotMatch(read(`src/${file}`), /importmap/, file);
  }
});

// A saved page can reuse a URL's earlier capture. Module URLs that stayed the
// same across a change left a new page running the old scene-player.js, which
// failed with: Failed to resolve module specifier "three".
test("templates name modules through the versioned base", () => {
  for (const file of templates) {
    assert.doesNotMatch(
      read(`src/${file}`),
      /(src="|from "|\{\{ ')\/js\//,
      file,
    );
  }
});

test("the module version follows the modules and three.js", () => {
  const dir = mkdtempSync(join(tmpdir(), "module-version-"));
  writeFileSync(join(dir, "a.js"), "export const a = 1;");
  writeFileSync(join(dir, "notes.txt"), "not a module");
  const version = moduleVersion(dir, "0.186.1");
  assert.match(version, /^[0-9a-f]{10}$/);
  assert.equal(moduleVersion(dir, "0.186.1"), version);
  assert.notEqual(moduleVersion(dir, "0.187.0"), version);
  writeFileSync(join(dir, "notes.txt"), "still not a module");
  assert.equal(moduleVersion(dir, "0.186.1"), version);
  writeFileSync(join(dir, "a.js"), "export const a = 2;");
  assert.notEqual(moduleVersion(dir, "0.186.1"), version);
});
