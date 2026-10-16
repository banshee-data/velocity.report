/**
 * Names of a capture's files in the published dataset.
 *
 * One rule, used both by the script that stages the dataset and by the site
 * that links to it, so the two cannot drift apart: a file is
 *
 *   raw/lidar/<YYYYMMDDTHHMM>_<S2 L13 display>_<site id>.pcapng
 *
 * The timestamp is the site's start in America/Los_Angeles, the L13 cell is
 * derived from the site's field-map position, and the id is the archive site
 * id. See huggingface-dataset-layout.md.
 */
import { s2 } from "s2js";

export const DATASET_REPO = "banshee-data/sf-street-speeds";
export const DATASET_URL = `https://huggingface.co/datasets/${DATASET_REPO}`;
export const TIMEZONE = "America/Los_Angeles";

/** The human-facing 5+3 form of an S2 token, e.g. 80858-0ac. */
export function tokenDisplay(token) {
  return `${token.slice(0, 5)}-${token.slice(5)}`;
}

/** A capture start as the filename timestamp, YYYYMMDDTHHMM in site-local time. */
export function timestampParts(timestamp) {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-CA", {
      timeZone: TIMEZONE,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    })
      .formatToParts(new Date(timestamp))
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value]),
  );
  return `${parts.year}${parts.month}${parts.day}T${parts.hour}${parts.minute}`;
}

/** S2 tokens for a site's position, at the levels the dataset names. */
export function siteCells(site) {
  const leaf = s2.cellid.fromLatLng(s2.LatLng.fromDegrees(site.lat, site.lon));
  return {
    l10: s2.cellid.toToken(s2.cellid.parent(leaf, 10)),
    l13: s2.cellid.toToken(s2.cellid.parent(leaf, 13)),
  };
}

/**
 * The stem every file of an archive site's capture shares, or null for a site
 * with no position: the stem carries its cell, so it cannot be named yet.
 */
export function captureStem(site) {
  if (site.lat == null || site.lon == null || !site.start) return null;
  const { l13 } = siteCells(site);
  return `${timestampParts(site.start)}_${tokenDisplay(l13)}_${site.id}`;
}

/** Dataset-relative path of the capture's raw LiDAR file. */
export function rawLidarPath(site) {
  const stem = captureStem(site);
  return stem ? `raw/lidar/${stem}.pcapng` : null;
}
