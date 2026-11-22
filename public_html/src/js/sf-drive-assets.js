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

async function verifiedBytes(url, info, signal) {
  if (!info || !sha256Pattern.test(info.sha256))
    throw new Error(`Missing checksum for ${url.pathname}.`);
  const bytes = await readBytes(url, signal);
  if (
    (info.bytes !== undefined && bytes.byteLength !== info.bytes) ||
    (await sha256(bytes)) !== info.sha256
  )
    throw new Error(`${info.label || url.pathname} failed verification.`);
  return bytes;
}

async function openRoadOverlay(url, baseUrl, signal, info, baseInput) {
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
    info ? verifiedBytes(url, info, signal) : readBytes(url, signal),
    baseInput || readBytes(baseUrl, signal),
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
  for (const [token, info] of Object.entries(overlay.tiles)) {
    if (
      !/^[0-9a-f]{7}[4c]$/.test(token) ||
      !sha256Pattern.test(info.sha256) ||
      !Number.isSafeInteger(info.bytes) ||
      info.bytes <= 0
    ) {
      throw new Error("The overlay manifest has an invalid tile entry.");
    }
  }
  return {
    overlay,
    base,
    async loadOwner(token, requestSignal) {
      const bytes = await verifiedBytes(
        new URL(`${token}.json`, url),
        { ...overlay.tiles[token], label: `Tile ${token}` },
        requestSignal,
      );
      return JSON.parse(decoder.decode(bytes));
    },
  };
}

export async function loadRoadOverlay(url, baseUrl, signal) {
  const result = await openRoadOverlay(url, baseUrl, signal);
  const tiles = new Map();
  for (const token of Object.keys(result.overlay.tiles))
    tiles.set(token, await result.loadOwner(token, signal));
  return { ...result, tiles };
}

export async function openRoadRegion(url, region, baseUrl, signal) {
  const decoder = new TextDecoder();
  const [catalogBytes, baseBytes] = await Promise.all([
    readBytes(url, signal),
    readBytes(baseUrl, signal),
  ]);
  const catalog = JSON.parse(decoder.decode(catalogBytes));
  if (
    !globalThis.crypto?.subtle ||
    catalog.state !== "regional_review" ||
    (await sha256(baseBytes)) !== catalog.base_manifest_sha256
  )
    throw new Error(
      "The regional catalog does not match the SF game manifest.",
    );
  const entry = catalog.regions.find(
    (record) => record.region === region && record.state === "review",
  );
  if (!entry)
    throw new Error(`Region ${region} is not in this review catalog.`);
  const manifestUrl = new URL(entry.manifest_url, url);
  if (manifestUrl.origin !== url.origin)
    throw new Error("The region must be hosted by the catalog server.");
  const manifest = JSON.parse(
    decoder.decode(await verifiedBytes(manifestUrl, entry, signal)),
  );
  if (
    manifest.kind !== "gerardus-sf-road-regional-review" ||
    manifest.compatibility_version !== 1 ||
    manifest.state !== "review" ||
    manifest.region !== entry.region ||
    manifest.revision !== entry.revision ||
    manifest.base_manifest_sha256 !== catalog.base_manifest_sha256 ||
    manifest.geometry_manifest !== "geometry/manifest.json"
  )
    throw new Error(
      "The regional review manifest does not match its catalog entry.",
    );
  const geometryFile = manifest.files[manifest.geometry_manifest];
  if (!geometryFile)
    throw new Error("The regional geometry checksum is missing.");
  const [geometry, reviewBytes] = await Promise.all([
    openRoadOverlay(
      new URL(manifest.geometry_manifest, manifestUrl),
      baseUrl,
      signal,
      geometryFile,
      baseBytes,
    ),
    verifiedBytes(
      new URL("review.json", manifestUrl),
      manifest.files["review.json"],
      signal,
    ),
  ]);
  for (const [token, info] of Object.entries(geometry.overlay.tiles)) {
    const copy = manifest.files[`geometry/${token}.json`];
    if (copy?.sha256 !== info.sha256 || copy.bytes !== info.bytes)
      throw new Error(`Tile ${token} does not match the regional manifest.`);
  }
  return {
    ...geometry,
    catalog,
    entry,
    review: JSON.parse(decoder.decode(reviewBytes)),
  };
}
