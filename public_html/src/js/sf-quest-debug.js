const SVG_NS = "http://www.w3.org/2000/svg";

function svgElement(doc, tag, attributes) {
  const element = doc.createElementNS(SVG_NS, tag);
  for (const [name, value] of Object.entries(attributes))
    element.setAttribute(name, value);
  return element;
}

function pathData(lines, close = false) {
  return lines
    .map((line) => {
      const points = line.map(([x, y]) => `${x},${-y}`).join("L");
      return `M${points}${close ? "Z" : ""}`;
    })
    .join("");
}

export function showQuestDebug(doc, manifest, route, coverage) {
  const get = (id) => doc.getElementById(id);
  const map = get("city-map");
  const cells = get("cells");
  const markers = get("markers");
  const list = get("tiles");
  const counts = get("counts");
  const query = get("query");
  const filter = get("filter");
  const targets = new Map(route.targets.map((target) => [target.t, target]));
  const skipped = new Map(route.skipped.map(({ t, reason }) => [t, reason]));
  const rows = new Map();

  const corners = Object.values(coverage.cells).flat();
  const xs = corners.map(([x]) => x);
  const ys = corners.map(([, y]) => -y);
  const pad = 280;
  const bounds = {
    x: Math.min(...xs) - pad,
    y: Math.min(...ys) - pad,
    width: Math.max(...xs) - Math.min(...xs) + 2 * pad,
    height: Math.max(...ys) - Math.min(...ys) + 2 * pad,
  };
  let view = { ...bounds };
  let selected;
  let pointer;
  let ignoreClick = false;

  function redraw() {
    map.setAttribute(
      "viewBox",
      `${view.x} ${view.y} ${view.width} ${view.height}`,
    );
    markers.style.setProperty("--marker-size", `${view.width / 50}px`);
  }

  function select(row) {
    if (selected) {
      selected.cell?.classList.remove("selected");
      selected.marker?.classList.remove("selected");
      selected.button.removeAttribute("aria-current");
    }
    selected = row;
    row.cell?.classList.add("selected");
    row.marker?.classList.add("selected");
    row.button.setAttribute("aria-current", "true");
    get("details").querySelector("h2").textContent = row.target
      ? `${row.target.emoji}  ${row.token}`
      : row.token;
    get("intersection").textContent = row.label;
    get("coordinates").textContent = row.target
      ? `E ${row.target.x.toFixed(1)} m · N ${row.target.y.toFixed(1)} m · heading ${row.target.heading.toFixed(3)} rad`
      : "";
  }

  get("land").setAttribute("d", pathData(coverage.coast.flat(), true));
  get("parks").setAttribute("d", pathData(coverage.parks.flat(), true));
  get("roads").setAttribute("d", pathData(coverage.arterials));
  for (const { t: token } of manifest.tiles) {
    const target = targets.get(token);
    const label = target
      ? target.names.join(" / ")
      : skipped.get(token) || "No pickup";
    const ring = coverage.cells[token];
    const cell = ring
      ? svgElement(doc, "path", {
          d: pathData([ring], true),
          "data-tile": token,
          class: target ? "cell" : "cell skipped",
        })
      : null;
    if (cell) cells.append(cell);
    const marker = target
      ? svgElement(doc, "g", {
          class: "marker",
          "data-tile": token,
          transform: `translate(${target.x} ${-target.y})`,
        })
      : null;
    if (marker) {
      const glyph = svgElement(doc, "text", {});
      glyph.textContent = target.emoji;
      marker.append(svgElement(doc, "circle", {}), glyph);
      markers.append(marker);
    }
    const item = doc.createElement("li");
    const button = doc.createElement("button");
    button.type = "button";
    const glyph = doc.createElement("span");
    glyph.className = "glyph";
    glyph.textContent = target?.emoji || "—";
    const text = doc.createElement("span");
    const meta = doc.createElement("span");
    meta.className = "meta";
    meta.textContent = token;
    text.append(label, doc.createElement("br"), meta);
    button.append(glyph, text);
    item.append(button);
    list.append(item);
    const row = { token, target, label, cell, marker, item, button };
    rows.set(token, row);
    button.addEventListener("click", () => select(row));
  }

  function applyFilter() {
    const needle = query.value.trim().toLowerCase();
    const kind = filter.value;
    let visible = 0;
    for (const row of rows.values()) {
      const category = row.target ? "pickup" : "skipped";
      const match =
        (kind === "all" || kind === category) &&
        `${row.token} ${row.target?.emoji || ""} ${row.label}`
          .toLowerCase()
          .includes(needle);
      row.item.hidden = !match;
      row.cell?.classList.toggle("dim", !match);
      row.marker?.classList.toggle("dim", !match);
      if (match) visible++;
    }
    get("empty").hidden = visible !== 0;
    const total = `${rows.size} tiles · ${targets.size} pickups · ${skipped.size} skipped`;
    counts.textContent =
      visible === rows.size ? total : `${visible} shown · ${total}`;
  }

  function zoom(scale) {
    const width = view.width * scale;
    if (width < bounds.width / 8 || width > bounds.width * 1.6) return;
    const height = view.height * scale;
    view = {
      x: view.x + (view.width - width) / 2,
      y: view.y + (view.height - height) / 2,
      width,
      height,
    };
    redraw();
  }

  query.addEventListener("input", applyFilter);
  filter.addEventListener("change", applyFilter);
  get("zoom-in").addEventListener("click", () => zoom(0.75));
  get("zoom-out").addEventListener("click", () => zoom(4 / 3));
  get("fit").addEventListener("click", () => {
    view = { ...bounds };
    redraw();
  });
  map.addEventListener("click", (event) => {
    if (ignoreClick) return;
    const row = rows.get(event.target.closest("[data-tile]")?.dataset.tile);
    if (!row) return;
    select(row);
    if (!row.item.hidden) {
      list.scrollTop +=
        row.item.getBoundingClientRect().top - list.getBoundingClientRect().top;
    }
  });
  map.addEventListener("pointerdown", (event) => {
    if (!event.isPrimary || event.button !== 0) return;
    ignoreClick = false;
    pointer = { id: event.pointerId, x: event.clientX, y: event.clientY };
  });
  map.addEventListener("pointermove", (event) => {
    if (pointer?.id !== event.pointerId) return;
    const dx = event.clientX - pointer.x;
    const dy = event.clientY - pointer.y;
    if (!ignoreClick && Math.hypot(dx, dy) < 5) return;
    if (!ignoreClick) map.setPointerCapture(event.pointerId);
    ignoreClick = true;
    map.classList.add("dragging");
    const scale = map.getScreenCTM().a;
    view.x -= dx / scale;
    view.y -= dy / scale;
    pointer.x = event.clientX;
    pointer.y = event.clientY;
    redraw();
  });
  function endPan(event) {
    if (pointer?.id !== event.pointerId) return;
    pointer = null;
    map.classList.remove("dragging");
  }
  map.addEventListener("pointerup", endPan);
  map.addEventListener("pointercancel", endPan);
  map.addEventListener("lostpointercapture", endPan);
  map.addEventListener("pointerleave", (event) => {
    if (!map.hasPointerCapture(event.pointerId)) endPan(event);
  });
  redraw();
  applyFilter();
}
