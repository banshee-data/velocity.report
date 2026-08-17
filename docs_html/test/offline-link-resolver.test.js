const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const configure = require("../.eleventy.js");
const helpers = configure._test;
const repoRoot = path.resolve(__dirname, "../..");
const docsRoot = path.join(repoRoot, "docs_html");

test("rewrites repository docs, data, source, homepage, and placeholders", () => {
  const input = path.join(repoRoot, "docs/plans/qemu-headscale-dev-vm-plan.md");

  const docs = helpers.resolveHrefForInput("../../docs/platform/operations/", input);
  assert.deepEqual(docs, {
    href: "/docs/platform/operations/",
    resolved: true,
  });

  const data = helpers.resolveHrefForInput("../../data/explore/kirk0-lifecycle", input);
  assert.deepEqual(data, {
    href: "/data/explore/kirk0-lifecycle/",
    resolved: true,
  });

  const source = helpers.resolveHrefForInput("../../internal/lidar", input);
  assert.match(
    source.href,
    /^https:\/\/github\.com\/banshee-data\/velocity\.report\/tree\/main\/internal\/lidar$/,
  );
  assert.equal(source.resolved, false);

  assert.deepEqual(helpers.resolveHrefForInput("../../public_html/", input), {
    href: "/public_html/",
    resolved: false,
    appSurface: true,
  });

  assert.deepEqual(helpers.resolveHrefForInput("linked-plan-name/", input), {
    href: "linked-plan-name/",
    resolved: false,
    unavailable: true,
  });
});

test("recognises GitHub paths and preserves offline docs targets", () => {
  assert.equal(
    helpers.githubRepositoryPath(
      "https://github.com/banshee-data/velocity.report/blob/abc1234/internal/config/tuning.go#L1",
    ),
    "internal/config/tuning.go",
  );
  assert.equal(
    helpers.githubRepositoryPath(
      "https://github.com/banshee-data/velocity.report/tree/main/docs/platform",
    ),
    "docs/platform",
  );
  assert.equal(helpers.githubRepositoryPath("https://example.com/a"), null);

  const docs = helpers.resolveHrefForInput(
    "https://github.com/banshee-data/velocity.report/blob/main/data/maths/MATHS.md",
    path.join(repoRoot, "README.md"),
  );
  assert.deepEqual(docs, { href: "/data/maths/MATHS/", resolved: true });
});

test("path and navigation helpers cover routing edge cases", () => {
  assert.equal(helpers.isExternalHref("https://example.com"), true);
  assert.equal(helpers.isExternalHref("#section"), true);
  assert.equal(helpers.isExternalHref("README.md"), false);
  assert.equal(helpers.rewriteMarkdownHref("README.md#start"), "README/#start");
  assert.equal(helpers.rewriteMarkdownHref("asset.json"), "asset.json");
  assert.equal(
    helpers.relativeURLForPage("/docs/platform/", "/docs/plans/example/"),
    "../../platform/",
  );
  assert.deepEqual(helpers.splitHref("path?q=1#x"), {
    pathname: "path",
    query: "?q=1",
    hash: "#x",
  });
  assert.equal(helpers.isWithin(repoRoot, path.join(repoRoot, "docs")), true);
  assert.equal(helpers.isWithin(repoRoot, path.resolve(repoRoot, "..")), false);
  assert.equal(helpers.outputURLForSourcePath(repoRoot, path.join(repoRoot, "README.md")), "/README/");
  assert.equal(helpers.outputURLForSourcePath(repoRoot, path.join(repoRoot, "docs/ui/DESIGN.md")), "/docs/ui/design-document/");
  assert.equal(helpers.outputURLForSourcePath(repoRoot, path.join(repoRoot, "docs/platform/README.md")), "/docs/platform/");
  assert.equal(helpers.outputURLForSourcePath(repoRoot, path.join(repoRoot, "asset.json")), "/asset.json");
  assert.equal(helpers.relativeURLForPage("/docs/", "/docs/"), "./");
  assert.equal(helpers.relativeURLForPage("/docs/guide", "/"), "./docs/guide");
  assert.equal(helpers.relativeURLForPage("/docs/z/", "/docs/a/"), "../z/");
  assert.equal(helpers.navGroup("src/docs/radar/example.md"), "Radar");
  assert.equal(helpers.navGroup("src/docs/lidar/example.md"), "LiDAR");
  assert.equal(helpers.navGroup("src/docs/platform/example.md"), "Platform");
  assert.equal(helpers.navGroup("src/docs/ui/example.md"), "UI");
  assert.equal(helpers.navGroup("src/docs/plans/example.md"), "Plans");
  assert.equal(helpers.navGroup("src/data/structures/example.md"), "Data Structures");
  assert.equal(helpers.navGroup("src/data/maths/example.md"), "Maths");
  assert.equal(helpers.navGroup("src/data/example.md"), "Data");
  assert.equal(helpers.navGroup("src/docs/example.md"), "Docs");
  assert.equal(helpers.navGroup("README.md"), "Repository");
  assert.equal(helpers.humanizeSegment("velocity_report"), "Velocity Report");
  assert.equal(helpers.githubSlugify("A <Test> & Value"), "a-test--value");
});

