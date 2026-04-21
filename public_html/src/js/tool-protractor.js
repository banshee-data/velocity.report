(() => {
  "use strict";

  const root = document.getElementById("angle-tool-app");
  if (!root) return;

  const KERB_COLOR = "#374151";
  const SENSOR_COLOR = "#059669";
  const TAG_TEXT_COLOR = "#f8fafc";
  const TEXT_STROKE_COLOR = "rgba(0, 0, 0, 0.78)";
  const HALO_COLOR = "rgba(0, 0, 0, 0.55)";
  const HANDLE_RADIUS_CSS = 10;
  const HANDLE_HIT_PAD = 12;
  const FONT_FAMILY =
    '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
  const LABEL_FONT =
    `700 24px ${FONT_FAMILY}`;
  const EXPORT_FONT_FAMILY = FONT_FAMILY;
  const MAX_ZOOM_MULT = 8;
  const EXPORT_ARC_RADIUS = 300;
  const EXPORT_REFERENCE_LENGTH = 336;

  const els = {
    empty: document.getElementById("angle-empty-state"),
    editor: document.getElementById("angle-editor"),
    canvas: document.getElementById("angle-canvas"),
    canvasWrap: document.getElementById("angle-canvas-wrap"),
    hint: document.getElementById("angle-hint"),
    alignValue: document.getElementById("angle-align-value"),
    fitBtn: document.getElementById("angle-fit-btn"),
    resetBtn: document.getElementById("angle-reset-btn"),
    downloadBtn: document.getElementById("angle-download-btn"),
    file1: document.getElementById("angle-file-input"),
    file2: document.getElementById("angle-file-input-2"),
  };

  if (!els.canvas || !els.canvasWrap) return;

  const ctx = els.canvas.getContext("2d");
  if (!ctx) return;

  const state = {
    image: null,
    imgW: 0,
    imgH: 0,
    points: [],
    scale: 1,
    ox: 0,
    oy: 0,
    fitScale: 1,
    userAdjustedView: false,
    cssW: 0,
    cssH: 0,
    dpr: 1,
  };

  const pointers = new Map();
  let gesture = null;

  function onFileChosen(ev) {
    const file = ev.target.files && ev.target.files[0];
    if (!file) return;

    const url = URL.createObjectURL(file);
    const img = new Image();

    img.onload = () => {
      state.image = img;
      state.imgW = img.naturalWidth;
      state.imgH = img.naturalHeight;
      state.points = [];
      state.userAdjustedView = false;
      pointers.clear();
      gesture = null;
      els.empty.classList.add("hidden");
      els.editor.classList.remove("hidden");
      resizeCanvas();
      updateReadout();
      updateHint();
      render();
      requestAnimationFrame(() => {
        els.editor.scrollIntoView({ behavior: "smooth", block: "start" });
      });
      els.file1.value = "";
      els.file2.value = "";
      URL.revokeObjectURL(url);
    };

    img.onerror = () => {
      alert("Could not load that image.");
      URL.revokeObjectURL(url);
    };

    img.src = url;
  }

  els.file1.addEventListener("change", onFileChosen);
  els.file2.addEventListener("change", onFileChosen);

  function resizeCanvas() {
    const rect = els.canvasWrap.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;

    state.cssW = rect.width;
    state.cssH = rect.height;
    state.dpr = dpr;

    els.canvas.width = Math.round(rect.width * dpr);
    els.canvas.height = Math.round(rect.height * dpr);
    els.canvas.style.width = rect.width + "px";
    els.canvas.style.height = rect.height + "px";
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    if (!state.image || !state.cssW || !state.cssH) return;

    state.fitScale = Math.min(state.cssW / state.imgW, state.cssH / state.imgH);
    if (!state.userAdjustedView) {
      applyFitTransform();
    } else {
      state.scale = clampScale(state.scale);
      constrainPan();
    }
  }

  function applyFitTransform() {
    state.scale = state.fitScale;
    state.ox = (state.cssW - state.imgW * state.scale) / 2;
    state.oy = (state.cssH - state.imgH * state.scale) / 2;
  }

  const resizeObserver = new ResizeObserver(() => {
    if (!state.image) return;
    resizeCanvas();
    render();
  });
  resizeObserver.observe(els.canvasWrap);

  function imgToCanvas(p) {
    return { x: p.x * state.scale + state.ox, y: p.y * state.scale + state.oy };
  }

  function canvasToImg(cx, cy) {
    return { x: (cx - state.ox) / state.scale, y: (cy - state.oy) / state.scale };
  }

  function clientToCanvas(ev) {
    const rect = els.canvas.getBoundingClientRect();
    return { x: ev.clientX - rect.left, y: ev.clientY - rect.top };
  }

  function clampScale(scale) {
    const minScale = state.fitScale;
    const maxScale = state.fitScale * MAX_ZOOM_MULT;
    return Math.max(minScale, Math.min(maxScale, scale));
  }

  function constrainPan() {
    const imgCW = state.imgW * state.scale;
    const imgCH = state.imgH * state.scale;

    if (imgCW <= state.cssW) {
      state.ox = (state.cssW - imgCW) / 2;
    } else {
      state.ox = Math.max(state.cssW - imgCW, Math.min(0, state.ox));
    }

    if (imgCH <= state.cssH) {
      state.oy = (state.cssH - imgCH) / 2;
    } else {
      state.oy = Math.max(state.cssH - imgCH, Math.min(0, state.oy));
    }
  }

  function hitTestHandle(cx, cy) {
    const threshold = HANDLE_RADIUS_CSS + HANDLE_HIT_PAD;
    let bestIdx = -1;
    let bestDist = threshold;

    for (let i = 0; i < state.points.length; i += 1) {
      const cp = imgToCanvas(state.points[i]);
      const distance = Math.hypot(cp.x - cx, cp.y - cy);
      if (distance <= bestDist) {
        bestDist = distance;
        bestIdx = i;
      }
    }

    return bestIdx;
  }

  function startPinch() {
    const ids = Array.from(pointers.keys());
    if (ids.length < 2) return;

    const [id1, id2] = ids.slice(0, 2);
    const p1 = pointers.get(id1);
    const p2 = pointers.get(id2);
    const mid = { x: (p1.x + p2.x) / 2, y: (p1.y + p2.y) / 2 };
    const dist = Math.hypot(p2.x - p1.x, p2.y - p1.y);

    gesture = {
      type: "pinch",
      id1,
      id2,
      startDist: Math.max(dist, 1),
      startScale: state.scale,
      imgPtAtMid: canvasToImg(mid.x, mid.y),
    };
  }

  function onPointerDown(ev) {
    if (!state.image) return;

    ev.preventDefault();
    const { x, y } = clientToCanvas(ev);
    pointers.set(ev.pointerId, { x, y });

    try {
      els.canvas.setPointerCapture(ev.pointerId);
    } catch (_error) {}

    if (pointers.size >= 2) {
      startPinch();
      return;
    }

    if (state.points.length < 4) {
      state.points.push(canvasToImg(x, y));
      gesture = {
        type: "dragHandle",
        id: ev.pointerId,
        index: state.points.length - 1,
      };
      updateReadout();
      updateHint();
      render();
      return;
    }

    const idx = hitTestHandle(x, y);
    if (idx >= 0) {
      gesture = { type: "dragHandle", id: ev.pointerId, index: idx };
      return;
    }

    gesture = {
      type: "pan",
      id: ev.pointerId,
      startX: x,
      startY: y,
      startOx: state.ox,
      startOy: state.oy,
    };
  }

  function onPointerMove(ev) {
    if (!pointers.has(ev.pointerId)) return;

    const { x, y } = clientToCanvas(ev);
    pointers.set(ev.pointerId, { x, y });

    if (!gesture) return;

    if (gesture.type === "pinch") {
      const p1 = pointers.get(gesture.id1);
      const p2 = pointers.get(gesture.id2);
      if (!p1 || !p2) return;

      const mid = { x: (p1.x + p2.x) / 2, y: (p1.y + p2.y) / 2 };
      const dist = Math.hypot(p2.x - p1.x, p2.y - p1.y);
      if (dist < 1) return;

      const newScale = clampScale(gesture.startScale * (dist / gesture.startDist));
      state.scale = newScale;
      state.ox = mid.x - gesture.imgPtAtMid.x * newScale;
      state.oy = mid.y - gesture.imgPtAtMid.y * newScale;
      constrainPan();
      state.userAdjustedView = true;
      render();
      return;
    }

    if (gesture.type === "dragHandle" && gesture.id === ev.pointerId) {
      state.points[gesture.index] = canvasToImg(x, y);
      updateReadout();
      render();
      return;
    }

    if (gesture.type === "pan" && gesture.id === ev.pointerId) {
      state.ox = gesture.startOx + (x - gesture.startX);
      state.oy = gesture.startOy + (y - gesture.startY);
      constrainPan();
      state.userAdjustedView = true;
      render();
    }
  }

  function onPointerUp(ev) {
    pointers.delete(ev.pointerId);

    try {
      els.canvas.releasePointerCapture(ev.pointerId);
    } catch (_error) {}

    if (!gesture) return;

    if (gesture.type === "pinch") {
      if (pointers.size < 2) gesture = null;
    } else if (gesture.id === ev.pointerId) {
      gesture = null;
    }

    updateHint();
    render();
  }

  els.canvas.addEventListener("pointerdown", onPointerDown);
  els.canvas.addEventListener("pointermove", onPointerMove);
  els.canvas.addEventListener("pointerup", onPointerUp);
  els.canvas.addEventListener("pointercancel", onPointerUp);

  els.canvas.addEventListener(
    "wheel",
    (ev) => {
      if (!state.image) return;

      ev.preventDefault();
      const { x, y } = clientToCanvas(ev);
      const imgPt = canvasToImg(x, y);
      const factor = Math.exp(-ev.deltaY * 0.0015);
      const newScale = clampScale(state.scale * factor);
      if (newScale === state.scale) return;

      state.scale = newScale;
      state.ox = x - imgPt.x * newScale;
      state.oy = y - imgPt.y * newScale;
      constrainPan();
      state.userAdjustedView = true;
      render();
    },
    { passive: false },
  );

  els.resetBtn.addEventListener("click", () => {
    state.points = [];
    gesture = null;
    updateReadout();
    updateHint();
    render();
  });

  els.fitBtn.addEventListener("click", () => {
    if (!state.image) return;
    applyFitTransform();
    state.userAdjustedView = false;
    render();
  });

  els.downloadBtn.addEventListener("click", () => {
    if (!state.image) return;
    downloadComposite();
  });

  function computeMeasurement() {
    if (state.points.length < 4) return null;

    const [kerbStart, kerbEnd, sensorStart, sensorEnd] = state.points;
    const kerbVec = { x: kerbEnd.x - kerbStart.x, y: kerbEnd.y - kerbStart.y };
    const sensorVec = {
      x: sensorEnd.x - sensorStart.x,
      y: sensorEnd.y - sensorStart.y,
    };
    const kerbLength = Math.hypot(kerbVec.x, kerbVec.y);
    const sensorLength = Math.hypot(sensorVec.x, sensorVec.y);

    if (kerbLength < 1e-6 || sensorLength < 1e-6) return null;

    let cosTheta =
      (kerbVec.x * sensorVec.x + kerbVec.y * sensorVec.y) /
      (kerbLength * sensorLength);
    cosTheta = Math.max(-1, Math.min(1, cosTheta));

    const lineAngle = (Math.acos(Math.abs(cosTheta)) * 180) / Math.PI;
    const alignmentAngle = Math.max(0, 90 - lineAngle);

    return {
      kerbStart,
      kerbEnd,
      sensorStart,
      sensorEnd,
      kerbVec,
      sensorVec,
      lineAngle,
      alignmentAngle,
    };
  }

  function getGrade(alignmentAngle) {
    if (alignmentAngle < 30) {
      return {
        name: "good",
        fill: "rgba(4, 120, 87, 0.7)",
        stroke: "rgba(4, 120, 87, 0.95)",
      };
    }

    if (alignmentAngle <= 45) {
      return {
        name: "warn",
        fill: "rgba(217, 119, 6, 0.7)",
        stroke: "rgba(217, 119, 6, 0.95)",
      };
    }

    return {
      name: "bad",
      fill: "rgba(220, 38, 38, 0.7)",
      stroke: "rgba(220, 38, 38, 0.95)",
    };
  }

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function getSectorLabelRadius(arcRadius, angleRadians) {
    const theta = Math.max(Math.abs(angleRadians), 1e-3);
    const centroidRadius =
      (4 * arcRadius * Math.sin(theta / 2)) / (3 * theta);
    const preferredOuterRadius =
      theta < Math.PI / 8
        ? arcRadius * 0.9
        : theta < Math.PI / 3
          ? arcRadius * 0.84
          : arcRadius * 0.76;
    return clamp(
      Math.max(centroidRadius + arcRadius * 0.08, preferredOuterRadius),
      arcRadius * 0.46,
      arcRadius * 0.92,
    );
  }

  function getMaxRadiusForBounds(center, angle, boundW, boundH, padding) {
    let maxRadius = Infinity;
    const cosAngle = Math.cos(angle);
    const sinAngle = Math.sin(angle);

    if (Math.abs(cosAngle) > 1e-6) {
      const xLimit =
        cosAngle > 0
          ? (boundW - padding - center.x) / cosAngle
          : (padding - center.x) / cosAngle;
      if (xLimit >= 0) {
        maxRadius = Math.min(maxRadius, xLimit);
      }
    }

    if (Math.abs(sinAngle) > 1e-6) {
      const yLimit =
        sinAngle > 0
          ? (boundH - padding - center.y) / sinAngle
          : (padding - center.y) / sinAngle;
      if (yLimit >= 0) {
        maxRadius = Math.min(maxRadius, yLimit);
      }
    }

    return Number.isFinite(maxRadius) ? Math.max(0, maxRadius) : 0;
  }

  function classify(alignmentAngle) {
    return getGrade(alignmentAngle).name;
  }

  function updateReadout() {
    const measurement = computeMeasurement();
    els.alignValue.classList.remove("good", "warn", "bad");

    if (!measurement) {
      els.alignValue.textContent = "-";
      return;
    }

    els.alignValue.textContent = `${measurement.alignmentAngle.toFixed(1)}°`;
    els.alignValue.classList.add(classify(measurement.alignmentAngle));
  }

  function normaliseAngle(angle) {
    return ((angle + Math.PI * 3) % (Math.PI * 2)) - Math.PI;
  }

  function updateHint() {
    const pointCount = state.points.length;

    if (pointCount === 0) {
      els.hint.textContent = "Tap the first point on the straight kerb edge near the car";
    } else if (pointCount === 1) {
      els.hint.textContent = "Tap the second point on that kerb edge to draw the road-direction line";
    } else if (pointCount === 2) {
      els.hint.textContent = "Tap the first point on the sensor baseplate";
    } else if (pointCount === 3) {
      els.hint.textContent = "Tap the second point on the baseplate to draw the sensor line";
    } else {
      els.hint.textContent = "Drag handles to adjust. Pinch or scroll to zoom.";
    }
  }

  function drawLineWithHalo(context, point1, point2, color, mapPoint, options = {}) {
    const a = mapPoint(point1);
    const b = mapPoint(point2);
    const haloWidth = options.haloWidth ?? 7;
    const lineWidth = options.lineWidth ?? 3;

    context.lineCap = "round";
    context.strokeStyle = HALO_COLOR;
    context.lineWidth = haloWidth;
    context.beginPath();
    context.moveTo(a.x, a.y);
    context.lineTo(b.x, b.y);
    context.stroke();

    context.strokeStyle = color;
    context.lineWidth = lineWidth;
    context.beginPath();
    context.moveTo(a.x, a.y);
    context.lineTo(b.x, b.y);
    context.stroke();
  }

  function drawHandle(context, point, color, mapPoint, radius = HANDLE_RADIUS_CSS) {
    const canvasPoint = mapPoint(point);

    context.fillStyle = HALO_COLOR;
    context.beginPath();
    context.arc(canvasPoint.x, canvasPoint.y, radius + 2, 0, Math.PI * 2);
    context.fill();

    context.fillStyle = color;
    context.beginPath();
    context.arc(canvasPoint.x, canvasPoint.y, radius, 0, Math.PI * 2);
    context.fill();

    context.strokeStyle = "#fff";
    context.lineWidth = Math.max(2, radius * 0.2);
    context.beginPath();
    context.arc(canvasPoint.x, canvasPoint.y, radius, 0, Math.PI * 2);
    context.stroke();
  }

  function lineIntersection(a1, a2, b1, b2) {
    const x1 = a1.x;
    const y1 = a1.y;
    const x2 = a2.x;
    const y2 = a2.y;
    const x3 = b1.x;
    const y3 = b1.y;
    const x4 = b2.x;
    const y4 = b2.y;
    const denom = (x1 - x2) * (y3 - y4) - (y1 - y2) * (x3 - x4);

    if (Math.abs(denom) < 1e-6) return null;

    const t = ((x1 - x3) * (y3 - y4) - (y1 - y3) * (x3 - x4)) / denom;
    return { x: x1 + t * (x2 - x1), y: y1 + t * (y2 - y1) };
  }

  function getAngleGeometry() {
    const measurement = computeMeasurement();
    if (!measurement) return null;

    const intersection = lineIntersection(
      measurement.kerbStart,
      measurement.kerbEnd,
      measurement.sensorStart,
      measurement.sensorEnd,
    );
    const center = intersection || {
      x: (measurement.kerbStart.x + measurement.kerbEnd.x) / 2,
      y: (measurement.kerbStart.y + measurement.kerbEnd.y) / 2,
    };

    const kerbAngle = Math.atan2(measurement.kerbVec.y, measurement.kerbVec.x);
    const sensorAngle = Math.atan2(measurement.sensorVec.y, measurement.sensorVec.x);
    const normalAngles = [kerbAngle + Math.PI / 2, kerbAngle - Math.PI / 2];
    const upwardNormalAngle = normalAngles.reduce((bestAngle, candidateAngle) => {
      if (bestAngle === null) return candidateAngle;
      return Math.sin(candidateAngle) < Math.sin(bestAngle) ? candidateAngle : bestAngle;
    }, null);
    const sensorAngles = [sensorAngle, sensorAngle + Math.PI];

    let bestGeometry = null;
    for (const sensorLineAngle of sensorAngles) {
      const diff = normaliseAngle(sensorLineAngle - upwardNormalAngle);
      if (!bestGeometry || Math.abs(diff) < Math.abs(bestGeometry.diff)) {
        bestGeometry = { start: upwardNormalAngle, diff };
      }
    }

    return {
      measurement,
      center,
      start: bestGeometry.start,
      diff: bestGeometry.diff,
      labelText: `${measurement.alignmentAngle.toFixed(1)}°`,
    };
  }

  function drawAngleText(context, x, y, text, options = {}) {
    context.save();
    context.font = options.font ?? LABEL_FONT;
    context.fillStyle = options.fillStyle ?? TAG_TEXT_COLOR;
    context.strokeStyle = options.strokeStyle ?? TEXT_STROKE_COLOR;
    context.lineWidth = options.lineWidth ?? 4;
    context.lineJoin = "round";
    context.textAlign = "center";
    context.textBaseline = "middle";
    context.strokeText(text, x, y);
    context.fillText(text, x, y);
    context.restore();
  }

  function drawAngleArc(context, mapPoint, options = {}) {
    const geometry = getAngleGeometry();
    if (!geometry) return;
    const grade = getGrade(geometry.measurement.alignmentAngle);

    const center = mapPoint(geometry.center);
    const arcRadius = options.arcRadius ?? 34;
    const referenceLength = options.referenceLength ?? arcRadius + 72;
    const startX = center.x + Math.cos(geometry.start) * arcRadius;
    const startY = center.y + Math.sin(geometry.start) * arcRadius;
    const endAngle = geometry.start + geometry.diff;

    context.fillStyle = options.sectorFill ?? grade.fill;
    context.beginPath();
    context.moveTo(center.x, center.y);
    context.lineTo(startX, startY);
    if (geometry.diff >= 0) {
      context.arc(center.x, center.y, arcRadius, geometry.start, endAngle, false);
    } else {
      context.arc(center.x, center.y, arcRadius, geometry.start, endAngle, true);
    }
    context.closePath();
    context.fill();

    context.strokeStyle = options.referenceColor ?? "rgba(248, 250, 252, 0.92)";
    context.lineWidth = options.referenceWidth ?? 2;
    context.beginPath();
    context.moveTo(center.x, center.y);
    context.lineTo(
      center.x + Math.cos(geometry.start) * referenceLength,
      center.y + Math.sin(geometry.start) * referenceLength,
    );
    context.stroke();

    context.strokeStyle = options.strokeColor ?? grade.stroke;
    context.lineWidth = options.strokeWidth ?? 2;
    context.beginPath();
    if (geometry.diff >= 0) {
      context.arc(center.x, center.y, arcRadius, geometry.start, endAngle, false);
    } else {
      context.arc(center.x, center.y, arcRadius, geometry.start, endAngle, true);
    }
    context.stroke();

    const labelAngle = geometry.start + geometry.diff / 2;
    const labelPadding = options.margin ?? 24;
    const maxRadiusForBounds = getMaxRadiusForBounds(
      center,
      labelAngle,
      options.boundW,
      options.boundH,
      labelPadding,
    );
    const labelRadius = clamp(
      Math.min(
        options.labelRadius ?? getSectorLabelRadius(arcRadius, geometry.diff),
        maxRadiusForBounds,
        arcRadius * 0.92,
      ),
      arcRadius * 0.35,
      arcRadius * 0.92,
    );
    const labelX = center.x + Math.cos(labelAngle) * labelRadius;
    const labelY = center.y + Math.sin(labelAngle) * labelRadius;
    drawAngleText(
      context,
      labelX,
      labelY,
      geometry.labelText,
      {
        font: options.annotationFont,
        fillStyle: options.annotationFill,
        strokeStyle: options.annotationStroke,
        lineWidth: options.annotationStrokeWidth,
      },
    );
  }

  function drawOverlay(context, mapPoint, options = {}) {
    const lineWidth = options.lineWidth ?? 3;
    const haloWidth = options.haloWidth ?? 7;
    const handleRadius = options.handleRadius ?? HANDLE_RADIUS_CSS;

    if (state.points.length >= 2) {
      drawLineWithHalo(context, state.points[0], state.points[1], KERB_COLOR, mapPoint, {
        lineWidth,
        haloWidth,
      });
    }

    if (state.points.length >= 4) {
      drawLineWithHalo(context, state.points[2], state.points[3], SENSOR_COLOR, mapPoint, {
        lineWidth,
        haloWidth,
      });
    }

    if (state.points.length >= 4) {
      drawAngleArc(context, mapPoint, {
        boundW: options.boundW,
        boundH: options.boundH,
        arcRadius: options.arcRadius,
        referenceLength: options.referenceLength,
        referenceColor: options.referenceColor,
        referenceWidth: options.referenceWidth,
        sectorFill: options.sectorFill,
        strokeColor: options.strokeColor,
        strokeWidth: options.strokeWidth,
        labelRadius: options.labelRadius,
        annotationFont: options.annotationFont,
        annotationFill: options.annotationFill,
        annotationStroke: options.annotationStroke,
        annotationStrokeWidth: options.annotationStrokeWidth,
        margin: options.margin,
      });
    }

    if (options.showHandles) {
      for (let i = 0; i < state.points.length; i += 1) {
        drawHandle(
          context,
          state.points[i],
          i < 2 ? KERB_COLOR : SENSOR_COLOR,
          mapPoint,
          handleRadius,
        );
      }
    }
  }

  function downloadComposite() {
    const exportCanvas = document.createElement("canvas");
    exportCanvas.width = state.imgW;
    exportCanvas.height = state.imgH;

    const exportCtx = exportCanvas.getContext("2d");
    if (!exportCtx) return;

    exportCtx.drawImage(state.image, 0, 0, state.imgW, state.imgH);

    const scaleUnit = Math.max(1, Math.min(state.imgW, state.imgH) / 1200);
    drawOverlay(exportCtx, (point) => point, {
      boundW: state.imgW,
      boundH: state.imgH,
      lineWidth: Math.max(4, 5 * scaleUnit),
      haloWidth: Math.max(8, 12 * scaleUnit),
      handleRadius: Math.max(7, 10 * scaleUnit),
      arcRadius: Math.max(EXPORT_ARC_RADIUS, EXPORT_ARC_RADIUS * scaleUnit),
      referenceLength: Math.max(
        EXPORT_REFERENCE_LENGTH,
        EXPORT_REFERENCE_LENGTH * scaleUnit,
      ),
      referenceColor: "rgba(248, 250, 252, 0.92)",
      referenceWidth: Math.max(2, 3 * scaleUnit),
      strokeColor: undefined,
      strokeWidth: Math.max(2, 3 * scaleUnit),
      annotationFont: `${Math.round(Math.max(28, 36 * scaleUnit))}px ${EXPORT_FONT_FAMILY}`,
      annotationFill: TAG_TEXT_COLOR,
      annotationStroke: TEXT_STROKE_COLOR,
      annotationStrokeWidth: Math.max(3, 4 * scaleUnit),
      margin: Math.max(40, 56 * scaleUnit),
      showHandles: false,
    });

    exportCanvas.toBlob((blob) => {
      if (!blob) return;
      const objectUrl = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = objectUrl;
      link.download = "velocity-report-protractor-overlay.png";
      link.click();
      URL.revokeObjectURL(objectUrl);
    }, "image/png");
  }

  function roundRect(context, x, y, w, h, r) {
    context.beginPath();
    context.moveTo(x + r, y);
    context.arcTo(x + w, y, x + w, y + h, r);
    context.arcTo(x + w, y + h, x, y + h, r);
    context.arcTo(x, y + h, x, y, r);
    context.arcTo(x, y, x + w, y, r);
    context.closePath();
  }

  function render() {
    ctx.clearRect(0, 0, state.cssW, state.cssH);
    if (!state.image) return;

    const liveScaleUnit = Math.max(state.scale, 0.55);

    ctx.drawImage(
      state.image,
      state.ox,
      state.oy,
      state.imgW * state.scale,
      state.imgH * state.scale,
    );

    drawOverlay(ctx, imgToCanvas, {
      boundW: state.cssW,
      boundH: state.cssH,
      lineWidth: 3,
      haloWidth: 7,
      handleRadius: HANDLE_RADIUS_CSS,
      arcRadius: EXPORT_ARC_RADIUS * liveScaleUnit,
      referenceLength: EXPORT_REFERENCE_LENGTH * liveScaleUnit,
      referenceColor: "rgba(248, 250, 252, 0.92)",
      referenceWidth: 2,
      strokeColor: undefined,
      strokeWidth: 2,
      annotationFont: `700 ${Math.round(Math.max(24, 34 * liveScaleUnit))}px ${FONT_FAMILY}`,
      annotationFill: TAG_TEXT_COLOR,
      annotationStroke: TEXT_STROKE_COLOR,
      annotationStrokeWidth: 5,
      margin: 24,
      showHandles: true,
    });
  }

  updateHint();
})();
