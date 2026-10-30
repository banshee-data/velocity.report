// The viewer's release is owned by the root Makefile. Recording headers
// describe the pipeline that made the measurements, not this site's viewer.
const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
module.exports = function () {
  const root = path.resolve(__dirname, "../../..");
  const match = fs
    .readFileSync(path.join(root, "Makefile"), "utf8")
    .match(/^VERSION\s*:=\s*(\S+)/m);
  if (!match) throw new Error("Viewer build has no Makefile VERSION");
  let gitSHA = process.env.GITHUB_SHA ?? null;
  if (!gitSHA) {
    try {
      gitSHA = execFileSync("git", ["-C", root, "rev-parse", "HEAD"], {
        encoding: "utf8",
        stdio: ["ignore", "pipe", "ignore"],
      }).trim();
    } catch {
      /* Source archives may have no Git metadata. */
    }
  }
  return { version: match[1], gitSHA };
};