test("resolves local inputs and every repository fallback", () => {
  const docsInput = path.join(repoRoot, "docs/plans/qemu-headscale-dev-vm-plan.md");
  const siteInput = path.join(docsRoot, "src/README.md");

  assert.deepEqual(helpers.resolveHrefForInput("README.md", siteInput), {
    href: "/README/",
    resolved: true,
  });
  assert.deepEqual(helpers.resolveHrefForInput("docs/platform/operations", docsInput), {
    href: "/docs/platform/operations/",
    resolved: true,
  });
  assert.deepEqual(helpers.resolveHrefForInput("public_html", docsInput), {
    href: "/public_html/",
    resolved: false,
    appSurface: true,
  });
  assert.match(
    helpers.resolveHrefForInput("internal/lidar", docsInput).href,
    /\/tree\/main\/internal\/lidar$/,
  );
  assert.deepEqual(helpers.resolveHrefForInput("https://example.com", docsInput), {
    href: "https://example.com",
    resolved: false,
  });
  assert.deepEqual(helpers.resolveHrefForInput("README.md", ""), {
    href: "README.md",
    resolved: false,
  });
  assert.deepEqual(helpers.resolveHrefForInput("", docsInput), {
    href: "",
    resolved: false,
  });
  assert.deepEqual(
    helpers.resolveHrefForInput("absent.md", path.join(repoRoot, "not-a-real-input.md")),
    { href: "absent/", resolved: false, unavailable: true },
  );
  assert.equal(helpers.documentationURLForRepositoryPath(path.join(repoRoot, "internal")), null);
  assert.equal(helpers.documentationURLForRepositoryPath(path.join(repoRoot, "docs", "missing")), null);
  assert.equal(helpers.documentationURLForRepositoryPath(path.resolve(repoRoot, "..")), null);
  assert.equal(helpers.isPublicHomepageRoot(path.join(repoRoot, "missing")), false);
  assert.equal(helpers.githubRepositoryHref(path.resolve(repoRoot, "..")), null);
});

test("folder, tree, and breadcrumb helpers expose rendered hierarchy", () => {
  const folderPages = helpers.buildFolderPages(path.join(repoRoot, "data"), "data");
  assert(folderPages.some((page) => page.url === "/data/explore/kirk0-lifecycle/"));

  const pages = [
    { url: "/docs/", data: { title: "Docs" } },
    { url: "/docs/platform/", data: { title: "Platform" } },
    { url: "/data/", data: { title: "Data" } },
    { url: "/alpha/", data: { title: "Alpha" } },
    { url: "/beta/", data: { title: "Beta" } },
  ];
  const tree = helpers.buildDocsTree(pages, "/docs/platform/");
  assert.equal(tree.find((node) => node.name === "Data").url, "/data/");
  assert.equal(tree.find((node) => node.name === "Docs").hasCurrent, true);
  assert.deepEqual(helpers.buildBreadcrumbs("/docs/platform/", pages), [
    { name: "Offline docs", url: "/", current: false },
    { name: "Docs", url: "/docs/", current: false },
    { name: "Platform", url: null, current: true },
  ]);
  assert.deepEqual(helpers.buildBreadcrumbs("/", pages), [
    { name: "Offline docs", url: null, current: true },
  ]);
});

