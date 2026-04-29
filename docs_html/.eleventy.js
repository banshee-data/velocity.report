const fs = require("fs");
const path = require("path");
const cheerio = require("cheerio");
const markdownIt = require("markdown-it");
const markdownItAnchor = require("markdown-it-anchor");
const markdownItTexmath = require("markdown-it-texmath");
const katex = require("katex");

function isExternalHref(href) {
  return (
    !href ||
    href.startsWith("#") ||
    href.startsWith("//") ||
    /^[a-z][a-z0-9+.-]*:/i.test(href)
  );
}

function rewriteMarkdownHref(href) {
  if (isExternalHref(href)) return href;

  const hashIndex = href.indexOf("#");
  const beforeHash = hashIndex === -1 ? href : href.slice(0, hashIndex);
  const hash = hashIndex === -1 ? "" : href.slice(hashIndex);
  const queryIndex = beforeHash.indexOf("?");
  const pathname =
    queryIndex === -1 ? beforeHash : beforeHash.slice(0, queryIndex);
  const query = queryIndex === -1 ? "" : beforeHash.slice(queryIndex);

  if (!pathname.toLowerCase().endsWith(".md")) return href;

  return `${pathname.slice(0, -3)}/${query}${hash}`;
}

function splitHref(href) {
  const hashIndex = href.indexOf("#");
  const beforeHash = hashIndex === -1 ? href : href.slice(0, hashIndex);
  const hash = hashIndex === -1 ? "" : href.slice(hashIndex);
  const queryIndex = beforeHash.indexOf("?");
  const pathname =
    queryIndex === -1 ? beforeHash : beforeHash.slice(0, queryIndex);
  const query = queryIndex === -1 ? "" : beforeHash.slice(queryIndex);
  return { pathname, query, hash };
}

function isWithin(parent, child) {
  const rel = path.relative(parent, child);
  return rel === "" || (!rel.startsWith("..") && !path.isAbsolute(rel));
}

function outputURLForSourcePath(inputRoot, targetPath) {
  const rel = path.relative(inputRoot, targetPath).replace(/\\/g, "/");
  if (!rel || rel.startsWith("../")) return null;
  if (rel === "README.md") return "/README/";
  if (rel.endsWith("/README.md")) {
    return `/${rel.slice(0, -"README.md".length)}`;
  }
  if (rel.toLowerCase().endsWith(".md")) {
    return `/${rel.slice(0, -3)}/`;
  }
  return `/${rel}`;
}

function rewriteHrefForInput(href, inputPath) {
  if (isExternalHref(href) || !inputPath) return href;

  const inputRoot = path.resolve("src");
  const sourceDir = path.dirname(path.resolve(inputPath));
  const { pathname, query, hash } = splitHref(href);
  if (!pathname) return href;

  const candidates = [];
  const direct = path.resolve(sourceDir, pathname);
  candidates.push(direct);
  if (!path.extname(pathname)) {
    candidates.push(`${direct}.md`);
  }
  candidates.push(path.join(direct, "README.md"));
  candidates.push(path.join(direct, "index.md"));

  for (const candidate of candidates) {
    if (!isWithin(inputRoot, candidate)) continue;
    try {
      if (!require("fs").statSync(candidate).isFile()) continue;
    } catch {
      continue;
    }
    const outputURL = outputURLForSourcePath(inputRoot, candidate);
    if (outputURL) return `${outputURL}${query}${hash}`;
  }

  return rewriteMarkdownHref(href);
}

