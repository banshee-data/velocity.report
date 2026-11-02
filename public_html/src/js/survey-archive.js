/**
 * Fetches every file a survey's playback uses, so that a web archive saving the
 * page records them. See src/scenes/survey-archive-pages.11ty.js.
 */

/** One at a time would outlast the save; all at once would swamp it. */
const CONCURRENT_FETCHES = 6;

/** Fetches each URL in full, reporting each as it finishes. Never rejects. */
export async function fetchAll(urls, onResult, limit = CONCURRENT_FETCHES) {
  const queue = [...urls];
  const worker = async () => {
    while (queue.length) {
      const url = queue.shift();
      let fetched = false;
      try {
        const res = await fetch(url);
        // The archive records a response as it is read, so read all of it.
        if (res.ok) await res.arrayBuffer();
        fetched = res.ok;
      } catch {
        // Reported as not fetched. One failure should not stop the rest.
      }
      onResult(url, fetched);
    }
  };
  await Promise.all(Array.from({ length: limit }, worker));
}

/** What the page says once `done` of `total` files are finished. */
export function archiveStatus(done, failed, total) {
  if (done < total) return `Fetched ${done} of ${total} files…`;
  if (failed === 0) return `All ${total} files fetched.`;
  return `${total - failed} of ${total} files fetched. ${failed} could not be.`;
}

export async function mountArchivePage(root) {
  const items = new Map();
  for (const li of root.querySelectorAll(".archive-file")) {
    items.set(li.querySelector("a").href, li);
  }
  const status = root.getElementById("archive-status");
  let done = 0;
  let failed = 0;
  status.textContent = archiveStatus(done, failed, items.size);
  await fetchAll([...items.keys()], (url, fetched) => {
    done++;
    if (!fetched) failed++;
    items.get(url).dataset.state = fetched ? "fetched" : "failed";
    status.textContent = archiveStatus(done, failed, items.size);
  });
}