test("post-build hook copies mermaid chunks and excludes paper PDFs", () => {
  const events = new Map();
  const noop = () => {};
  configure({
    setUseGitIgnore: noop,
    setLibrary: noop,
    addGlobalData: noop,
    addPassthroughCopy: noop,
    addWatchTarget: noop,
    addCollection: noop,
    addFilter: noop,
    addTransform: noop,
    on: (name, callback) => events.set(name, callback),
  });

  const originals = Object.fromEntries(
    ["writeFileSync", "existsSync", "mkdirSync", "readdirSync", "copyFileSync", "rmSync"].map(
      (name) => [name, fs[name]],
    ),
  );
  const calls = [];
  try {
    fs.writeFileSync = (target, contents) => calls.push(["write", target, contents]);
    fs.existsSync = (target) => target.includes("mermaid.esm.min") || target.includes("papers");
    fs.mkdirSync = (target) => calls.push(["mkdir", target]);
    fs.readdirSync = (target) =>
      target.includes("mermaid.esm.min") ? ["chunk.mjs", "skip.map"] : ["paper.pdf", "notes.txt"];
    fs.copyFileSync = (source, destination) => calls.push(["copy", source, destination]);
    fs.rmSync = (target) => calls.push(["remove", target]);
    events.get("eleventy.after")();
  } finally {
    Object.assign(fs, originals);
  }
  assert.equal(calls.filter(([kind]) => kind === "write").length, 1);
  assert.equal(calls.filter(([kind]) => kind === "copy").length, 1);
  assert.equal(calls.filter(([kind]) => kind === "remove").length, 1);
});

test("the transform marks internal docs, externalises source, and removes placeholders", () => {
  const registrations = new Map();
  const globals = new Map();
  const collections = new Map();
  const events = new Map();
  let markdown;
  const noop = () => {};
  const config = {
    setUseGitIgnore: noop,
    setLibrary: (_name, value) => (markdown = value),
    addGlobalData: (name, value) => globals.set(name, value),
    addPassthroughCopy: noop,
    addWatchTarget: noop,
    addCollection: (name, value) => collections.set(name, value),
    addFilter: (name, value) => registrations.set(name, value),
    addTransform: (name, value) => registrations.set(name, value),
    on: (name, value) => events.set(name, value),
  };
  configure(config);
  assert.match(markdown.render("```mermaid\ngraph TD\n```"), /pre class="mermaid"/);
  assert.match(markdown.render("```js\nconst x = 1;\n```"), /<code/);
  const permalink = globals.get("eleventyComputed").permalink;
  assert.equal(permalink({ page: { inputPath: path.join(docsRoot, "src/docs/ui/DESIGN.md") } }), "docs/ui/design-document/index.html");
  assert.equal(permalink({ page: { inputPath: path.join(docsRoot, "src/README.md") } }), "README/index.html");
  assert.equal(permalink({ page: { inputPath: path.join(docsRoot, "src/docs/platform/README.md") } }), "docs/platform/index.html");
  assert.equal(permalink({ permalink: "unchanged", page: { inputPath: path.join(docsRoot, "src/docs/X.md") } }), "unchanged");
  assert.equal(collections.get("docsPages")({ getAll: () => [{ inputPath: "b.txt", url: "/b/" }, { inputPath: "a.md", url: "/z/" }, { inputPath: "b.md", url: "/a/" }] }).length, 2);
  assert(globals.get("folderPages")().length > 0);
  assert.equal(registrations.get("docsTitle")({ data: { title: "Named" } }), "Named");
  assert.equal(registrations.get("docsTitle")({ inputPath: "alpha-beta.md" }), "Alpha Beta");
  assert.equal(registrations.get("docsNavGroups")([{ inputPath: "src/docs/radar/a.md" }, { inputPath: "src/docs/radar/b.md" }]).length, 1);
  const rewrite = registrations.get("rewrite-md-hrefs");
  const rendered = rewrite.call(
    {
      inputPath: path.join(repoRoot, "docs/plans/qemu-headscale-dev-vm-plan.md"),
      page: { outputPath: "_site/docs/plans/qemu/index.html", url: "/docs/plans/qemu/" },
    },
    '<a href="../../docs/platform/operations/">docs</a> <a href="../../internal/lidar">source</a> <a href="linked-plan-name/">placeholder</a> <a href="../../public_html/">homepage</a>',
  );
  assert.match(rendered, /data-docs-internal="true"/);
  assert.match(rendered, /github\.com\/banshee-data\/velocity\.report\/tree\/main\/internal\/lidar/);
  assert.match(rendered, /<code>placeholder<\/code>/);
  assert.match(rendered, /href="\/public_html\/" data-docs-app-surface="true"/);
  assert.equal(rewrite.call({ page: { outputPath: "asset.json" } }, "raw"), "raw");
  assert(fs.existsSync(path.join(repoRoot, "docs")));
});
