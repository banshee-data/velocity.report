// A minimal local static file server exposing several independent
// directories by URL prefix: the recipe's export, the shared player
// modules, the vendored three.js build, this tool's harness page, and the
// output directory (once a contact sheet needs to reference the PNGs it
// just wrote). No framework — this never needs to be anything but a file
// server with a handful of fixed roots.

import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";

const CONTENT_TYPES = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".png": "image/png",
  ".gz": "application/octet-stream",
};

function contentTypeFor(filePath) {
  return CONTENT_TYPES[path.extname(filePath)] ?? "application/octet-stream";
}

/** Resolves urlPath under root, refusing a path that would escape it. */
function safeJoin(root, urlPath) {
  const resolved = path.normalize(path.join(root, urlPath));
  const rootWithSep = root.endsWith(path.sep) ? root : root + path.sep;
  if (resolved !== root && !resolved.startsWith(rootWithSep)) return null;
  return resolved;
}

/**
 * Starts the server. `mounts` is an ordered list of `{prefix, root}`; the
 * first prefix a request path starts with owns that request outright
 * (found or not) — list more specific prefixes before `/`.
 */
export function startServer(mounts) {
  const server = createServer(async (req, res) => {
    let pathname;
    try {
      pathname = decodeURIComponent(
        new URL(req.url, "http://localhost").pathname,
      );
    } catch {
      res.writeHead(400).end("Bad Request");
      return;
    }

    for (const mount of mounts) {
      if (!pathname.startsWith(mount.prefix)) continue;
      let rel = pathname.slice(mount.prefix.length);
      if (rel === "" || rel.endsWith("/")) rel += "index.html";
      const filePath = safeJoin(mount.root, rel);
      if (!filePath) {
        res.writeHead(403).end("Forbidden");
        return;
      }
      try {
        const body = await readFile(filePath);
        res.writeHead(200, { "Content-Type": contentTypeFor(filePath) });
        res.end(body);
      } catch {
        res.writeHead(404).end("Not Found");
      }
      return;
    }
    res.writeHead(404).end("Not Found");
  });

  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      resolve({
        url: `http://127.0.0.1:${port}`,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}
