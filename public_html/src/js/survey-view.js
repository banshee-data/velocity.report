/**
 * Everything a survey template needs that depends on the site's speed limit:
 * findings, shares over the limit, and the charts at a wide and a narrow size
 * (CSS shows one), so SVG text stays legible at both. Used by the Eleventy
 * `surveyView` filter; kept here so it can be tested.
 */
import {
  excessBands,
  pct,
  shareAtOrAbove,
  speedFinding,
  validLimit,
} from "./survey-model.js";
import {
  histogramSvg,
  percentileStripSvg,
  recordingColour,
  shareCeiling,
  sparklineSvg,
  speedScale,
} from "./survey-charts.js";

export function surveyView(site, rawLimit) {
  if (!site) throw new Error("surveyView: the site is not in scenes.json");
  const limit = validLimit(rawLimit);
  const recs = site.recordings;
  const scale = speedScale(
    recs.map((r) => r.speed),
    limit,
  );
  const ymax = shareCeiling(recs.map((r) => r.speed));
  const recordings = recs.map((r, i) => {
    const speed = r.speed;
    const over = shareAtOrAbove(speed, limit);
    return {
      ...r,
      colour: recordingColour(i, recs.length),
      isLatest: i === recs.length - 1,
      over,
      overText: pct(over),
      bands: excessBands(speed, limit),
      finding: speedFinding(speed, limit),
      charts: speed
        ? {
            wide: histogramSvg(speed, {
              width: 1000,
              height: 250,
              limit,
              scale,
            }),
            narrow: histogramSvg(speed, {
              width: 343,
              height: 200,
              limit,
              scale,
            }),
            strip: percentileStripSvg(speed, { width: 360, limit }),
            stripNarrow: percentileStripSvg(speed, { width: 311, limit }),
            spark: sparklineSvg(speed, { width: 90, height: 22, limit }),
          }
        : null,
    };
  });
  return {
    limit,
    recordings,
    latest: recordings.at(-1) ?? null,
    canCompare: recordings.filter((r) => r.speed).length >= 2,
    // The 3D player and its point-cloud toggles are rendered when any
    // recording can use them; the page shows them per selected recording.
    hasPlayback: recordings.some((r) => r.hasPlayback),
    hasPointCloud: recordings.some((r) => r.hasPointCloud),
    // What the browser needs for the recording switcher and compare view.
    client: {
      id: site.id,
      title: site.title,
      limit,
      scale,
      ymax,
      recordings: recordings.map(
        ({ charts: _c, finding: _f, bands: _b, ...r }) => r,
      ),
    },
  };
}
