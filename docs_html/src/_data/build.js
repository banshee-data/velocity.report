module.exports = {
  version: process.env.VELOCITY_DOCS_VERSION || "dev",
  gitSHA: process.env.VELOCITY_DOCS_GIT_SHA || "unknown",
  gitShort: (process.env.VELOCITY_DOCS_GIT_SHA || "unknown").slice(0, 12),
  buildTime: process.env.VELOCITY_DOCS_BUILD_TIME || "unknown",
};
