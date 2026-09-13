// Builds a 5-column contact sheet from already-captured views. Rendered as
// an HTML grid and screenshotted with the same browser page already driving
// the capture, rather than pulling in an image-compositing library for a
// problem a browser's own layout engine already solves.

export const CONTACT_SHEET_COLUMNS = 5;

const THUMB_WIDTH = 240;
const CAPTION_HEIGHT = 22;
const GAP = 6;

function escapeHtml(value) {
  return String(value).replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
}

/**
 * @param {{fileName: string, caption: string}[]} items one per view, in
 *   recipe order — fileName is resolved relative to this HTML file, so it
 *   must live alongside the PNGs (the output directory).
 * @param {number} viewportWidth the recipe's capture viewport width, used to
 *   size thumbnails at the same aspect ratio as the full images.
 * @param {number} viewportHeight
 */
export function buildContactSheetHtml(items, viewportWidth, viewportHeight) {
  const thumbHeight = Math.round(
    (THUMB_WIDTH * viewportHeight) / viewportWidth,
  );
  const cells = items
    .map(
      ({ fileName, caption }) => `
      <figure>
        <img src="${escapeHtml(fileName)}" width="${THUMB_WIDTH}" height="${thumbHeight}">
        <figcaption>${escapeHtml(caption)}</figcaption>
      </figure>`,
    )
    .join("\n");
  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>Contact sheet</title>
<style>
  html, body { margin: 0; background: #07090c; }
  .grid {
    display: grid;
    grid-template-columns: repeat(${CONTACT_SHEET_COLUMNS}, ${THUMB_WIDTH}px);
    gap: ${GAP}px;
  }
  figure { margin: 0; }
  img { display: block; width: ${THUMB_WIDTH}px; height: ${thumbHeight}px; object-fit: cover; }
  figcaption {
    height: ${CAPTION_HEIGHT}px;
    line-height: ${CAPTION_HEIGHT}px;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
    color: #cfe3e8;
    font: 11px ui-monospace, monospace;
    padding: 0 2px;
  }
</style>
</head>
<body>
<div class="grid">${cells}</div>
</body>
</html>
`;
}

/** The exact pixel size buildContactSheetHtml's grid renders at. */
export function contactSheetSize(itemCount, viewportWidth, viewportHeight) {
  const thumbHeight = Math.round(
    (THUMB_WIDTH * viewportHeight) / viewportWidth,
  );
  const rows = Math.ceil(itemCount / CONTACT_SHEET_COLUMNS);
  const width =
    CONTACT_SHEET_COLUMNS * THUMB_WIDTH + (CONTACT_SHEET_COLUMNS - 1) * GAP;
  const height = rows * (thumbHeight + CAPTION_HEIGHT) + (rows - 1) * GAP;
  return { width, height };
}

/**
 * Renders the contact sheet HTML (already written to `htmlUrl`, alongside
 * the view PNGs it references) to a PNG at `outputPath`.
 */
export async function renderContactSheet(
  page,
  htmlUrl,
  itemCount,
  viewportWidth,
  viewportHeight,
  outputPath,
) {
  const size = contactSheetSize(itemCount, viewportWidth, viewportHeight);
  await page.setViewportSize(size);
  await page.goto(htmlUrl);
  await page.waitForFunction(() =>
    Array.from(document.images).every((img) => img.complete),
  );
  await page.screenshot({ path: outputPath });
}
