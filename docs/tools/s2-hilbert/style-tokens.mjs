/**
 * One source of truth for every visual token.
 *
 * Each class is emitted twice: as a CSS rule inside the SVG, and as
 * presentation attributes on the element itself. Design tools such as
 * Illustrator, Inkscape and Figma routinely ignore an embedded stylesheet, and
 * a path with no stroke and no fill declared then falls back to "filled black",
 * which turns a Hilbert curve into a solid blob. Presentation attributes render
 * correctly everywhere; CSS still wins where it is honoured, so a stylesheet
 * swap keeps working.
 */

export const HILBERT_TOKENS = {
  "s2-cell": { fill: "none", stroke: "#315a70", "stroke-width": "0.8", opacity: "0.42" },
  "s2-parent": { fill: "none", stroke: "#163747", "stroke-width": "2" },
  "hilbert-path": {
    fill: "none",
    stroke: "#006d77",
    "stroke-width": "5",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "hilbert-land": { opacity: "1" },
  "hilbert-water": { opacity: "0.3" },
  "hilbert-unclassified": { opacity: "1" },
  "hilbert-chevron": {
    fill: "none",
    stroke: "#023e4a",
    "stroke-width": "2.6",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "hilbert-start": { fill: "#fff", stroke: "#1b7f5a", "stroke-width": "4" },
  "hilbert-start-core": { fill: "#1b7f5a", stroke: "none" },
  "hilbert-end": { fill: "#d1495b", stroke: "#fff", "stroke-width": "2" },
  "endpoint-label": {
    "font-family": "system-ui, sans-serif",
    "font-size": "12",
    "font-weight": "700",
    "text-anchor": "middle",
    "dominant-baseline": "central",
    "paint-order": "stroke",
    stroke: "#fff",
    "stroke-width": "3.5",
    "stroke-linejoin": "round",
  },
  "endpoint-label-start": { fill: "#1b7f5a" },
  "endpoint-label-end": { fill: "#d1495b" },
  "s2-label": {
    fill: "#163747",
    "font-family": "ui-monospace, SFMono-Regular, Menlo, monospace",
    "font-size": "10",
    "text-anchor": "middle",
    "dominant-baseline": "central",
  },
};

export const COMPOSITE_TOKENS = {
  "s2-parent": { fill: "none", stroke: "#163747", "stroke-width": "2" },
  "tile-selected": {
    fill: "#006d77",
    "fill-opacity": "0.16",
    stroke: "#006d77",
    "stroke-width": "1.2",
    "stroke-opacity": "0.55",
  },
  "hilbert-detail": {
    fill: "none",
    stroke: "#006d77",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "hilbert-detail-light": { "stroke-width": "2.2", opacity: "0.3" },
  "hilbert-detail-heavy": { "stroke-width": "5", opacity: "1" },
  "hilbert-coarse": {
    fill: "none",
    stroke: "#8c5a2b",
    "stroke-width": "5.5",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "chain-connector": {
    fill: "none",
    stroke: "#8c5a2b",
    "stroke-width": "5.5",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "chain-stub": {
    fill: "none",
    stroke: "#8c5a2b",
    "stroke-width": "5.5",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "chain-detail": {
    fill: "none",
    stroke: "#006d77",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
    "stroke-width": "2.2",
    opacity: "0.3",
  },
  "composite-chevron": {
    fill: "none",
    stroke: "#5d3a18",
    "stroke-width": "2.2",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
};

export const LEGEND_TOKENS = {
  "legend-panel": { fill: "#f8fbfc", stroke: "#c7d7de", "stroke-width": "1" },
  "legend-parent": { fill: "none", stroke: "#315a70", "stroke-width": "1.5" },
  "legend-path": {
    fill: "none",
    stroke: "#006d77",
    "stroke-width": "4",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "legend-chevron": {
    fill: "none",
    stroke: "#023e4a",
    "stroke-width": "1.8",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  },
  "legend-start": { fill: "#fff", stroke: "#1b7f5a", "stroke-width": "3" },
  "legend-start-core": { fill: "#1b7f5a", stroke: "none" },
  "legend-end": { fill: "#d1495b", stroke: "#fff", "stroke-width": "1.5" },
  "legend-title": {
    fill: "#163747",
    "font-family": "system-ui, sans-serif",
    "font-size": "16",
    "font-weight": "600",
    "text-anchor": "middle",
  },
};

/** Render a token table as the CSS that goes inside the SVG's <style>. */
export function tokensToCss(tokens) {
  return Object.entries(tokens)
    .map(([className, declarations]) => {
      const body = Object.entries(declarations)
        .map(([property, value]) => `${property}: ${value};`)
        .join(" ");
      return `  .${className} { ${body} }`;
    })
    .join("\n");
}

/**
 * Presentation attributes for a space-separated class list. Later classes win,
 * matching the way the equivalent CSS rules cascade.
 */
export function tokensToAttributes(tokens, classList) {
  const merged = {};
  for (const className of classList.split(/\s+/).filter(Boolean)) {
    Object.assign(merged, tokens[className] ?? {});
  }
  return Object.entries(merged)
    .map(([property, value]) => ` ${property}="${value}"`)
    .join("");
}
