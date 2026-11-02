const crypto = require("crypto");
const fs = require("fs");
const path = require("path");

/**
 * Names one build of the browser modules. Web archives keep a URL's first
 * capture when a later page save skips it, so a page archived after a change
 * can run beside modules from before it. Publishing each build under its own
 * directory keeps every captured page on the modules it was built with.
 *
 * @param {string} jsDir directory of the site's modules
 * @param {string} threeVersion version of the vendored three.js
 * @returns {string} ten hex digits, unchanged until a module or three.js is
 */
function moduleVersion(jsDir, threeVersion) {
  const hash = crypto.createHash("sha256").update(threeVersion);
  for (const file of fs.readdirSync(jsDir).sort()) {
    if (!file.endsWith(".js")) continue;
    hash.update(file).update(fs.readFileSync(path.join(jsDir, file)));
  }
  return hash.digest("hex").slice(0, 10);
}

module.exports = { moduleVersion };
