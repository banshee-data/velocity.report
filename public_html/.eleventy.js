const fs = require("fs");
const syntaxHighlight = require("@11ty/eleventy-plugin-syntaxhighlight");
const markdownIt = require("markdown-it");
const markdownItAnchor = require("markdown-it-anchor");
const cheerio = require("cheerio");

function normalisePathPrefix(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed || trimmed === "/") return "";
  return `/${trimmed.replace(/^\/+|\/+$/g, "")}/`;
}

const pathPrefix = normalisePathPrefix(
  process.env.VELOCITY_PUBLIC_HTML_PATH_PREFIX,
);
const OFFLINE_DOCS_REPO_PATH = /^(?:docs|data)\/.+/;
const OFFLINE_DOCS_ROOT_MARKDOWN = /^[^/]+\.md$/i;

function offlineDocsHref(href) {
  const match = href.match(
    /^https:\/\/github\.com\/banshee-data\/velocity\.report\/(blob|tree)\/main\/([^?#]+)([?#].*)?$/,
  );
  if (!match) return href;

  const [, kind, repoPath, suffix = ""] = match;
  const isOfflineDocsPath =
    OFFLINE_DOCS_REPO_PATH.test(repoPath) ||
    (kind === "blob" && OFFLINE_DOCS_ROOT_MARKDOWN.test(repoPath));
  if (!isOfflineDocsPath) return href;

  if (kind === "blob" && repoPath.toLowerCase().endsWith(".md")) {
    return `/docs/${repoPath.slice(0, -3)}/${suffix}`;
  }
  if (kind === "tree") return `/docs/${repoPath.replace(/\/+$/, "")}/${suffix}`;

  return href;
}

/**
 * Everything a survey template needs that depends on the site's speed limit:
 * findings, shares over the limit, and the charts at a wide and a narrow size
 * (CSS shows one), so SVG text stays legible at both.
 */
function surveyView(model, charts, site, rawLimit) {
  const limit = model.validLimit(rawLimit);
  const recs = site.recordings;
  const scale = charts.speedScale(
    recs.map((r) => r.speed),
    limit,
  );
  const ymax = charts.shareCeiling(recs.map((r) => r.speed));
  const recordings = recs.map((r, i) => {
    const speed = r.speed;
    const over = model.shareAtOrAbove(speed, limit);
    return {
      ...r,
      colour: charts.recordingColour(i, recs.length),
      isLatest: i === recs.length - 1,
      over,
      overText: model.pct(over),
      bands: model.excessBands(speed, limit),
      finding: model.speedFinding(speed, limit),
      charts: speed
        ? {
            wide: charts.histogramSvg(speed, {
              width: 1000,
              height: 250,
              limit,
              scale,
            }),
            narrow: charts.histogramSvg(speed, {
              width: 343,
              height: 200,
              limit,
              scale,
            }),
            strip: charts.percentileStripSvg(speed, { width: 360, limit }),
            stripNarrow: charts.percentileStripSvg(speed, {
              width: 311,
              limit,
            }),
            spark: charts.sparklineSvg(speed, { width: 90, height: 22, limit }),
          }
        : null,
    };
  });
  return {
    limit,
    recordings,
    latest: recordings.at(-1) ?? null,
    canCompare: recordings.filter((r) => r.speed).length >= 2,
    // What the browser needs for the recording switcher and compare view.
    client: {
      id: site.id,
      title: site.title,
      limit,
      scale,
      ymax,
      recordings: recordings.map(
        ({ charts: _c, finding: _f, bands: _b, ...r }) => r,
      ),
    },
  };
}

module.exports = async function (eleventyConfig) {
  // Survey pages render the same charts at build time and in the browser, so
  // the model and chart code are ES modules under src/js, imported here.
  const surveyModel = await import("./src/js/survey-model.js");
  const surveyCharts = await import("./src/js/survey-charts.js");

  // Add syntax highlighting plugin
  eleventyConfig.addPlugin(syntaxHighlight);

  // Configure markdown-it with plugins
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
    level: [1, 2, 3, 4],
    slugify: eleventyConfig.getFilter("slugify"),
  });

  // Wrap images in a link that opens the full-size image in a new tab
  const defaultImageRender =
    markdownLibrary.renderer.rules.image ||
    function (tokens, idx, options, env, self) {
      return self.renderToken(tokens, idx, options);
    };
  markdownLibrary.renderer.rules.image = function (
    tokens,
    idx,
    options,
    env,
    self,
  ) {
    const token = tokens[idx];
    const src = token.attrGet("src") || "";
    const escapedSrc = markdownLibrary.utils.escapeHtml(src);
    const img = defaultImageRender(tokens, idx, options, env, self);
    return `<a href="${escapedSrc}" target="_blank" rel="noopener noreferrer">${img}</a>`;
  };

  eleventyConfig.setLibrary("md", markdownLibrary);

  // The public site normally lives at the origin. The Pi image also serves it
  // under /public_html/, so rewrite root-relative HTML URLs only for that build.
  // CSS font URLs are relative (see fonts.css), and therefore work in both
  // locations without this transform.
  if (pathPrefix) {
    eleventyConfig.addTransform(
      "link-offline-repository-docs",
      function (content, outputPath) {
        if (!outputPath || !outputPath.endsWith(".html")) return content;
        return content.replace(
          /\bhref=(["'])(https:\/\/github\.com\/banshee-data\/velocity\.report\/(?:blob|tree)\/main\/[^"']+)\1/g,
          (whole, quote, href) => {
            const offlineHref = offlineDocsHref(href);
            return offlineHref === href
              ? whole
              : `href=${quote}${offlineHref}${quote}`;
          },
        );
      },
    );
    eleventyConfig.addTransform(
      "prefix-offline-root-urls",
      function (content, outputPath) {
        if (!outputPath || !outputPath.endsWith(".html")) return content;
        return content.replace(
          /\b(href|src)=(['"])\/(?!\/|docs\/)/g,
          `$1=$2${pathPrefix}`,
        );
      },
    );
  }

  // Copy static files directly to output
  eleventyConfig.addPassthroughCopy({ "src/images": "img" });
  eleventyConfig.addPassthroughCopy({ "src/geo": "img/maps" });
  eleventyConfig.addPassthroughCopy("src/js");
  // Scene assets are exported binaries (gzipped NDJSON frames plus their
  // index), not templates, so they are copied verbatim.
  // Published under /surveys/; the source directory keeps its internal name.
  for (const entry of fs.readdirSync("src/scenes", { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    if (!fs.existsSync(`src/scenes/${entry.name}/assets`)) continue;
    eleventyConfig.addPassthroughCopy({
      [`src/scenes/${entry.name}/assets`]: `surveys/${entry.name}/assets`,
    });
  }
  // Further recordings at a site sit beside the original one; see
  // src/_data/surveys.js for the layout.
  for (const entry of fs.readdirSync("src/scenes", { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    if (!fs.existsSync(`src/scenes/${entry.name}/recordings`)) continue;
    eleventyConfig.addPassthroughCopy({
      [`src/scenes/${entry.name}/recordings`]: `surveys/${entry.name}/recordings`,
    });
  }
  eleventyConfig.addPassthroughCopy({ "src/design": "design" });

  // Homepage uses a pure CSS file (not processed by Tailwind) — pass it through
  eleventyConfig.addPassthroughCopy({
    "src/css/homepage.css": "css/homepage.css",
  });
  eleventyConfig.addPassthroughCopy({ "src/css/survey.css": "css/survey.css" });
  eleventyConfig.addPassthroughCopy({ "src/css/header.css": "css/header.css" });
  eleventyConfig.addPassthroughCopy({ "src/css/footer.css": "css/footer.css" });
  eleventyConfig.addPassthroughCopy({ "src/css/fonts.css": "css/fonts.css" });
  // three.module.js re-exports from ./three.core.js (a sibling relative import),
  // so both files must be vendored or the browser 404s on three.core.js.
  eleventyConfig.addPassthroughCopy({
    "node_modules/three/build/three.module.js": "vendor/three/three.module.js",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/three/build/three.core.js": "vendor/three/three.core.js",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/geist/files/geist-latin-400-normal.woff2":
      "vendor/fonts/geist/geist-latin-400-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/geist/files/geist-latin-500-normal.woff2":
      "vendor/fonts/geist/geist-latin-500-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/geist/files/geist-latin-600-normal.woff2":
      "vendor/fonts/geist/geist-latin-600-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/geist/files/geist-latin-700-normal.woff2":
      "vendor/fonts/geist/geist-latin-700-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/geist/files/geist-latin-800-normal.woff2":
      "vendor/fonts/geist/geist-latin-800-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/jetbrains-mono/files/jetbrains-mono-latin-400-normal.woff2":
      "vendor/fonts/jetbrains-mono/jetbrains-mono-latin-400-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/jetbrains-mono/files/jetbrains-mono-latin-500-normal.woff2":
      "vendor/fonts/jetbrains-mono/jetbrains-mono-latin-500-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/jetbrains-mono/files/jetbrains-mono-latin-600-normal.woff2":
      "vendor/fonts/jetbrains-mono/jetbrains-mono-latin-600-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/newsreader/files/newsreader-latin-400-italic.woff2":
      "vendor/fonts/newsreader/newsreader-latin-400-italic.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/eb-garamond/files/eb-garamond-latin-400-normal.woff2":
      "vendor/fonts/eb-garamond/eb-garamond-latin-400-normal.woff2",
  });
  eleventyConfig.addPassthroughCopy({
    "node_modules/@fontsource/eb-garamond/files/eb-garamond-latin-500-normal.woff2":
      "vendor/fonts/eb-garamond/eb-garamond-latin-500-normal.woff2",
  });

  // Copy video files to output
  eleventyConfig.addPassthroughCopy("src/video");

  // Copy os-list JSON for Raspberry Pi Imager catalogue
  eleventyConfig.addPassthroughCopy({
    "../image/os-list-velocity.json": "rpi.json",
  });

  // Serve release metadata at /release.json for velocity-ctl and external consumers
  eleventyConfig.addPassthroughCopy({
    "src/_data/release.json": "release.json",
  });

  // Watch CSS source files for changes (triggers Eleventy rebuild)
  eleventyConfig.addWatchTarget("./src/css/");

  // Tell the dev server to reload when Tailwind writes compiled CSS to _site/
  eleventyConfig.setServerOptions({
    liveReload: true,
    domDiff: true,
    watch: ["_site/css/**"],
  });

  // Survey pages: the posted limit is set per site in front matter
  // (speedLimitMph), so the listing reads it from the pages themselves.
  eleventyConfig.addCollection("surveyLimits", function (collectionApi) {
    const limits = {};
    for (const item of collectionApi.getFilteredByGlob(
      "src/scenes/*/index.njk",
    )) {
      if (item.data.sceneId)
        limits[item.data.sceneId] = surveyModel.validLimit(
          item.data.speedLimitMph,
        );
    }
    return limits;
  });
  eleventyConfig.addFilter("surveyView", (site, limit) =>
    surveyView(surveyModel, surveyCharts, site, limit),
  );

  // Add collection for guides
  eleventyConfig.addCollection("guides", function (collectionApi) {
    return collectionApi.getFilteredByGlob("src/guides/**/*.md");
  });

  // Add collection for getting started pages
  eleventyConfig.addCollection("gettingStarted", function (collectionApi) {
    return collectionApi.getFilteredByGlob("src/getting-started/**/*.md");
  });

  // Add collection for reference docs
  eleventyConfig.addCollection("reference", function (collectionApi) {
    return collectionApi.getFilteredByGlob("src/reference/**/*.md");
  });

  // Add a custom filter for reading time estimation
  eleventyConfig.addFilter("readingTime", (content) => {
    const wordsPerMinute = 200;
    const numberOfWords = content.split(/\s/g).length;
    const minutes = Math.ceil(numberOfWords / wordsPerMinute);
    return minutes;
  });

  // Add a date filter for formatting dates
  eleventyConfig.addFilter("dateDisplay", (dateObj) => {
    return new Date(dateObj).toLocaleDateString("en-US", {
      year: "numeric",
      month: "long",
      day: "numeric",
    });
  });

  // Table of contents: extract h2 headings into a flat list
  eleventyConfig.addFilter("table_of_contents", (html) => {
    if (!html || typeof html !== "string") return [];

    const $ = cheerio.load(html, { decodeEntities: false }, false);
    const headings = $("h2").toArray();

    if (headings.length < 2) return [];

    const items = [];
    for (const heading of headings) {
      const text = $(heading).text().replace(/#$/, "").trim();
      const id = heading.attribs?.id;
      if (!text || !id) continue;
      items.push({ id, text });
    }

    return items;
  });

  // Split content at first <h2> — returns everything before it
  eleventyConfig.addFilter("content_preamble", (html) => {
    if (!html || typeof html !== "string") return html;
    const idx = html.search(/<h2[\s>]/i);
    return idx === -1 ? html : html.slice(0, idx);
  });

  // Split content at first <h2> — returns everything from it onward
  eleventyConfig.addFilter("content_body", (html) => {
    if (!html || typeof html !== "string") return "";
    const idx = html.search(/<h2[\s>]/i);
    return idx === -1 ? "" : html.slice(idx);
  });

  return {
    pathPrefix,
    dir: {
      input: "src",
      output: "_site",
      includes: "_includes",
      layouts: "_layouts",
      data: "_data",
    },
    templateFormats: ["md", "njk", "11ty.js"],
    htmlTemplateEngine: false,
    markdownTemplateEngine: "njk",
  };
};
