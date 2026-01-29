const syntaxHighlight = require("@11ty/eleventy-plugin-syntaxhighlight");
const markdownIt = require("markdown-it");
const markdownItAnchor = require("markdown-it-anchor");

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

  eleventyConfig.setLibrary("md", markdownLibrary);

  // Copy static files directly to output
  eleventyConfig.addPassthroughCopy("src/images");
  eleventyConfig.addPassthroughCopy("src/js");

  // Copy images with img alias for backwards compatibility
  eleventyConfig.addPassthroughCopy({ "src/images": "img" });

  // Watch CSS files for changes
  eleventyConfig.addWatchTarget("./src/css/");

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
