// San Francisco driving view (/sf/): drive anywhere in the city over measured
// geometry. Tiles (one per S2 level-13 cell, about 1.1 km across) stream in
// around the car: ground, buildings, trees, roads with kerbs, and street-name
// signposts. The chase camera sits behind the car; a mirror shows the road behind.
import * as THREE from "../vendor/three/three.module.js";
import {
  carStep,
  groundAtWorld,
  nextCrossing,
  streetAt,
  tilesAround,
} from "./sf-drive-core.js";

const css = (n) =>
  getComputedStyle(document.documentElement).getPropertyValue(n).trim();
const col = (n) => new THREE.Color(css(n));

async function fetchJsonGz(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: ${res.status}`);
  const raw = await res.arrayBuffer();
  const head = new Uint8Array(raw, 0, 2);
  let text;
  if (head[0] === 0x1f && head[1] === 0x8b) {
    const stream = new Blob([raw])
      .stream()
      .pipeThrough(new DecompressionStream("gzip"));
    text = await new Response(stream).text();
  } else {
    text = new TextDecoder().decode(raw); // a host that already decoded the gzip
  }
  return JSON.parse(text);
}

// World (x east, y north, z up) to three.js (x, y up, -z north).
const V = (x, y, z) => [x, z, -y];

function groundFn(tile) {
  return (lx, ly) => {
    const g = tile.g;
    const n = g.n;
    const fx = Math.min(Math.max((lx - g.x0) / g.s, 0), n - 1.001);
    const fy = Math.min(Math.max((ly - g.y0) / g.s, 0), n - 1.001);
    const i = Math.floor(fx);
    const j = Math.floor(fy);
    const tx = fx - i;
    const ty = fy - j;
    const zs = [
      g.z[j * n + i],
      g.z[j * n + i + 1],
      g.z[(j + 1) * n + i],
      g.z[(j + 1) * n + i + 1],
    ];
    const ws = [(1 - tx) * (1 - ty), tx * (1 - ty), (1 - tx) * ty, tx * ty];
    let s = 0;
    let w = 0;
    for (let k = 0; k < 4; k++) {
      if (zs[k] === -32768) continue;
      s += (zs[k] / 10) * ws[k];
      w += ws[k];
    }
    return w > 0 ? s / w : tile.zfill;
  };
}

function mesh(positions, colour, opts = {}) {
  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.Float32BufferAttribute(positions, 3));
  if (opts.colours)
    g.setAttribute("color", new THREE.Float32BufferAttribute(opts.colours, 3));
  g.computeVertexNormals();
  const m = new THREE.MeshLambertMaterial({
    color: opts.colours ? 0xffffff : colour,
    vertexColors: !!opts.colours,
    side: THREE.DoubleSide,
    polygonOffset: !!opts.offset,
    polygonOffsetFactor: -(opts.offset || 0),
    polygonOffsetUnits: -(opts.offset || 0),
  });
  return new THREE.Mesh(g, m);
}

function buildTile(tile, C) {
  const ox = tile.o[0];
  const oy = tile.o[1];
  const gz = groundFn(tile);
  const grp = new THREE.Group();
  // ground
  const g = tile.g;
  const n = g.n;
  const vals = g.z.filter((v) => v !== -32768);
  tile.zfill = vals.length
    ? vals.reduce((a, b) => a + b, 0) / vals.length / 10
    : 0;
  const gp = new Float32Array(n * n * 3);
  for (let j = 0; j < n; j++) {
    for (let i = 0; i < n; i++) {
      const lx = g.x0 + i * g.s;
      const ly = g.y0 + j * g.s;
      const z = g.z[j * n + i] === -32768 ? tile.zfill : g.z[j * n + i] / 10;
      gp.set(V(ox + lx, oy + ly, z), (j * n + i) * 3);
    }
  }
  const idx = [];
  for (let j = 0; j < n - 1; j++) {
    for (let i = 0; i < n - 1; i++) {
      const a = j * n + i;
      if ([a, a + 1, a + n, a + n + 1].some((k) => g.z[k] === -32768)) continue;
      idx.push(a, a + 1, a + n, a + 1, a + n + 1, a + n);
    }
  }
  const gg = new THREE.BufferGeometry();
  gg.setAttribute("position", new THREE.BufferAttribute(gp, 3));
  gg.setIndex(idx);
  gg.computeVertexNormals();
  grp.add(new THREE.Mesh(gg, new THREE.MeshLambertMaterial({ color: C.land })));

  // buildings: each roof face a prism from the building base to its plane
  const P = [];
  const CL = [];
  for (const b of tile.b) {
    const zb = b[0] / 10;
    const roof = b[2] ? C.extra : C.roof;
    const wall = b[2] ? C.extra : C.wall;
    for (const face of b[1]) {
      const rings = face.map((r) => {
        const a = [];
        for (let i = 0; i < r.length; i += 3)
          a.push([ox + r[i] / 10, oy + r[i + 1] / 10, r[i + 2] / 10]);
        return a;
      });
      const outer = rings[0].map((v) => new THREE.Vector2(v[0], v[1]));
      const holes = rings
        .slice(1)
        .map((h) => h.map((v) => new THREE.Vector2(v[0], v[1])));
      let tris = [];
      try {
        tris = THREE.ShapeUtils.triangulateShape(outer, holes);
      } catch {
        tris = [];
      }
      const all = [].concat(...rings);
      for (const t of tris) {
        for (const k of t) {
          P.push(...V(all[k][0], all[k][1], all[k][2]));
          CL.push(roof.r, roof.g, roof.b);
        }
      }
      for (const r of rings) {
        for (let i = 0; i < r.length; i++) {
          const a = r[i];
          const c = r[(i + 1) % r.length];
          const q = [
            [a[0], a[1], zb],
            [c[0], c[1], zb],
            [c[0], c[1], c[2]],
            [a[0], a[1], zb],
            [c[0], c[1], c[2]],
            [a[0], a[1], a[2]],
          ];
          for (const v of q) {
            P.push(...V(v[0], v[1], v[2]));
            CL.push(wall.r, wall.g, wall.b);
          }
        }
      }
    }
  }
  if (P.length) grp.add(mesh(P, null, { colours: CL }));

  // roads: asphalt between the kerbs, dashed centrelines on two-way streets, paths
  const A = [];
  const D = [];
  const W = [];
  for (const r of tile.r) {
    const p = r[3];
    const w = r[4];
    const m = p.length / 2;
    if (m < 2) continue;
    const pt = (i) => [ox + p[2 * i] / 10, oy + p[2 * i + 1] / 10];
    const L = [];
    const R = [];
    for (let i = 0; i < m; i++) {
      const a = pt(Math.max(0, i - 1));
      const c = pt(Math.min(m - 1, i + 1));
      let tx = c[0] - a[0];
      let ty = c[1] - a[1];
      const tl = Math.hypot(tx, ty) || 1;
      tx /= tl;
      ty /= tl;
      const [x, y] = pt(i);
      const wl = w.length ? w[2 * i] / 10 : 1.2;
      const wr = w.length ? w[2 * i + 1] / 10 : 1.2;
      const lx = x - ty * wl;
      const ly = y + tx * wl;
      const rx = x + ty * wr;
      const ry = y - tx * wr;
      L.push(V(lx, ly, gz(lx - ox, ly - oy) + 0.06));
      R.push(V(rx, ry, gz(rx - ox, ry - oy) + 0.06));
    }
    const target = w.length ? A : W;
    for (let i = 0; i + 1 < m; i++)
      target.push(
        ...L[i],
        ...R[i],
        ...L[i + 1],
        ...R[i],
        ...R[i + 1],
        ...L[i + 1],
      );
    if (w.length && r[2] === 0) {
      let acc = 0;
      for (let i = 0; i + 1 < m; i++) {
        const [x0, y0] = pt(i);
        const [x1, y1] = pt(i + 1);
        const len = Math.hypot(x1 - x0, y1 - y0);
        if (len > 1e-6 && Math.floor(acc / 3) % 2 === 0) {
          const nx = (-(y1 - y0) / len) * 0.075;
          const ny = ((x1 - x0) / len) * 0.075;
          const z0 = gz(x0 - ox, y0 - oy) + 0.09;
          const z1 = gz(x1 - ox, y1 - oy) + 0.09;
          D.push(
            ...V(x0 + nx, y0 + ny, z0),
            ...V(x0 - nx, y0 - ny, z0),
            ...V(x1 + nx, y1 + ny, z1),
          );
          D.push(
            ...V(x0 - nx, y0 - ny, z0),
            ...V(x1 - nx, y1 - ny, z1),
            ...V(x1 + nx, y1 + ny, z1),
          );
        }
        acc += len;
      }
    }
  }
  if (A.length) grp.add(mesh(A, C.asphalt, { offset: 2 }));
  if (W.length) grp.add(mesh(W, C.path, { offset: 2 }));
  if (D.length) grp.add(mesh(D, C.dash, { offset: 3 }));

  // kerbs: 0.3 m wide, raised 15 cm
  const K = [];
  for (const k of tile.k) {
    for (let i = 0; i + 3 < k.length; i += 2) {
      const x0 = ox + k[i] / 10;
      const y0 = oy + k[i + 1] / 10;
      const x1 = ox + k[i + 2] / 10;
      const y1 = oy + k[i + 3] / 10;
      const len = Math.hypot(x1 - x0, y1 - y0);
      if (len < 1e-6) continue;
      const nx = (-(y1 - y0) / len) * 0.15;
      const ny = ((x1 - x0) / len) * 0.15;
      const z0 = gz(x0 - ox, y0 - oy);
      const z1 = gz(x1 - ox, y1 - oy);
      const a = V(x0 + nx, y0 + ny, z0 + 0.15);
      const b = V(x0 - nx, y0 - ny, z0 + 0.15);
      const c = V(x1 + nx, y1 + ny, z1 + 0.15);
      const d = V(x1 - nx, y1 - ny, z1 + 0.15);
      const e = V(x0 + nx, y0 + ny, z0);
      const f = V(x1 + nx, y1 + ny, z1);
      const h = V(x0 - nx, y0 - ny, z0);
      const q = V(x1 - nx, y1 - ny, z1);
      K.push(
        ...a,
        ...b,
        ...c,
        ...b,
        ...d,
        ...c,
        ...e,
        ...a,
        ...f,
        ...a,
        ...c,
        ...f,
        ...h,
        ...q,
        ...b,
        ...b,
        ...q,
        ...d,
      );
    }
  }
  if (K.length) grp.add(mesh(K, C.kerb, { offset: 1 }));

  // trees: crown and trunk, instanced
  const nt = tile.tr.length;
  if (nt) {
    const crown = new THREE.InstancedMesh(
      new THREE.IcosahedronGeometry(1, 1),
      new THREE.MeshLambertMaterial({ color: C.tree }),
      nt,
    );
    const trunk = new THREE.InstancedMesh(
      new THREE.CylinderGeometry(1, 1, 1, 6),
      new THREE.MeshLambertMaterial({ color: C.trunk }),
      nt,
    );
    const mtx = new THREE.Matrix4();
    const qn = new THREE.Quaternion();
    const s = new THREE.Vector3();
    const p = new THREE.Vector3();
    tile.tr.forEach(([x, y, h, r], i) => {
      const wx = ox + x / 10;
      const wy = oy + y / 10;
      const z = gz(x / 10, y / 10);
      const ch = Math.max(1.5, Math.min(h * 0.6, 2.2 * r));
      p.set(...V(wx, wy, z + h - ch / 2));
      s.set(r, ch / 2, r);
      mtx.compose(p, qn, s);
      crown.setMatrixAt(i, mtx);
      const tw = Math.max(0.12, 0.04 * h);
      p.set(...V(wx, wy, z + (h - ch) / 2));
      s.set(tw, Math.max(0.5, h - ch), tw);
      mtx.compose(p, qn, s);
      trunk.setMatrixAt(i, mtx);
    });
    grp.add(crown, trunk);
  }

  // signposts: a pole with the cross streets' names
  for (const sg of tile.s) {
    const x = ox + sg[0] / 10;
    const y = oy + sg[1] / 10;
    const z = gz(sg[0] / 10, sg[1] / 10);
    const names = sg
      .slice(2)
      .map((k) => tile.n[k])
      .filter(Boolean);
    if (!names.length) continue;
    const pole = new THREE.Mesh(
      new THREE.CylinderGeometry(0.05, 0.05, 3.6, 6),
      new THREE.MeshLambertMaterial({ color: C.pole }),
    );
    pole.position.set(...V(x, y, z + 1.8));
    grp.add(pole);
    const sprite = signSprite(names, C);
    sprite.position.set(...V(x, y, z + 4.6));
    grp.add(sprite);
  }
  return grp;
}

const signCache = new Map();
function signSprite(names, C) {
  const key = names.join("|");
  let entry = signCache.get(key);
  if (!entry) {
    const lines = names.slice(0, 3);
    const cv = document.createElement("canvas");
    const ctx = cv.getContext("2d");
    ctx.font = "600 44px system-ui, -apple-system, Segoe UI, sans-serif";
    const w = Math.max(...lines.map((l) => ctx.measureText(l).width)) + 48;
    cv.width = Math.ceil(w);
    cv.height = 64 * lines.length + 16;
    ctx.fillStyle = `#${C.sign.getHexString()}`;
    ctx.fillRect(0, 0, cv.width, cv.height);
    ctx.strokeStyle = "#ffffff";
    ctx.lineWidth = 4;
    ctx.strokeRect(6, 6, cv.width - 12, cv.height - 12);
    ctx.fillStyle = "#ffffff";
    ctx.font = "600 44px system-ui, -apple-system, Segoe UI, sans-serif";
    ctx.textBaseline = "middle";
    lines.forEach((l, i) => ctx.fillText(l, 24, 8 + 32 + 64 * i));
    const tex = new THREE.CanvasTexture(cv);
    tex.colorSpace = THREE.SRGBColorSpace;
    entry = { tex, aspect: cv.width / cv.height };
    signCache.set(key, entry);
  }
  const mat = new THREE.SpriteMaterial({ map: entry.tex, depthTest: true });
  const sp = new THREE.Sprite(mat);
  const h = 0.8 * Math.min(3, names.length) + 0.3;
  sp.scale.set(h * entry.aspect, h, 1);
  return sp;
}

