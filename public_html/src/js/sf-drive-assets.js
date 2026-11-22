export async function fetchJsonGz(url, options) {
  const res = await fetch(url, options);
  if (!res.ok) throw new Error(`${url}: ${res.status}`);
  const raw = await res.arrayBuffer();
  const head = new Uint8Array(raw, 0, 2);
  let text;
  if (head[0] === 0x1f && head[1] === 0x8b) {
    const stream = new Blob([raw])
      .stream()
      .pipeThrough(new DecompressionStream("gzip"));
    text = await new Response(stream).text();
  } else {
    text = new TextDecoder().decode(raw); // a host that already decoded the gzip
  }
  return JSON.parse(text);
}

const sha256Pattern = /^[0-9a-f]{64}$/;

async function readBytes(url, signal) {
  const response = await fetch(url, { signal });
  if (!response.ok)
    throw new Error(`${url.pathname} returned ${response.status}.`);
  return response.arrayBuffer();
}

async function sha256(bytes) {
  const hash = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(hash), (b) =>
    b.toString(16).padStart(2, "0"),
  ).join("");
}

export async function readGameManifest(url, expectedHash, signal) {
  const bytes = await readBytes(url, signal);
  if (!globalThis.crypto?.subtle || (await sha256(bytes)) !== expectedHash)
    throw new Error(
      "This tile server needs a manifest matching the game's bundled routes.",
    );
  return JSON.parse(new TextDecoder().decode(bytes));
}

export async function loadRoadOverlay(url, baseUrl, signal) {
  if (!["http:", "https:"].includes(url.protocol)) {
    throw new Error("Enter an HTTP or HTTPS manifest URL.");
  }
  if (!globalThis.crypto?.subtle) {
    throw new Error(
      "Open this viewer on localhost or HTTPS to verify the overlay.",
    );
  }
  const decoder = new TextDecoder();
  const [overlayBytes, baseBytes] = await Promise.all([
    readBytes(url, signal),
    readBytes(baseUrl, signal),
  ]);
  const overlay = JSON.parse(decoder.decode(overlayBytes));
  if (overlay.compatibility_version !== 1) {
    throw new Error("This road overlay uses an unsupported format.");
  }
  const base = JSON.parse(decoder.decode(baseBytes));
  if (
    (await sha256(baseBytes)) !== overlay.base_manifest_sha256 ||
    overlay.horizontal_crs !== "EPSG:7131" ||
    overlay.vertical_crs !== "EPSG:5703" ||
    overlay.units !== "m" ||
    overlay.origin_epsg7131?.length !== 2 ||
    !overlay.origin_epsg7131.every(
      (value, i) => value === base.origin_epsg7131[i],
    )
  ) {
    throw new Error(
      "This revision does not match the SF game manifest and frame.",
    );
  }
  const tiles = new Map();
  // Read each indexed owner once. Coverings refer back to these same records.
  for (const [token, info] of Object.entries(overlay.tiles)) {
    if (!/^[0-9a-f]{7}[4c]$/.test(token) || !sha256Pattern.test(info.sha256)) {
      throw new Error("The overlay manifest has an invalid tile entry.");
    }
    const bytes = await readBytes(new URL(`${token}.json`, url), signal);
    if (
      bytes.byteLength !== info.bytes ||
      (await sha256(bytes)) !== info.sha256
    ) {
      throw new Error(`Tile ${token} failed verification.`);
    }
    tiles.set(token, JSON.parse(decoder.decode(bytes)));
  }
  return { overlay, base, tiles };
}
