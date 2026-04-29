#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const { createRequire } = require("module");

const repoRoot = path.resolve(__dirname, "..");
const args = process.argv.slice(2);
const strictAnchors = args.includes("--strict-anchors");
const positional = args.find((arg) => !arg.startsWith("-"));
const defaultSiteRoot = path.join(repoRoot, "docs_html/_site");
const siteRoot = path.resolve(positional || defaultSiteRoot);
const explicitSiteRoot = Boolean(positional);

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(full, out);
    } else if (entry.isFile() && entry.name.endsWith(".html")) {
      out.push(full);
    }
  }
  return out;
}

function isExternal(href) {
  return !href || href.startsWith("//") || /^[a-z][a-z0-9+.-]*:/i.test(href);
}

function pageURLForFile(file) {
  const rel = path.relative(siteRoot, file).split(path.sep).join("/");
  if (rel.endsWith("/index.html") || rel === "index.html") {
    return `/${rel.replace(/(^|\/)index\.html$/, "$1")}`;
  }
  return `/${rel}`;
}

function decodeURLComponent(value) {
  try {
    return { value: decodeURIComponent(value) };
  } catch (error) {
    return { error };
  }
}

function fileForURLPath(urlPathname) {
  const decodedPath = decodeURLComponent(urlPathname);
  if (decodedPath.error) {
    return {
      error: `malformed percent-encoding in path ${urlPathname}: ${decodedPath.error.message}`,
    };
  }

  const decoded = decodedPath.value;
  const relative = decoded.replace(/^\/+/, "");
  const candidate = path.resolve(siteRoot, relative);
  const relativeToSite = path.relative(siteRoot, candidate);
  if (relativeToSite.startsWith("..") || path.isAbsolute(relativeToSite)) {
    return { file: null };
  }

  if (decoded.endsWith("/")) {
    return { file: path.join(candidate, "index.html") };
  }
  if (fs.existsSync(candidate) && fs.statSync(candidate).isDirectory()) {
    return { file: path.join(candidate, "index.html") };
  }
  if (fs.existsSync(candidate)) {
    return { file: candidate };
  }
  const indexCandidate = path.join(candidate, "index.html");
  if (fs.existsSync(indexCandidate)) {
    return { file: indexCandidate };
  }
  return { file: candidate };
}

const anchorCache = new Map();

function anchorsForFile(file) {
  if (anchorCache.has(file)) return anchorCache.get(file);
  if (!fs.existsSync(file) || !file.endsWith(".html")) {
    anchorCache.set(file, null);
    return null;
  }

  const html = fs.readFileSync(file, "utf8");
  const $ = cheerio.load(html);
  const anchors = new Set();
  $("[id]").each((_, element) => anchors.add($(element).attr("id")));
  $("[name]").each((_, element) => anchors.add($(element).attr("name")));
  anchorCache.set(file, anchors);
  return anchors;
}

function targetHasAnchor(file, anchor) {
  if (!anchor) return true;
  const anchors = anchorsForFile(file);
  return Boolean(anchors && anchors.has(anchor));
}

if (!fs.existsSync(siteRoot)) {
  if (!explicitSiteRoot && siteRoot === path.resolve(defaultSiteRoot)) {
    console.log(
      `Offline docs site not found: ${siteRoot}; skipping link check.`,
    );
    process.exit(0);
  }
  console.error(`Offline docs site not found: ${siteRoot}`);
  process.exit(1);
}

const errors = [];
const warnings = [];
const htmlFiles = walk(siteRoot);

// If the default site contains only the tracked embed marker, offline docs have
// not been built yet. Skip before loading docs_html dependencies so `make lint`
// remains non-mutating on clean checkouts.
if (!explicitSiteRoot && htmlFiles.length === 0) {
  console.log("Offline docs site contains no HTML files; skipping link check.");
  process.exit(0);
}

// If the site only contains the embed stub (i.e. offline docs were never built
// but a Make/CI stub script has run), skip the link check rather than failing.
// `make build-docs-offline` produces a real site.
const stubSource = path.resolve(repoRoot, "docs_html/stub-index.html");
const stubTarget = path.join(siteRoot, "index.html");
if (
  htmlFiles.length === 1 &&
  htmlFiles[0] === stubTarget &&
  fs.existsSync(stubSource) &&
  fs.readFileSync(stubTarget, "utf8") === fs.readFileSync(stubSource, "utf8")
) {
  console.log(
    "Offline docs site contains only the embed stub; skipping link check.",
  );
  process.exit(0);
}

// Real site: load cheerio from the docs_html dependency tree. Deferred until
// here so the stub-skip path doesn't require `make install-docs-offline`.
const docsRequire = createRequire(
  path.join(repoRoot, "docs_html/package.json"),
);
const cheerio = docsRequire("cheerio");

for (const file of htmlFiles) {
  const html = fs.readFileSync(file, "utf8");
  const $ = cheerio.load(html);
  const pageURL = pageURLForFile(file);
  const base = new URL(pageURL, "http://offline.local");

  $("a[href]").each((_, element) => {
    const href = $(element).attr("href");
    if (!href) return;

    if (/^https:\/\/github\.com\/.+\/blob\//i.test(href)) {
      warnings.push(
        `${path.relative(siteRoot, file)}: GitHub blob URL remains external: ${href}`,
      );
    }

    if (isExternal(href)) return;
    if (!href.startsWith("/") && !href.startsWith("#")) {
      if (/\.md(?:$|[?#])/i.test(href)) {
        warnings.push(
          `${path.relative(siteRoot, file)}: unresolved Markdown-style href was not rewritten: ${href}`,
        );
      }
      return;
    }

    let resolved;
    try {
      resolved = new URL(href, base);
    } catch (error) {
      errors.push(
        `${path.relative(siteRoot, file)}: invalid href ${href}: ${error.message}`,
      );
      return;
    }

    const target = fileForURLPath(resolved.pathname);
    if (target.error) {
      errors.push(
        `${path.relative(siteRoot, file)}: invalid href ${href}: ${target.error}`,
      );
      return;
    }

    const targetFile = target.file;
    if (!targetFile || !fs.existsSync(targetFile)) {
      errors.push(
        `${path.relative(siteRoot, file)}: unresolved link ${href} -> ${resolved.pathname}`,
      );
      return;
    }

    const rawAnchor = resolved.hash.replace(/^#/, "");
    const decodedAnchor = decodeURLComponent(rawAnchor);
    if (decodedAnchor.error) {
      errors.push(
        `${path.relative(siteRoot, file)}: invalid href ${href}: malformed percent-encoding in anchor ${rawAnchor}: ${decodedAnchor.error.message}`,
      );
      return;
    }

    const anchor = decodedAnchor.value;
    if (/^L\d+(?:-L\d+)?$/.test(anchor)) return;
    if (anchor && !targetHasAnchor(targetFile, anchor)) {
      const message = `${path.relative(siteRoot, file)}: missing anchor ${href} -> ${path.relative(
        siteRoot,
        targetFile,
      )}#${anchor}`;
      if (strictAnchors) {
        errors.push(message);
      } else {
        warnings.push(message);
      }
    }
  });
}

for (const warning of warnings) {
  console.warn(`warning: ${warning}`);
}

if (errors.length > 0) {
  console.error("Offline docs link check failed:");
  for (const error of errors) {
    console.error(`  - ${error}`);
  }
  process.exit(1);
}

console.log(
  `Offline docs links OK (${htmlFiles.length} HTML file(s) checked).`,
);
