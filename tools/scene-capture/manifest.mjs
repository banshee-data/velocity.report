// Provenance helpers for the capture manifest: content hashes, code
// revision, and UTC timestamps in the repo's machine-timestamp convention
// (ISO 8601, no sub-second precision, trailing Z).

import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

export async function sha256File(filePath) {
  const data = await readFile(filePath);
  return createHash("sha256").update(data).digest("hex");
}

/**
 * The commit and dirty state of the working tree that produced this build,
 * so a captured image is never mistaken for the output of a clean commit
 * when uncommitted changes were actually in play. Uses `git status
 * --porcelain` rather than `git describe --dirty`, which only compares
 * tracked-file diffs and would miss new, untracked files.
 */
export async function codeRevision(cwd) {
  try {
    const [{ stdout: commit }, { stdout: status }] = await Promise.all([
      execFileAsync("git", ["rev-parse", "HEAD"], { cwd }),
      execFileAsync(
        "git",
        ["status", "--porcelain", "--untracked-files=normal"],
        { cwd },
      ),
    ]);
    return { commit: commit.trim(), dirty: status.trim().length > 0 };
  } catch {
    return { commit: null, dirty: null };
  }
}

/** UTC ISO 8601 with a trailing Z and no sub-second precision. */
export function nowIso() {
  return new Date().toISOString().replace(/\.\d+Z$/, "Z");
}
