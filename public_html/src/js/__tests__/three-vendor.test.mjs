import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import * as THREE from "three";
import * as VendoredTHREE from "../../vendor/three/three.module.js";

const VENDOR_SPECIFIER = "../vendor/three/three.module.js";
const fromSite = (path) => new URL(`../../../${path}`, import.meta.url);
const read = (path) => readFileSync(fromSite(path), "utf8");
const modules = readdirSync(fromSite("src/js")).filter((f) =>
  f.endsWith(".js"),
);
const layouts = readdirSync(fromSite("src/_layouts")).filter((f) =>
  f.endsWith(".njk"),
);

test("the vendored path resolves to the same module the tests import", () => {
  assert.equal(VendoredTHREE, THREE);
});

test("the build copies three.js to where the scene modules import it from", () => {
  const served = new URL(VENDOR_SPECIFIER, "https://example.test/js/a.js");
  assert.ok(read(".eleventy.js").includes(`"${served.pathname.slice(1)}"`));
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

test("no layout carries an import map", () => {
  for (const file of layouts) {
    assert.doesNotMatch(read(`src/_layouts/${file}`), /importmap/, file);
  }
});
