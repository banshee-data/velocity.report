const syntaxHighlight = require("@11ty/eleventy-plugin-syntaxhighlight");
const markdownIt = require("markdown-it");
const markdownItAnchor = require("markdown-it-anchor");
const cheerio = require("cheerio");

module.exports = function (eleventyConfig) {
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
    const img = defaultImageRender(tokens, idx, options, env, self);
    return `<a href="${src}" target="_blank" rel="noopener">${img}</a>`;
  };

  eleventyConfig.setLibrary("md", markdownLibrary);

  // Copy static files directly to output
  eleventyConfig.addPassthroughCopy({ "src/images": "img" });
  eleventyConfig.addPassthroughCopy("src/js");

  // Copy video files to output
  eleventyConfig.addPassthroughCopy("src/video");

  // Copy os-list JSON for Raspberry Pi Imager catalogue
  eleventyConfig.addPassthroughCopy({
    "../image/os-list-velocity.json": "rpi.json",
  });

  // Watch CSS source files for changes (triggers Eleventy rebuild)
  eleventyConfig.addWatchTarget("./src/css/");

  // Tell the dev server to reload when Tailwind writes compiled CSS to _site/
  eleventyConfig.setServerOptions({
    liveReload: true,
    domDiff: true,
    watch: ["_site/css/**"],
  });

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
    dir: {
      input: "src",
      output: "_site",
      includes: "_includes",
      layouts: "_layouts",
      data: "_data",
    },
    templateFormats: ["html", "md", "njk"],
    htmlTemplateEngine: "njk",
    markdownTemplateEngine: "njk",
  };
};