function humanizePath(inputPath) {
  const parsed = path.parse(inputPath || "");
  const base = parsed.name || inputPath || "Untitled";
  return base
    .replace(/[-_]+/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function navGroup(inputPath) {
  const normalized = String(inputPath || "").replace(/\\/g, "/");
  if (normalized.includes("/docs/radar/")) return "Radar";
  if (normalized.includes("/docs/lidar/")) return "LiDAR";
  if (normalized.includes("/docs/platform/")) return "Platform";
  if (normalized.includes("/docs/ui/")) return "UI";
  if (normalized.includes("/docs/plans/")) return "Plans";
  if (normalized.includes("/data/structures/")) return "Data Structures";
  if (normalized.includes("/data/maths/")) return "Maths";
  if (normalized.includes("/data/")) return "Data";
  if (normalized.includes("/docs/")) return "Docs";
  return "Repository";
}

function humanizeSegment(segment) {
  if (!segment) return "";
  return String(segment)
    .replace(/[-_]+/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

// Build a hierarchical tree of pages keyed by URL segments. Each node has:
//   { name, segment, path, url, title, children, hasCurrent }
// A node's `url` is null when the segment has no corresponding page (a pure
// folder); the tree is otherwise navigable via the URLs alone.
function buildDocsTree(pages, currentUrl) {
  const root = { children: new Map() };
  for (const page of pages || []) {
    if (!page.url || page.url === "/") continue;
    const segments = page.url.split("/").filter(Boolean);
    if (!segments.length) continue;
    let node = root;
    let acc = "";
    for (let i = 0; i < segments.length; i++) {
      const segment = segments[i];
      acc += "/" + segment;
      if (!node.children.has(segment)) {
        node.children.set(segment, {
          segment,
          name: humanizeSegment(segment),
          path: acc,
          url: null,
          title: null,
          children: new Map(),
        });
      }
      node = node.children.get(segment);
      if (i === segments.length - 1) {
        node.url = page.url;
        node.title = page?.data?.title || node.name;
      }
    }
  }
  function finalize(node) {
    const list = [];
    for (const child of node.children.values()) {
      const finalized = finalize(child);
      list.push(finalized);
    }
    list.sort((a, b) => {
      const aFolder = a.children.length > 0 ? 0 : 1;
      const bFolder = b.children.length > 0 ? 0 : 1;
      if (aFolder !== bFolder) return aFolder - bFolder;
      return a.name.localeCompare(b.name);
    });
    const hasCurrent =
      (currentUrl && node.url === currentUrl) ||
      list.some((c) => c.hasCurrent);
    return {
      segment: node.segment,
      name: node.name,
      path: node.path,
      url: node.url,
      title: node.title || node.name,
      children: list,
      hasCurrent: !!hasCurrent,
    };
  }
  const finalizedRoot = finalize(root);
  return finalizedRoot.children;
}

// Breadcrumb trail for a given page URL. Each entry is `{name, url}`; the
// final entry has `url: null` (current page, not clickable). Intermediate
// segments are linked iff a page exists at that path.
function buildBreadcrumbs(currentUrl, pages) {
  const crumbs = [{ name: "Offline docs", url: "/", current: false }];
  if (!currentUrl || currentUrl === "/") {
    crumbs[0].current = true;
    crumbs[0].url = null;
    return crumbs;
  }
  const segments = currentUrl.split("/").filter(Boolean);
  const urlsByPath = new Set((pages || []).map((p) => p.url));
  let acc = "";
  for (let i = 0; i < segments.length; i++) {
    const segment = segments[i];
    acc += "/" + segment;
    const candidate = acc + "/";
    const isLast = i === segments.length - 1;
    const url = isLast ? null : urlsByPath.has(candidate) ? candidate : null;
    crumbs.push({
      name: humanizeSegment(segment),
      url,
      current: isLast,
    });
  }
  return crumbs;
}

function githubSlugify(value) {
  return String(value)
    .trim()
    .toLowerCase()
    .replace(/[<>]/g, "")
    .replace(/[^\p{Letter}\p{Number}_\- ]/gu, "")
    .replace(/ /g, "-");
}

module.exports = function (eleventyConfig) {
  eleventyConfig.setUseGitIgnore(false);

  const markdownLibrary = markdownIt({
    html: true,
    breaks: false,
    linkify: true,
    typographer: true,
  })
    .use(markdownItAnchor, {
      permalink: markdownItAnchor.permalink.ariaHidden({
        placement: "after",
        class: "header-anchor",
        symbol: "#",
        ariaHidden: false,
      }),
      level: [1, 2, 3, 4, 5, 6],
      slugify: githubSlugify,
    })
    .use(markdownItTexmath, {
      engine: katex,
      delimiters: "dollars",
      katexOptions: {
        // throwOnError must be false: a malformed equation in a single doc
        // page should produce a visible error in that one block, not break
        // the whole offline-docs build.
        throwOnError: false,
        strict: "ignore",
        trust: true,
      },
    });

  const defaultFence =
    markdownLibrary.renderer.rules.fence ||
    function (tokens, idx, options, env, self) {
      return self.renderToken(tokens, idx, options);
    };
  markdownLibrary.renderer.rules.fence = function (
    tokens,
    idx,
    options,
    env,
    self,
  ) {
    const token = tokens[idx];
    const language = token.info.trim().split(/\s+/)[0];
    if (language === "mermaid") {
      const escaped = markdownLibrary.utils.escapeHtml(token.content);
      return `<pre class="mermaid">${escaped}</pre>`;
    }
    return defaultFence(tokens, idx, options, env, self);
  };

  eleventyConfig.setLibrary("md", markdownLibrary);
  eleventyConfig.addGlobalData("layout", "base.njk");
  eleventyConfig.addGlobalData("eleventyComputed", {
    permalink: (data) => {
      const inputPath = data.page?.inputPath || "";
      if (!inputPath.endsWith("/README.md")) return data.permalink;

      const rel = path.relative(path.resolve("src"), path.resolve(inputPath));
      if (rel === "README.md") return "README/index.html";
      return `${rel.slice(0, -"README.md".length)}index.html`;
    },
  });

  eleventyConfig.addPassthroughCopy("src/assets");
  eleventyConfig.addPassthroughCopy({
    "node_modules/mermaid/dist/mermaid.esm.min.mjs":
      "assets/mermaid.esm.min.mjs",
    // KaTeX CSS references `fonts/<font>.woff2` etc. with relative URLs, so
    // place the fonts as a sibling of katex.min.css in `_site/assets/`.
    "node_modules/katex/dist/katex.min.css": "assets/katex.min.css",
    "node_modules/katex/dist/fonts": "assets/fonts",
  });

  // The mermaid ESM entry imports `./chunks/mermaid.esm.min/chunk-*.mjs` at
  // runtime, so the entry alone is not loadable in the browser. Copy each
  // sibling `.mjs` chunk into `_site/assets/chunks/mermaid.esm.min/` and skip
  // the matching `.map` files (~11 MB of sourcemaps that would inflate the
  // embedded binary for no runtime benefit).
  eleventyConfig.on("eleventy.after", () => {
    const chunkSrc = path.resolve(
      __dirname,
      "node_modules/mermaid/dist/chunks/mermaid.esm.min",
    );
    const chunkDest = path.resolve(
      __dirname,
      "_site/assets/chunks/mermaid.esm.min",
    );
    if (!fs.existsSync(chunkSrc)) return;
    fs.mkdirSync(chunkDest, { recursive: true });
    for (const entry of fs.readdirSync(chunkSrc)) {
      if (!entry.endsWith(".mjs")) continue;
      fs.copyFileSync(path.join(chunkSrc, entry), path.join(chunkDest, entry));
    }
  });
  eleventyConfig.addPassthroughCopy(
    "src/**/*.{png,jpg,jpeg,gif,svg,webp,pdf,json,yml,yaml,bib,txt,py,toml}",
  );

  for (const watchTarget of [
    "../docs",
    "../data",
    "../README.md",
    "../ARCHITECTURE.md",
    "../TENETS.md",
    "../CLAUDE.md",
  ]) {
    eleventyConfig.addWatchTarget(watchTarget);
  }

  eleventyConfig.addCollection("docsPages", (collectionApi) => {
    return collectionApi
      .getAll()
      .filter((item) => item.inputPath.endsWith(".md"))
      .sort((a, b) => a.url.localeCompare(b.url));
  });

  eleventyConfig.addFilter("docsTitle", (item) => {
    if (item?.data?.title) return item.data.title;
    return humanizePath(item?.inputPath || item?.page?.inputPath);
  });

  eleventyConfig.addFilter("docsNavGroups", (items) => {
    const groups = [];
    const byName = new Map();
    for (const item of items || []) {
      const groupName = navGroup(item.inputPath);
      if (!byName.has(groupName)) {
        const group = { name: groupName, items: [] };
        byName.set(groupName, group);
        groups.push(group);
      }
      byName.get(groupName).items.push(item);
    }
    return groups;
  });

  eleventyConfig.addFilter("docsTree", (items, currentUrl) =>
    buildDocsTree(items, currentUrl),
  );

  eleventyConfig.addFilter("breadcrumbs", (currentUrl, items) =>
    buildBreadcrumbs(currentUrl, items),
  );

  eleventyConfig.addFilter("titleFromPath", humanizePath);
  eleventyConfig.addFilter("navGroup", navGroup);

  eleventyConfig.addTransform("rewrite-md-hrefs", function (content) {
    if (!this.page.outputPath || !this.page.outputPath.endsWith(".html")) {
      return content;
    }
    const $ = cheerio.load(content, { decodeEntities: false });
    $("a[href]").each((_, element) => {
      const href = $(element).attr("href");
      $(element).attr("href", rewriteHrefForInput(href, this.inputPath));
    });
    return $.html();
  });

  return {
    dir: {
      input: "src",
      output: "_site",
      includes: "_includes",
      layouts: "_layouts",
      data: "_data",
    },
    templateFormats: ["html", "md", "njk"],
    htmlTemplateEngine: "njk",
    markdownTemplateEngine: false,
  };
};
