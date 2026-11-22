// A full-city download inventory. Use encoded HTTP response bodies when the
// browser exposes them; keep build-time gzip estimates for unavailable timings.
// Counting one body per URL makes the total independent of cache hits, retries
// and tile unloading. It does not fetch resources to discover their sizes.
export function createDownloadSizeTracker({
  tiles,
  assets,
  tileBase,
  assetBase = tileBase,
  moduleBase = import.meta.url,
}) {
  const files = new Map();
  for (const tile of tiles) {
    files.set(new URL(`${tile.t}.json.gz`, tileBase).href, {
      bytes: tile.bytes,
      tile: tile.t,
      estimated: false,
    });
  }
  for (const asset of assets) {
    files.set(
      new URL(asset.url, asset.base === "tiles" ? assetBase : moduleBase).href,
      {
        bytes: asset.bytes,
        estimated: true,
      },
    );
  }
  return {
    observe(entries) {
      for (const entry of entries) {
        const file = files.get(entry.name);
        // transferSize includes headers and becomes zero on cache hits;
        // decodedBodySize measures expanded content. Neither is the payload size.
        if (
          file &&
          (!entry.responseStatus ||
            entry.responseStatus === 200 ||
            entry.responseStatus === 304) &&
          Number.isFinite(entry.encodedBodySize) &&
          entry.encodedBodySize > 0
        ) {
          file.bytes = entry.encodedBodySize;
          file.estimated = false;
        }
      }
    },
    sizes(loadedTokens) {
      const loaded = new Set(loadedTokens);
      let total = 0,
        tiles = 0,
        resident = 0,
        estimated = false;
      for (const file of files.values()) {
        total += file.bytes;
        estimated ||= file.estimated;
        if (file.tile) {
          tiles += file.bytes;
          if (loaded.has(file.tile)) resident += file.bytes;
        }
      }
      return { total, tiles, resident, estimated };
    },
  };
}