function makeCar(C) {
  const car = new THREE.Group();
  const body = new THREE.Mesh(
    new THREE.BoxGeometry(4.4, 0.9, 1.85),
    new THREE.MeshLambertMaterial({ color: C.car }),
  );
  body.position.y = 0.7;
  const cab = new THREE.Mesh(
    new THREE.BoxGeometry(2.2, 0.7, 1.65),
    new THREE.MeshLambertMaterial({ color: C.carDark }),
  );
  cab.position.set(-0.3, 1.45, 0);
  car.add(body, cab);
  return car;
}

export async function startDrive({ canvas, base, hud }) {
  const manifest = await fetchJsonGz(`${base}manifest.json`);
  const C = {
    sky: col("--sky"),
    land: col("--land"),
    roof: col("--roof"),
    wall: col("--wall"),
    extra: col("--extra"),
    asphalt: col("--asphalt"),
    path: col("--path"),
    dash: col("--dash"),
    kerb: col("--kerb"),
    tree: col("--tree"),
    trunk: col("--trunk"),
    pole: col("--pole"),
    sign: col("--sign"),
    car: col("--car"),
    carDark: col("--car-dark"),
  };
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
  renderer.setPixelRatio(Math.min(2, window.devicePixelRatio || 1));
  const scene = new THREE.Scene();
  scene.background = C.sky;
  scene.fog = new THREE.Fog(C.sky, 350, 1300);
  scene.add(new THREE.HemisphereLight(0xffffff, 0x8a8f80, 1.9));
  const sun = new THREE.DirectionalLight(0xffffff, 1.7);
  sun.position.set(-0.4, 1, 0.3);
  scene.add(sun);
  const camera = new THREE.PerspectiveCamera(62, 1, 0.3, 2500);
  const mirrorCam = new THREE.PerspectiveCamera(48, 3.2, 0.3, 900);
  const carMesh = makeCar(C);
  scene.add(carMesh);

  const places = manifest.places || [];
  const start =
    places.find(
      (p) => p.id === (new URLSearchParams(location.search).get("at") || ""),
    ) || places[0];
  let car = { x: start.x, y: start.y, heading: start.heading, v: 0 };
  let carZ = 0;
  const loaded = new Map(); // token -> {tile, grp}
  const loading = new Set();
  const byToken = new Map(manifest.tiles.map((t) => [t.t, t]));
  const state = {
    mirror: true,
    view: "chase",
    keys: new Set(),
    touch: { throttle: 0, steer: 0, brake: false },
  };

  function stream() {
    const { want, drop } = tilesAround(manifest.tiles, car.x, car.y);
    for (const t of drop) {
      const e = loaded.get(t);
      if (!e) continue;
      scene.remove(e.grp);
      e.grp.traverse((o) => {
        if (o.geometry) o.geometry.dispose();
        if (o.material && !o.isSprite) o.material.dispose();
        if (o.isSprite) o.material.dispose();
      });
      loaded.delete(t);
    }
    for (const t of want) {
      if (loaded.has(t) || loading.has(t) || loading.size >= 3) continue;
      loading.add(t);
      fetchJsonGz(`${base}${t}.json.gz`)
        .then((tile) => {
          tile.half = byToken.get(t).half;
          const grp = buildTile(tile, C);
          scene.add(grp);
          loaded.set(t, { tile, grp });
        })
        .catch((e) => console.warn("tile", t, e))
        .finally(() => loading.delete(t));
    }
    hud.tiles.textContent = `${loaded.size} tiles`;
  }

  function resize() {
    const r = canvas.getBoundingClientRect();
    renderer.setSize(r.width, r.height, false);
    camera.aspect = r.width / r.height;
    camera.updateProjectionMatrix();
  }
  window.addEventListener("resize", resize);
  resize();

  const keyMap = {
    ArrowUp: "up",
    KeyW: "up",
    ArrowDown: "down",
    KeyS: "down",
    ArrowLeft: "left",
    KeyA: "left",
    ArrowRight: "right",
    KeyD: "right",
    Space: "brake",
  };
  window.addEventListener("keydown", (e) => {
    if (e.target.closest && e.target.closest("select, input, button")) return;
    if (keyMap[e.code]) {
      state.keys.add(keyMap[e.code]);
      e.preventDefault();
    }
    if (e.code === "KeyM") toggleMirror();
    if (e.code === "KeyV") toggleView();
    if (e.code === "KeyR") reset();
  });
  window.addEventListener(
    "keyup",
    (e) => keyMap[e.code] && state.keys.delete(keyMap[e.code]),
  );
  const toggleMirror = () => {
    state.mirror = !state.mirror;
    hud.mirrorFrame.hidden = !state.mirror;
    hud.mirrorBtn.setAttribute("aria-pressed", String(state.mirror));
  };
  const toggleView = () => {
    state.view = state.view === "chase" ? "high" : "chase";
    hud.viewBtn.setAttribute("aria-pressed", String(state.view === "high"));
  };
  const reset = (p = start) => {
    car = { x: p.x, y: p.y, heading: p.heading, v: 0 };
  };
  hud.mirrorBtn.addEventListener("click", toggleMirror);
  hud.viewBtn.addEventListener("click", toggleView);
  hud.places.innerHTML = places
    .map((p) => `<option value="${p.id}">${p.name}</option>`)
    .join("");
  hud.places.value = start.id;
  hud.places.addEventListener("change", () => {
    const p = places.find((q) => q.id === hud.places.value);
    if (p) {
      reset(p);
      history.replaceState(null, "", `?at=${p.id}`);
    }
  });
  for (const b of hud.touchButtons) {
    const act = b.dataset.act;
    const on = (v) => (e) => {
      e.preventDefault();
      if (act === "brake") state.touch.brake = v;
      if (act === "up") state.touch.throttle = v ? 1 : 0;
      if (act === "down") state.touch.throttle = v ? -1 : 0;
      if (act === "left") state.touch.steer = v ? 1 : 0;
      if (act === "right") state.touch.steer = v ? -1 : 0;
    };
    b.addEventListener("pointerdown", on(true));
    b.addEventListener("pointerup", on(false));
    b.addEventListener("pointerleave", on(false));
  }

  let last = performance.now();
  let hudAt = 0;
  let streamAt = 0;
  const camPos = new THREE.Vector3();
  const camLook = new THREE.Vector3();
  let camInit = false;
  function frame(now) {
    requestAnimationFrame(frame);
    const dt = Math.min(0.05, (now - last) / 1000);
    last = now;
    const k = state.keys;
    const input = {
      throttle:
        (k.has("up") ? 1 : 0) - (k.has("down") ? 1 : 0) + state.touch.throttle,
      steer:
        (k.has("left") ? 1 : 0) - (k.has("right") ? 1 : 0) + state.touch.steer,
      brake: k.has("brake") || state.touch.brake,
    };
    car = carStep(car, input, dt);
    const tiles = [...loaded.values()].map((e) => e.tile);
    const z = groundAtWorld(tiles, car.x, car.y);
    if (z !== null) carZ = z;
    carMesh.position.set(...V(car.x, car.y, carZ));
    carMesh.rotation.set(0, car.heading, 0);

    const hx = Math.cos(car.heading);
    const hy = Math.sin(car.heading);
    const back = state.view === "chase" ? 10 : 34;
    const up = state.view === "chase" ? 3.6 : 26;
    const bx = car.x - hx * back;
    const by = car.y - hy * back;
    const bz = Math.max(groundAtWorld(tiles, bx, by) ?? carZ, carZ) + up;
    const target = new THREE.Vector3(...V(bx, by, bz));
    const look = new THREE.Vector3(
      ...V(car.x + hx * 14, car.y + hy * 14, carZ + 1.4),
    );
    if (!camInit) {
      camPos.copy(target);
      camLook.copy(look);
      camInit = true;
    } else {
      camPos.lerp(target, 0.12);
      camLook.lerp(look, 0.2);
    }
    camera.position.copy(camPos);
    camera.lookAt(camLook);

    const W = renderer.domElement.width;
    const H = renderer.domElement.height;
    const pr = renderer.getPixelRatio();
    renderer.setScissorTest(false);
    renderer.setViewport(0, 0, W / pr, H / pr);
    renderer.render(scene, camera);
    if (state.mirror) {
      // the mirror: from the cabin roof, looking back over the boot
      mirrorCam.position.set(
        ...V(car.x - hx * 0.4, car.y - hy * 0.4, carZ + 1.9),
      );
      mirrorCam.lookAt(...V(car.x - hx * 30, car.y - hy * 30, carZ + 1.2));
      const mw = Math.min(0.36 * W, 520 * pr);
      const mh = mw / 3.2;
      const mx = (W - mw) / 2;
      const my = H - mh - 14 * pr;
      mirrorCam.aspect = mw / mh;
      mirrorCam.updateProjectionMatrix();
      renderer.setScissorTest(true);
      renderer.setScissor(mx / pr, my / pr, mw / pr, mh / pr);
      renderer.setViewport(mx / pr, my / pr, mw / pr, mh / pr);
      carMesh.visible = false; // the view behind, not the inside of the cabin
      renderer.render(scene, mirrorCam);
      carMesh.visible = true;
      renderer.setScissorTest(false);
      Object.assign(hud.mirrorFrame.style, {
        width: `${mw / pr}px`,
        height: `${mh / pr}px`,
        left: `${mx / pr}px`,
        top: "14px",
      });
    }

    if (now - streamAt > 400) {
      streamAt = now;
      stream();
    }
    if (now - hudAt > 250) {
      hudAt = now;
      const street = streetAt(tiles, car.x, car.y, car.heading);
      const next = nextCrossing(tiles, car.x, car.y, car.heading, street);
      hud.street.textContent = street || "Off street";
      hud.next.textContent = next
        ? `${next.names.join(" · ")} in ${Math.round(next.distance / 10) * 10} m`
        : "";
      hud.speed.textContent = `${Math.round(Math.abs(car.v) * 2.23694)} mph`;
      const deg = (((90 - (car.heading * 180) / Math.PI) % 360) + 360) % 360;
      hud.compass.textContent = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"][
        Math.round(deg / 45) % 8
      ];
      if (!loaded.size) hud.street.textContent = "Loading the city around you…";
    }
  }
  stream();
  requestAnimationFrame(frame);
}
