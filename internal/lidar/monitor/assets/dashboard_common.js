/* dashboard_common.js — shared utilities for LiDAR monitor dashboards. */

/**
 * Escape a value for safe insertion into HTML.
 * Converts &, <, >, ", and ' to their corresponding HTML entities.
 */
function escapeHTML(str) {
  if (str == null) return "";
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * Parse a Go-style duration string to seconds.
 * Examples: "5s" -> 5, "500ms" -> 0.5, "2m" -> 120, "1m30s" -> 90.
 */
function parseDuration(s) {
  if (!s) return 0;
  var total = 0;
  var re = /(\d+(?:\.\d+)?)(ms|s|m|h)/g;
  var match;
  while ((match = re.exec(s)) !== null) {
    var v = parseFloat(match[1]);
    switch (match[2]) {
      case "h":
        total += v * 3600;
        break;
      case "m":
        total += v * 60;
        break;
      case "s":
        total += v;
        break;
      case "ms":
        total += v / 1000;
        break;
    }
  }
  return total;
}

/**
 * Format a number of seconds as a human-readable duration string.
 * Examples: 5 -> "5s", 90 -> "1m 30s", 3600 -> "1h".
 */
function formatDuration(secs) {
  // Round once to the nearest whole second, then decompose using integer maths
  // so that seconds/minutes never reach 60.
  var total = Math.round(secs);
  if (total < 60) return total + "s";
  if (total < 3600) {
    var m = Math.floor(total / 60);
    var s = total % 60;
    return s > 0 ? m + "m " + s + "s" : m + "m";
  }
  var h = Math.floor(total / 3600);
  var rm = Math.floor((total % 3600) / 60);
  return rm > 0 ? h + "h " + rm + "m" : h + "h";
}

// ---- CommonJS exports for testing ----
if (typeof module !== "undefined" && module.exports) {
  module.exports = {
    escapeHTML: escapeHTML,
    parseDuration: parseDuration,
    formatDuration: formatDuration,
  };
}
