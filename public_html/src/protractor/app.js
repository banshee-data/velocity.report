(() => {
  'use strict';

  const LINE_A_COLOR = '#22d3ee';
  const LINE_B_COLOR = '#f472b6';
  const HALO_COLOR = 'rgba(0, 0, 0, 0.55)';
  const HANDLE_RADIUS_CSS = 10;
  const HANDLE_HIT_PAD = 12;
  const LABEL_FONT = '600 16px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
  const MAX_ZOOM_MULT = 8;

  const els = {
    empty:      document.getElementById('empty-state'),
    editor:     document.getElementById('editor'),
    canvas:     document.getElementById('canvas'),
    canvasWrap: document.getElementById('canvas-wrap'),
    hint:       document.getElementById('hint'),
    alignValue: document.getElementById('align-value'),
    rawValue:   document.getElementById('raw-value'),
    fitBtn:     document.getElementById('fit-btn'),
    resetBtn:   document.getElementById('reset-btn'),
    file1:      document.getElementById('file-input'),
    file2:      document.getElementById('file-input-2'),
  };

  const ctx = els.canvas.getContext('2d');

  const state = {
    image: null,
    imgW: 0, imgH: 0,
    // Endpoints stored in IMAGE pixel coords (indices: 0,1=line A; 2,3=line B).
    points: [],
    // View transform (canvas CSS pixels): canvasX = imgX * scale + ox
    scale: 1, ox: 0, oy: 0,
    fitScale: 1,
    userAdjustedView: false,
    cssW: 0, cssH: 0, dpr: 1,
  };

  // Pointer + gesture state
  const pointers = new Map(); // pointerId -> {x, y} in canvas CSS coords
  // gesture shapes:
  //   { type: 'dragHandle', id, index }
  //   { type: 'pan', id, startX, startY, startOx, startOy }
  //   { type: 'pinch', id1, id2, startDist, startScale, imgPtAtMid }
  let gesture = null;

  // ---------- File loading ----------

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
      els.empty.classList.add('hidden');
      els.editor.classList.remove('hidden');
      resizeCanvas();
      updateReadout();
      updateHint();
      render();
      // Reset inputs so picking the same file again re-fires change.
      els.file1.value = '';
      els.file2.value = '';
      URL.revokeObjectURL(url);
    };
    img.onerror = () => {
      alert('Could not load that image.');
      URL.revokeObjectURL(url);
    };
    img.src = url;
  }

  els.file1.addEventListener('change', onFileChosen);
  els.file2.addEventListener('change', onFileChosen);

  // ---------- Canvas sizing ----------

  function resizeCanvas() {
    const rect = els.canvasWrap.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    state.cssW = rect.width;
    state.cssH = rect.height;
    state.dpr = dpr;
    els.canvas.width = Math.round(rect.width * dpr);
    els.canvas.height = Math.round(rect.height * dpr);
    els.canvas.style.width = rect.width + 'px';
    els.canvas.style.height = rect.height + 'px';
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

  // ---------- Coordinate transforms ----------

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

  function clampScale(s) {
    const minS = state.fitScale;
    const maxS = state.fitScale * MAX_ZOOM_MULT;
    return Math.max(minS, Math.min(maxS, s));
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

  // ---------- Pointer interaction ----------

  function hitTestHandle(cx, cy) {
    const threshold = HANDLE_RADIUS_CSS + HANDLE_HIT_PAD;
    let bestIdx = -1;
    let bestDist = threshold;
    for (let i = 0; i < state.points.length; i++) {
      const cp = imgToCanvas(state.points[i]);
      const d = Math.hypot(cp.x - cx, cp.y - cy);
      if (d <= bestDist) {
        bestDist = d;
        bestIdx = i;
      }
    }
    return bestIdx;
  }

  function startPinch() {
    const ids = Array.from(pointers.keys());
    if (ids.length < 2) return;
    const [id1, id2] = ids.slice(0, 2);
    const p1 = pointers.get(id1), p2 = pointers.get(id2);
    const mid = { x: (p1.x + p2.x) / 2, y: (p1.y + p2.y) / 2 };
    const dist = Math.hypot(p2.x - p1.x, p2.y - p1.y);
    gesture = {
      type: 'pinch',
      id1, id2,
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
    try { els.canvas.setPointerCapture(ev.pointerId); } catch (_) {}

    if (pointers.size >= 2) {
      startPinch();
      return;
    }

    // Single pointer
    if (state.points.length < 4) {
      // Placement mode: place the next point, then let the user drag it to refine.
      state.points.push(canvasToImg(x, y));
      gesture = { type: 'dragHandle', id: ev.pointerId, index: state.points.length - 1 };
      updateReadout();
      updateHint();
      render();
    } else {
      const idx = hitTestHandle(x, y);
      if (idx >= 0) {
        gesture = { type: 'dragHandle', id: ev.pointerId, index: idx };
      } else {
        gesture = {
          type: 'pan',
          id: ev.pointerId,
          startX: x, startY: y,
          startOx: state.ox, startOy: state.oy,
        };
      }
    }
  }

  function onPointerMove(ev) {
    if (!pointers.has(ev.pointerId)) return;
    const { x, y } = clientToCanvas(ev);
    pointers.set(ev.pointerId, { x, y });

    if (!gesture) return;

    if (gesture.type === 'pinch') {
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

    if (gesture.type === 'dragHandle' && gesture.id === ev.pointerId) {
      state.points[gesture.index] = canvasToImg(x, y);
      updateReadout();
      render();
      return;
    }

    if (gesture.type === 'pan' && gesture.id === ev.pointerId) {
      state.ox = gesture.startOx + (x - gesture.startX);
      state.oy = gesture.startOy + (y - gesture.startY);
      constrainPan();
      state.userAdjustedView = true;
      render();
      return;
    }
  }

  function onPointerUp(ev) {
    pointers.delete(ev.pointerId);
    try { els.canvas.releasePointerCapture(ev.pointerId); } catch (_) {}

    if (!gesture) return;

    if (gesture.type === 'pinch') {
      // End the pinch when either finger lifts. Don't auto-transition to pan
      // with the remaining finger — the user will lift and re-press if they
      // want to pan or drag a handle.
      if (pointers.size < 2) gesture = null;
    } else if (gesture.id === ev.pointerId) {
      gesture = null;
    }
    updateHint();
    render();
  }

  els.canvas.addEventListener('pointerdown', onPointerDown);
  els.canvas.addEventListener('pointermove', onPointerMove);
  els.canvas.addEventListener('pointerup', onPointerUp);
  els.canvas.addEventListener('pointercancel', onPointerUp);

  // Desktop mouse wheel zoom, anchored at cursor.
  els.canvas.addEventListener('wheel', (ev) => {
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
  }, { passive: false });

  // ---------- Buttons ----------

  els.resetBtn.addEventListener('click', () => {
    state.points = [];
    gesture = null;
    updateReadout();
    updateHint();
    render();
  });

  els.fitBtn.addEventListener('click', () => {
    if (!state.image) return;
    applyFitTransform();
    state.userAdjustedView = false;
    render();
  });

  // ---------- Geometry / readout ----------

  function computeAngles() {
    if (state.points.length < 4) return null;
    const [a1, a2, b1, b2] = state.points;
    const vAx = a2.x - a1.x, vAy = a2.y - a1.y;
    const vBx = b2.x - b1.x, vBy = b2.y - b1.y;
    const magA = Math.hypot(vAx, vAy);
    const magB = Math.hypot(vBx, vBy);
    if (magA < 1e-6 || magB < 1e-6) return null;
    let cosT = (vAx * vBx + vAy * vBy) / (magA * magB);
    cosT = Math.max(-1, Math.min(1, cosT));
    const theta = Math.acos(cosT) * 180 / Math.PI;
    const acute = Math.min(theta, 180 - theta);
    const align = 90 - acute;
    return { raw: acute, align };
  }

  function classify(align) {
    if (align >= 15 && align <= 25) return 'good';
    if ((align >= 10 && align < 15) || (align > 25 && align <= 30)) return 'warn';
    return 'bad';
  }

  function updateReadout() {
    const a = computeAngles();
    els.alignValue.classList.remove('good', 'warn', 'bad');
    if (!a) {
      els.alignValue.textContent = '—';
      els.rawValue.textContent = '—';
      return;
    }
    els.alignValue.textContent = a.align.toFixed(1) + '°';
    els.rawValue.textContent = a.raw.toFixed(1) + '°';
    els.alignValue.classList.add(classify(a.align));
  }

  function updateHint() {
    const n = state.points.length;
    if (n === 0)      els.hint.textContent = 'Tap to place line A start (along the kerb)';
    else if (n === 1) els.hint.textContent = 'Tap to place line A end';
    else if (n === 2) els.hint.textContent = 'Tap to place line B start (along the sensor beam)';
    else if (n === 3) els.hint.textContent = 'Tap to place line B end';
    else              els.hint.textContent = 'Drag handles to adjust · pinch or scroll to zoom';
  }

  // ---------- Rendering ----------

  function drawLineWithHalo(p1, p2, color) {
    const a = imgToCanvas(p1);
    const b = imgToCanvas(p2);
    ctx.lineCap = 'round';

    ctx.strokeStyle = HALO_COLOR;
    ctx.lineWidth = 7;
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();

    ctx.strokeStyle = color;
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();
  }

  function drawHandle(p, color) {
    const c = imgToCanvas(p);
    ctx.fillStyle = HALO_COLOR;
    ctx.beginPath();
    ctx.arc(c.x, c.y, HANDLE_RADIUS_CSS + 2, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(c.x, c.y, HANDLE_RADIUS_CSS, 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = '#fff';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.arc(c.x, c.y, HANDLE_RADIUS_CSS, 0, Math.PI * 2);
    ctx.stroke();
  }

  function lineIntersection(a1, a2, b1, b2) {
    const x1 = a1.x, y1 = a1.y, x2 = a2.x, y2 = a2.y;
    const x3 = b1.x, y3 = b1.y, x4 = b2.x, y4 = b2.y;
    const denom = (x1 - x2) * (y3 - y4) - (y1 - y2) * (x3 - x4);
    if (Math.abs(denom) < 1e-6) return null;
    const t = ((x1 - x3) * (y3 - y4) - (y1 - y3) * (x3 - x4)) / denom;
    return { x: x1 + t * (x2 - x1), y: y1 + t * (y2 - y1) };
  }

  function drawAngleArc() {
    if (state.points.length < 4) return;
    const [a1, a2, b1, b2] = state.points;
    const inter = lineIntersection(a1, a2, b1, b2);
    const center = inter || { x: (a1.x + a2.x) / 2, y: (a1.y + a2.y) / 2 };
    const c = imgToCanvas(center);

    const margin = 48;
    c.x = Math.max(margin, Math.min(state.cssW - margin, c.x));
    c.y = Math.max(margin, Math.min(state.cssH - margin, c.y));

    const a = computeAngles();
    if (!a) return;

    const ca1 = imgToCanvas(a1), ca2 = imgToCanvas(a2);
    const cb1 = imgToCanvas(b1), cb2 = imgToCanvas(b2);
    const angA = Math.atan2(ca2.y - ca1.y, ca2.x - ca1.x);
    const angB = Math.atan2(cb2.y - cb1.y, cb2.x - cb1.x);

    let start = angA, end = angB;
    let diff = ((end - start + Math.PI * 3) % (Math.PI * 2)) - Math.PI;
    if (Math.abs(diff) > Math.PI / 2) {
      end = angB + Math.PI;
      diff = ((end - start + Math.PI * 3) % (Math.PI * 2)) - Math.PI;
    }
    const arcRadius = 34;
    ctx.strokeStyle = '#fff';
    ctx.lineWidth = 2;
    ctx.beginPath();
    if (diff >= 0) ctx.arc(c.x, c.y, arcRadius, start, start + diff, false);
    else           ctx.arc(c.x, c.y, arcRadius, start, start + diff, true);
    ctx.stroke();

    const labelText = a.raw.toFixed(1) + '°';
    ctx.font = LABEL_FONT;
    const metrics = ctx.measureText(labelText);
    const tw = metrics.width;
    const th = 18;
    const labelAng = start + diff / 2;
    const lx = c.x + Math.cos(labelAng) * (arcRadius + 18);
    const ly = c.y + Math.sin(labelAng) * (arcRadius + 18);
    const padX = 6, padY = 3;
    ctx.fillStyle = 'rgba(0,0,0,0.7)';
    roundRect(ctx, lx - tw / 2 - padX, ly - th / 2 - padY, tw + padX * 2, th + padY * 2, 6);
    ctx.fill();
    ctx.fillStyle = '#fff';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(labelText, lx, ly);
    ctx.textAlign = 'start';
    ctx.textBaseline = 'alphabetic';
  }

  function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }

  function render() {
    ctx.clearRect(0, 0, state.cssW, state.cssH);
    if (!state.image) return;
    ctx.drawImage(
      state.image,
      state.ox, state.oy,
      state.imgW * state.scale, state.imgH * state.scale
    );

    if (state.points.length >= 2) drawLineWithHalo(state.points[0], state.points[1], LINE_A_COLOR);
    if (state.points.length >= 4) drawLineWithHalo(state.points[2], state.points[3], LINE_B_COLOR);
    drawAngleArc();
    for (let i = 0; i < state.points.length; i++) {
      const color = i < 2 ? LINE_A_COLOR : LINE_B_COLOR;
      drawHandle(state.points[i], color);
    }
  }

  updateHint();
})();
