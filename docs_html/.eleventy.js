const fs = require("fs");
const path = require("path");
const cheerio = require("cheerio");
const markdownIt = require("markdown-it");
const markdownItAnchor = require("markdown-it-anchor");

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
  }).use(markdownItAnchor, {
    permalink: markdownItAnchor.permalink.ariaHidden({
      placement: "after",
      class: "header-anchor",
      symbol: "#",
      ariaHidden: false,
    }),
    level: [1, 2, 3, 4, 5, 6],
    slugify: githubSlugify,
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
