// Build metadata for the docs footer, set by `make build-docs-offline`.
// commitTime is HEAD's committer date, not the wall clock: the site is
// embedded in the binary, and a wall-clock stamp would make two builds of
// one commit differ.
module.exports = {
  version: process.env.VELOCITY_DOCS_VERSION || "dev",
  gitSHA: process.env.VELOCITY_DOCS_GIT_SHA || "unknown",
  gitShort: (process.env.VELOCITY_DOCS_GIT_SHA || "unknown").slice(0, 12),
  commitTime: process.env.VELOCITY_DOCS_COMMIT_TIME || "unknown",
};
