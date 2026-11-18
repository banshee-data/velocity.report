// Package existing scene exports with the shared player. Raw canonical rows,
// source track keys and database paths never enter the browser bundle.
import {
  cp,
  mkdir,
  readFile,
  writeFile,
  readdir,
  copyFile,
} from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { gunzipSync } from "node:zlib";
import { execFileSync } from "node:child_process";

export function contextMatches(header, published, entry, vectors) {
  if (
    header.site !== entry.id ||
    header.sensor_id !== published.sensor_id ||
    header.coordinate_frame?.frame_id !==
      published.coordinate_frame?.frame_id ||
    header.coordinate_frame?.reference_frame !==
      published.coordinate_frame?.reference_frame ||
    vectors?.site !== entry.id ||
    vectors?.placement?.check?.passed !== true
  )
    return false;
  const start = BigInt(header.start_ns),
    end = start + BigInt(Math.round(header.duration_sec * 1e9));
  // The archive catalogue binds sensor placement to this capture interval.
  // Millisecond Date parsing needs a 2 ms rounding allowance, not a new pose.
  return (
    entry.static_parts?.some(
      (p) =>
        start >= BigInt(Date.parse(p.start)) * 1_000_000n - 2_000_000n &&
        end <= BigInt(Date.parse(p.end)) * 1_000_000n + 2_000_000n,
    ) ?? false
  );
}

export async function packageFollowingReview({
  corpus,
  output,
  sites,
  contextRoot,
  repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../.."),
}) {
  const complete = JSON.parse(
    await readFile(path.join(corpus, "complete.json"), "utf8"),
  );
  if (
    complete.schema !== "following_corpus_v1" ||
    sites.length !== 3 ||
    new Set(sites).size !== 3 ||
    sites.some((s) => !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(s))
  )
    throw Error("Select exactly three distinct development site slugs");
  const selected = sites.map((id) => {
    const matches = complete.recordings
      .map((r, i) => ({ r, i }))
      .filter((x) => x.r.metadata.binding.siteID === id);
    if (matches.length !== 1)
      throw Error(`Need exactly one completed recording for ${id}`);
    return { id, ...matches[0] };
  });
  await mkdir(output, { recursive: false });
  const catalog = {
    schema: "following_web_review_v1",
    viewer_git_sha: execFileSync("git", ["rev-parse", "HEAD"], {
      cwd: repo,
      encoding: "utf8",
    }).trim(),
    analysis_git_sha: complete.analysis_tool_git_sha,
    sites: [],
  };
  const archive = JSON.parse(
    await readFile(path.join(repo, "tools/s2-archive/site-index.json"), "utf8"),
  );
  await mkdir(path.join(output, "js"));
  for (const name of await readdir(path.join(repo, "public_html/src/js")))
    if (name.endsWith(".js"))
      await copyFile(
        path.join(repo, "public_html/src/js", name),
        path.join(output, "js", name),
      );
  await mkdir(path.join(output, "vendor/three"), { recursive: true });
  for (const name of ["three.module.js", "three.core.js"])
    await copyFile(
      path.join(repo, "public_html/node_modules/three/build", name),
      path.join(output, "vendor/three", name),
    );
  await copyFile(
    path.join(repo, "public_html/node_modules/three/LICENSE"),
    path.join(output, "vendor/three/LICENSE"),
  );
  await copyFile(
    path.join(repo, "tools/scene-capture/following-review.html"),
    path.join(output, "index.html"),
  );
  for (const { id, i, r } of selected) {
    const source = path.join(corpus, `recording-${String(i).padStart(3, "0")}`),
      dest = path.join(output, "sites", id);
    const inputHeader = JSON.parse(
      await readFile(path.join(source, "scene/header.json"), "utf8"),
    );
    if (
      inputHeader.site !== id ||
      !inputHeader.following?.summary ||
      inputHeader.following.evidence_sha256 !== r.metadata.evidence_sha256 ||
      inputHeader.following.calibration_id !== r.metadata.calibration_id
    )
      throw Error(
        `Following export is absent or mismatched for ${id}; re-export with the current corpus command`,
      );
    await mkdir(dest, { recursive: true });
    await copyFile(
      path.join(source, "manifest.json"),
      path.join(dest, "manifest.json"),
    );
    await cp(path.join(source, "scene"), path.join(dest, "scene"), {
      recursive: true,
      errorOnExist: true,
    });
    const context = path.join(
      contextRoot ?? path.join(repo, "public_html/src/scenes"),
      id,
      "assets",
    );
    let lanes = 0,
      proposedLanes = 0;
    try {
      const vectors = JSON.parse(
        gunzipSync(
          await readFile(path.join(context, "vectors/vectors.geojson.gz")),
        ),
      );
      const header = JSON.parse(
        await readFile(path.join(source, "scene/header.json"), "utf8"),
      );
      const published = JSON.parse(
        await readFile(path.join(context, "part-000/header.json"), "utf8"),
      );
      const entry = archive.find((s) => s.id === id);
      if (!entry || !contextMatches(header, published, entry, vectors))
        throw Error(
          `Static context does not match the recorded deployment for ${id}`,
        );
      for (const name of ["vectors", "background", "vantages.json"]) {
        try {
          await cp(path.join(context, name), path.join(dest, name), {
            recursive: true,
            errorOnExist: true,
          });
        } catch (e) {
          if (e.code !== "ENOENT") throw e;
        }
      }
      lanes = vectors.features.filter(
        (f) =>
          f.properties?.layer === "street_use" &&
          f.properties?.kind === "travel_lane",
      ).length;
      proposedLanes = vectors.features.filter(
        (f) =>
          f.properties?.layer === "lane_proposals" &&
          f.properties?.kind === "travel_lane",
      ).length;
    } catch (e) {
      if (e.code !== "ENOENT") throw e;
    }
    catalog.sites.push({
      id,
      title: id
        .split("-")
        .map((s) => s[0].toUpperCase() + s.slice(1))
        .join(" · "),
      lanes,
      proposedLanes,
    });
  }
  await writeFile(
    path.join(output, "bundle.json"),
    JSON.stringify(catalog, null, 2) + "\n",
    { flag: "wx" },
  );
  return catalog;
}
