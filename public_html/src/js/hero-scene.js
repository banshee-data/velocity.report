import * as THREE from 'three';

// =============================================================================
// Stationary roadside LiDAR background — v1 aesthetic restored.
// Static scene baked once (street + buildings + parked cars + trees + lampposts).
// Tracked road users move through and get corner-bracket bounding boxes with
// AV-style sprite labels.
// =============================================================================

const canvas = document.getElementById('hero-scene');
// Bail out cleanly if the canvas isn't on the page (e.g. on a content page that
// happens to import this script).
if (!canvas) {
  throw new Error('hero-scene: no canvas');
}
const renderer = new THREE.WebGLRenderer({ canvas, antialias:false, alpha:false, powerPreference:'high-performance' });
renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75));
renderer.setClearColor(0x06090e, 1);

const scene = new THREE.Scene();
scene.fog = new THREE.Fog(0x06090e, 14, 62);

const camera = new THREE.PerspectiveCamera(54, 1, 0.1, 220);
const CAM_BASE = new THREE.Vector3(-2.6, 2.6, 6.2);
camera.position.copy(CAM_BASE);
camera.lookAt(0, 1.2, -22);

function resize(){
  const w = canvas.clientWidth, h = canvas.clientHeight;
  renderer.setSize(w, h, false);
  camera.aspect = w/h;
  camera.updateProjectionMatrix();
}
resize();
window.addEventListener('resize', resize);

// ---------- palette ----------
const COL_ROAD    = new THREE.Color(0x2a3a4a);
const COL_LANE    = new THREE.Color(0xa8c0d4);
const COL_WALK    = new THREE.Color(0x3a4a5a);
const COL_BLDG    = new THREE.Color(0x35495e);
const COL_PARKED  = new THREE.Color(0x5a7a90);
const COL_TREE    = new THREE.Color(0x3a6a52);

// ---------- scene geometry params ----------
const ROAD_HALF_W = 4.2;
const WALK_W      = 2.2;
const Z_NEAR      = 10;
const Z_FAR       = -85;

// ---------- STATIC POINT CLOUD ----------
const STATIC_COUNT = 70000;
const sPos = new Float32Array(STATIC_COUNT * 3);
const sCol = new Float32Array(STATIC_COUNT * 3);
const sSize = new Float32Array(STATIC_COUNT);
const sIntensity = new Float32Array(STATIC_COUNT);
let sp = 0;
const tmp = new THREE.Color();

function pushStatic(x,y,z, color, size, shadeJitter=0.25){
  if (sp >= STATIC_COUNT) return;
  sPos[sp*3+0]=x; sPos[sp*3+1]=y; sPos[sp*3+2]=z;
  const s = 1 - shadeJitter*0.5 + Math.random()*shadeJitter;
  tmp.copy(color).multiplyScalar(s);
  sCol[sp*3+0]=tmp.r; sCol[sp*3+1]=tmp.g; sCol[sp*3+2]=tmp.b;
  sSize[sp] = size;
  sIntensity[sp] = Math.random();
  sp++;
}

function sampleRoad(n){
  for (let i=0;i<n;i++){
    const z = Z_NEAR + Math.random() * (Z_FAR - Z_NEAR);
    const x = (Math.random()*2 - 1) * ROAD_HALF_W;
    const y = -0.01 * Math.abs(x);
    const onCenter = Math.abs(x) < 0.06 && (Math.floor(z*0.5) % 2 === 0);
    const onEdge   = Math.abs(Math.abs(x) - (ROAD_HALF_W - 0.25)) < 0.05;
    if (onCenter || onEdge) pushStatic(x, y, z, COL_LANE, 1.4, 0.15);
    else                    pushStatic(x, y, z, COL_ROAD, 1.1, 0.4);
  }
}

function sampleSidewalk(n){
  for (let i=0;i<n;i++){
    const z = Z_NEAR + Math.random() * (Z_FAR - Z_NEAR);
    const side = Math.random() < 0.5 ? -1 : 1;
    const off = ROAD_HALF_W + 0.15 + Math.random() * WALK_W;
    pushStatic(side*off, 0.12, z, COL_WALK, 1.0, 0.3);
  }
}

function sampleCurb(n){
  for (let i=0;i<n;i++){
    const z = Z_NEAR + Math.random() * (Z_FAR - Z_NEAR);
    const side = Math.random() < 0.5 ? -1 : 1;
    pushStatic(side*(ROAD_HALF_W + 0.05 + Math.random()*0.1), 0.06 + Math.random()*0.06, z, COL_LANE, 1.0, 0.2);
  }
}

function sampleBox(cx,cy,cz, sx,sy,sz, n, color, size=1.2, jitter=0.025){
  for (let i=0;i<n;i++){
    const face = Math.floor(Math.random()*6);
    let lx=(Math.random()-0.5)*sx, ly=(Math.random()-0.5)*sy, lz=(Math.random()-0.5)*sz;
    if (face===0) lx= sx/2; else if (face===1) lx=-sx/2;
    else if (face===2) ly= sy/2; else if (face===3) ly=-sy/2;
    else if (face===4) lz= sz/2; else lz=-sz/2;
    pushStatic(cx+lx+(Math.random()-0.5)*jitter,
               cy+ly+(Math.random()-0.5)*jitter,
               cz+lz+(Math.random()-0.5)*jitter,
               color, size, 0.35);
  }
}

function sampleCylinder(cx,cz, r, y0, h, n, color){
  for (let i=0;i<n;i++){
    const a = Math.random()*Math.PI*2;
    pushStatic(cx + Math.cos(a)*r, y0 + Math.random()*h, cz + Math.sin(a)*r, color, 1.05, 0.3);
  }
}

function sampleSphere(cx,cy,cz, r, n, color){
  for (let i=0;i<n;i++){
    const a = Math.random()*Math.PI*2;
    const b = Math.acos(2*Math.random()-1);
    const rr = r * (0.85 + Math.random()*0.15);
    pushStatic(cx + rr*Math.sin(b)*Math.cos(a),
               cy + rr*Math.cos(b),
               cz + rr*Math.sin(b)*Math.sin(a),
               color, 1.1, 0.4);
  }
}

sampleRoad(20000);
sampleSidewalk(9000);
sampleCurb(4500);

// Continuous facade walls along each side (no random scatter — buildings share
// a fixed building line)
const FACADE_LEFT_X  = -(ROAD_HALF_W + WALK_W + 0.3);
const FACADE_RIGHT_X =  (ROAD_HALF_W + WALK_W + 0.3);
const LOT_DEPTH      = 7.0;
const STOREY_H       = 3.2;

function buildFacade(side){
  const faceX = side === 'left' ? FACADE_LEFT_X : FACADE_RIGHT_X;
  const outward = side === 'left' ? -1 : +1;
  let z = Z_NEAR;
  while (z > Z_FAR){
    const wide = Math.random() < 0.18;
    const lotW = wide ? (6 + Math.random()*4) : (3 + Math.random()*2.5);
    const storeys = Math.random() < 0.15 ? (1 + Math.floor(Math.random()*2))
                    : Math.random() < 0.7 ? (2 + Math.floor(Math.random()*2))
                    :                       (4 + Math.floor(Math.random()*3));
    const h = storeys * STOREY_H;
    const setback = (Math.random() - 0.5) * 0.5;
    const cx = faceX + outward * (LOT_DEPTH/2 + setback);
    const cz = z - lotW/2;
    sampleBox(cx, h/2, cz, LOT_DEPTH, h, lotW, 240, COL_BLDG, 1.2, 0.04);
    if (storeys >= 4 && Math.random() < 0.5){
      const parapetW = lotW * (0.4 + Math.random()*0.3);
      const parapetH = 0.6 + Math.random()*0.8;
      sampleBox(cx + outward * 0.3, h + parapetH/2, cz, LOT_DEPTH - 0.6, parapetH, parapetW, 30, COL_BLDG, 1.1, 0.04);
    }
    const gap = Math.random() < 0.12 ? (1.2 + Math.random()*1.2) : 0.0;
    z -= lotW + gap;
  }
}
buildFacade('left');
buildFacade('right');

// Parked cars on the far curb (right side from camera)
for (const pz of [-6, -14, -22, -34, -46, -58, -70]){
  sampleBox(ROAD_HALF_W - 0.95, 0.75, pz, 1.85, 1.5, 4.3, 260, COL_PARKED, 1.15, 0.025);
}
for (const pz of [-10, -38, -64]){
  sampleBox(-(ROAD_HALF_W - 0.95), 0.75, pz, 1.85, 1.5, 4.3, 220, COL_PARKED, 1.15, 0.025);
}

// Lampposts (alternating sides)
for (let z = Z_NEAR - 5; z > Z_FAR; z -= 14){
  const side = (Math.floor(z/14) % 2 === 0) ? -1 : 1;
  const x = side * (ROAD_HALF_W + WALK_W*0.5);
  sampleCylinder(x, z, 0.07, 0, 5.2, 100, COL_LANE);
  sampleBox(x + side*0.3, 5.0, z, 0.4, 0.2, 0.2, 30, COL_LANE, 1.2, 0.02);
}

// Trees
for (let z = Z_NEAR - 9; z > Z_FAR; z -= 18){
  const side = (Math.floor(z/18) % 2 === 0) ? 1 : -1;
  const x = side * (ROAD_HALF_W + WALK_W*0.6);
  sampleCylinder(x, z, 0.18, 0, 2.4, 70, COL_LANE);
  sampleSphere(x, 3.6, z, 1.6, 320, COL_TREE);
}

while (sp < STATIC_COUNT){
  const z = Z_NEAR + Math.random() * (Z_FAR - Z_NEAR);
  pushStatic((Math.random()*2-1)*16, Math.random()*6, z, COL_BLDG, 0.85, 0.3);
}

const staticGeom = new THREE.BufferGeometry();
staticGeom.setAttribute('position',  new THREE.BufferAttribute(sPos, 3));
staticGeom.setAttribute('color',     new THREE.BufferAttribute(sCol, 3));
staticGeom.setAttribute('size',      new THREE.BufferAttribute(sSize, 1));
staticGeom.setAttribute('intensity', new THREE.BufferAttribute(sIntensity, 1));

// ---------- MOVING OBJECTS ----------
const TYPES = {
  car:    { sx:1.85, sy:1.5,  sz:4.3,  density:280, color:'#4cd1a8', class:'vehicle'    },
  truck:  { sx:2.2,  sy:2.5,  sz:6.5,  density:380, color:'#4cd1a8', class:'vehicle'    },
  bike:   { sx:0.55, sy:1.55, sz:1.7,  density: 90, color:'#6aa9ff', class:'cyclist'    },
  ped:    { sx:0.5,  sy:1.75, sz:0.45, density: 70, color:'#ffb050', class:'pedestrian' },
};

// =============================================================================
// LOCAL CLUSTER GENERATORS — type-specific anatomical shapes
// Each object type uses a different point distribution so it reads as itself
// rather than a generic box:
//   - car/truck: silhouette of stacked body + cabin + wheels + mirrors
//   - pedestrian: head + torso + two legs
//   - cyclist: person bent over bike + two wheels + frame
// Object-local frame: +Z = forward (direction of motion), +Y = up, +X = right.
// Local Y=0 is the vertical CENTER of the object's bounding box, so the bottom
// is at Y = -sy/2 (where wheels sit / feet plant).
// =============================================================================

// ---- shape primitives. All write into `arr` starting at index `i*3`. ----
// Each returns the new index after writing `n` points.

function sBoxFace(arr, i, cx, cy, cz, sx, sy, sz, n, jitter=0.03){
  // n points distributed across all 6 faces of a centered box
  for (let k=0;k<n;k++,i++){
    const face = Math.floor(Math.random()*6);
    let lx=(Math.random()-0.5)*sx, ly=(Math.random()-0.5)*sy, lz=(Math.random()-0.5)*sz;
    if      (face===0) lx= sx/2; else if (face===1) lx=-sx/2;
    else if (face===2) ly= sy/2; else if (face===3) ly=-sy/2;
    else if (face===4) lz= sz/2; else lz=-sz/2;
    arr[i*3+0] = cx + lx + (Math.random()-0.5)*jitter;
    arr[i*3+1] = cy + ly + (Math.random()-0.5)*jitter;
    arr[i*3+2] = cz + lz + (Math.random()-0.5)*jitter;
  }
  return i;
}

function sSphere(arr, i, cx, cy, cz, r, n, jitter=0.02){
  // n points on a sphere surface
  for (let k=0;k<n;k++,i++){
    const u = Math.random()*Math.PI*2;
    const v = Math.acos(2*Math.random()-1);
    const rr = r * (0.92 + Math.random()*0.16);
    arr[i*3+0] = cx + rr*Math.sin(v)*Math.cos(u) + (Math.random()-0.5)*jitter;
    arr[i*3+1] = cy + rr*Math.cos(v)              + (Math.random()-0.5)*jitter;
    arr[i*3+2] = cz + rr*Math.sin(v)*Math.sin(u) + (Math.random()-0.5)*jitter;
  }
  return i;
}

function sCylY(arr, i, cx, cy0, cz, r, h, n, jitter=0.02){
  // n points on a vertical cylinder shell, base at cy0, height h
  for (let k=0;k<n;k++,i++){
    const a = Math.random()*Math.PI*2;
    arr[i*3+0] = cx + Math.cos(a)*r + (Math.random()-0.5)*jitter;
    arr[i*3+1] = cy0 + Math.random()*h + (Math.random()-0.5)*jitter;
    arr[i*3+2] = cz + Math.sin(a)*r + (Math.random()-0.5)*jitter;
  }
  return i;
}

function sRingX(arr, i, cx, cy, cz, r, n, jitter=0.02){
  // n points on a ring in the YZ plane (wheel mounted on a left/right axle —
  // wheel disc faces sideways, so the ring is vertical in YZ)
  for (let k=0;k<n;k++,i++){
    const a = Math.random()*Math.PI*2;
    const rr = r * (0.93 + Math.random()*0.14);
    arr[i*3+0] = cx + (Math.random()-0.5)*jitter*2;
    arr[i*3+1] = cy + Math.cos(a)*rr + (Math.random()-0.5)*jitter;
    arr[i*3+2] = cz + Math.sin(a)*rr + (Math.random()-0.5)*jitter;
  }
  return i;
}

function sLine(arr, i, ax, ay, az, bx, by, bz, n, jitter=0.025){
  // n points along a line segment (used for thin features: bike frame tubes,
  // handlebars, side mirrors)
  for (let k=0;k<n;k++,i++){
    const t = Math.random();
    arr[i*3+0] = ax + (bx-ax)*t + (Math.random()-0.5)*jitter;
    arr[i*3+1] = ay + (by-ay)*t + (Math.random()-0.5)*jitter;
    arr[i*3+2] = az + (bz-az)*t + (Math.random()-0.5)*jitter;
  }
  return i;
}

// ---- per-type local cluster builders ----

function localCar(t){
  // density 280 budget:
  //   lower body (hood/doors/trunk): 110
  //   cabin (windshield+roof+rear window): 90
  //   4 wheels: 12 each = 48
  //   2 side mirrors: 5 each = 10
  //   front/rear bumper detail: 22
  const arr = new Float32Array(t.density * 3);
  let i = 0;
  const bottom = -t.sy/2;            // ~ -0.75 (wheels/ground)
  const beltline = bottom + 0.92;    // top of doors / bottom of windows
  const roof = bottom + 1.45;        // roof top
  // Lower body: a box from bottom to beltline, full length & width
  i = sBoxFace(arr, i, 0, (bottom+beltline)/2, 0,
               t.sx*0.98, beltline-bottom, t.sz*0.98, 110, 0.025);
  // Cabin: a slightly narrower, shorter-in-length box on top
  i = sBoxFace(arr, i, 0, (beltline+roof)/2, -t.sz*0.05,
               t.sx*0.85, roof-beltline, t.sz*0.52, 90, 0.025);
  // Wheels: 4 vertical rings near the corners, just above ground
  const wheelR = 0.32;
  const wheelY = bottom + wheelR;
  const wxOff  = t.sx/2 + 0.02;
  const wzFront = -t.sz/2 + 0.7;
  const wzRear  =  t.sz/2 - 0.7;
  i = sRingX(arr, i, -wxOff, wheelY, wzFront, wheelR, 12);
  i = sRingX(arr, i,  wxOff, wheelY, wzFront, wheelR, 12);
  i = sRingX(arr, i, -wxOff, wheelY, wzRear,  wheelR, 12);
  i = sRingX(arr, i,  wxOff, wheelY, wzRear,  wheelR, 12);
  // Side mirrors: small bumps at front-of-cabin height, sticking out laterally
  const mirY = beltline + 0.15;
  const mirZ = -t.sz*0.18;
  i = sLine(arr, i, -t.sx/2,       mirY, mirZ, -t.sx/2 - 0.18, mirY, mirZ, 5);
  i = sLine(arr, i,  t.sx/2,       mirY, mirZ,  t.sx/2 + 0.18, mirY, mirZ, 5);
  // Bumper detail to round out remaining budget
  while (i < t.density){
    const front = Math.random() < 0.5;
    const cz = front ? -t.sz/2 : t.sz/2;
    i = sBoxFace(arr, i, 0, bottom + 0.35, cz, t.sx*0.9, 0.18, 0.06, 1, 0.02);
  }
  return arr;
}

function localTruck(t){
  // density 380 budget for a delivery/box truck (cab + cargo box):
  //   cab: 90
  //   cargo box: 180
  //   6 wheels: 14 each = 84
  //   2 mirrors: 8 each = 16
  //   remaining: bumper / details
  const arr = new Float32Array(t.density * 3);
  let i = 0;
  const bottom = -t.sy/2;
  const top = t.sy/2;
  const halfZ = t.sz/2;
  // Cab: front 28% of length, full height
  const cabLen = t.sz * 0.28;
  const cabFront = -halfZ;
  const cabBack  = cabFront + cabLen;
  i = sBoxFace(arr, i, 0, 0, (cabFront+cabBack)/2,
               t.sx*0.95, t.sy*0.82, cabLen, 90, 0.03);
  // Cargo box: back 68% (with a 4% gap from cab)
  const cargoFront = cabBack + t.sz*0.04;
  const cargoBack  = halfZ;
  i = sBoxFace(arr, i, 0, t.sy*0.06, (cargoFront+cargoBack)/2,
               t.sx, t.sy*0.92, cargoBack-cargoFront, 180, 0.025);
  // Wheels: 1 front axle, 2 rear axles (typical box truck)
  const wheelR = 0.42;
  const wheelY = bottom + wheelR;
  const wxOff  = t.sx/2 + 0.02;
  const wzFrontAxle = cabFront + cabLen*0.5;
  const wzRearAxle1 = cargoBack - 0.8;
  const wzRearAxle2 = cargoBack - 1.8;
  for (const wz of [wzFrontAxle, wzRearAxle1, wzRearAxle2]){
    i = sRingX(arr, i, -wxOff, wheelY, wz, wheelR, 14);
    i = sRingX(arr, i,  wxOff, wheelY, wz, wheelR, 14);
  }
  // Side mirrors on the cab (large truck mirrors)
  const mirY = top - 0.2;
  const mirZ = cabFront + 0.3;
  i = sLine(arr, i, -t.sx/2, mirY, mirZ, -t.sx/2 - 0.28, mirY-0.3, mirZ, 8);
  i = sLine(arr, i,  t.sx/2, mirY, mirZ,  t.sx/2 + 0.28, mirY-0.3, mirZ, 8);
  // Fill remaining with bumper/edge detail
  while (i < t.density){
    i = sBoxFace(arr, i, 0, bottom + 0.3, -halfZ, t.sx*0.9, 0.22, 0.08, 1, 0.02);
  }
  return arr;
}

function localPed(t){
  // density 70 budget for a pedestrian (anthropomorphic):
  //   head: 10
  //   torso: 25
  //   2 legs: 12 each = 24
  //   2 arms (thin): 4 each = 8
  //   misc: 3
  const arr = new Float32Array(t.density * 3);
  let i = 0;
  const bottom = -t.sy/2;            // foot level
  // Proportions for a ~1.75m tall person:
  //   legs: 0..0.85 from bottom
  //   torso: 0.85..1.55
  //   head: 1.55..1.75
  const legTop  = bottom + 0.85;
  const torsoTop = bottom + 1.55;
  const headCY  = bottom + 1.65;
  // Head: small sphere
  i = sSphere(arr, i, 0, headCY, 0, 0.10, 10);
  // Torso: a slightly tapered box (shoulders wider than waist; we approximate
  // with a single box at shoulder width)
  i = sBoxFace(arr, i, 0, (legTop+torsoTop)/2, 0, 0.40, torsoTop-legTop, 0.22, 25, 0.02);
  // Legs: two thin vertical cylinders, separated laterally
  i = sCylY(arr, i, -0.08, bottom, 0, 0.07, legTop-bottom, 12);
  i = sCylY(arr, i,  0.08, bottom, 0, 0.07, legTop-bottom, 12);
  // Arms: thin lines hanging down from shoulders
  const shoulderY = torsoTop - 0.05;
  const shoulderX = 0.20;
  i = sLine(arr, i, -shoulderX, shoulderY, 0, -shoulderX - 0.04, shoulderY - 0.55, 0, 4);
  i = sLine(arr, i,  shoulderX, shoulderY, 0,  shoulderX + 0.04, shoulderY - 0.55, 0, 4);
  // misc filler if needed
  while (i < t.density){
    i = sSphere(arr, i, 0, headCY, 0, 0.10, 1);
  }
  return arr;
}

function localBike(t){
  // density 90 budget for a cyclist (person + bike):
  //   front wheel ring: 16
  //   rear wheel ring: 16
  //   bike frame (top tube + down tube + seat post + chainstay): 14
  //   handlebars: 4
  //   person torso (bent forward): 22
  //   person head: 8
  //   person arms+legs: 10
  const arr = new Float32Array(t.density * 3);
  let i = 0;
  const bottom = -t.sy/2;
  const wheelR = 0.34;
  const wheelY = bottom + wheelR;
  const wzFront = -t.sz/2 + wheelR + 0.05;
  const wzRear  =  t.sz/2 - wheelR - 0.05;
  // Wheels (vertical rings perpendicular to motion)
  i = sRingX(arr, i, 0, wheelY, wzFront, wheelR, 16);
  i = sRingX(arr, i, 0, wheelY, wzRear,  wheelR, 16);
  // Bike frame: down tube from front hub up to head tube; top tube to seat
  const headTubeY = wheelY + 0.55;       // top of front fork
  const seatY     = wheelY + 0.70;       // saddle height
  const bbY       = wheelY + 0.15;       // bottom bracket
  // down tube (head tube down to BB)
  i = sLine(arr, i, 0, headTubeY, wzFront + 0.05, 0, bbY, (wzFront+wzRear)/2, 4);
  // top tube (head tube back to seat post top)
  i = sLine(arr, i, 0, headTubeY, wzFront + 0.05, 0, seatY, wzRear - 0.15, 4);
  // seat post
  i = sLine(arr, i, 0, bbY, wzRear - 0.15, 0, seatY, wzRear - 0.15, 3);
  // chainstay (BB to rear hub)
  i = sLine(arr, i, 0, bbY, (wzFront+wzRear)/2, 0, wheelY, wzRear, 3);
  // Handlebars: short horizontal line at head tube top
  i = sLine(arr, i, -0.22, headTubeY + 0.08, wzFront + 0.05,
                    0.22, headTubeY + 0.08, wzFront + 0.05, 4);
  // Cyclist: bent forward over the bike. Torso angles from seat up-and-forward
  // to a position above the handlebars. We sample along that angled volume.
  const torsoStartY = seatY + 0.05;
  const torsoStartZ = wzRear - 0.15;
  const torsoEndY   = headTubeY + 0.35;
  const torsoEndZ   = wzFront + 0.10;
  // 22 points along the torso, jittered laterally for body width
  for (let k=0;k<22;k++,i++){
    const u = Math.random();
    const cy = torsoStartY + (torsoEndY - torsoStartY) * u;
    const cz = torsoStartZ + (torsoEndZ - torsoStartZ) * u;
    const cx = (Math.random()-0.5) * 0.28;
    arr[i*3+0] = cx + (Math.random()-0.5)*0.03;
    arr[i*3+1] = cy + (Math.random()-0.5)*0.04;
    arr[i*3+2] = cz + (Math.random()-0.5)*0.04;
  }
  // Head: above torso end
  i = sSphere(arr, i, 0, torsoEndY + 0.15, torsoEndZ - 0.05, 0.10, 8);
  // Arms reaching forward to handlebars (thin)
  i = sLine(arr, i, -0.14, torsoStartY + 0.1, torsoStartZ + 0.2,
                   -0.20, headTubeY + 0.08, wzFront + 0.05, 3);
  i = sLine(arr, i,  0.14, torsoStartY + 0.1, torsoStartZ + 0.2,
                    0.20, headTubeY + 0.08, wzFront + 0.05, 3);
  // Legs hint (pedaling — short lines from BB down)
  while (i < t.density){
    i = sLine(arr, i, 0, bbY, (wzFront+wzRear)/2 + (Math.random()-0.5)*0.3,
                      (Math.random()-0.5)*0.12, wheelY + 0.05, (wzFront+wzRear)/2 + (Math.random()-0.5)*0.3, 1);
  }
  return arr;
}

function makeLocalCluster(type){
  const t = TYPES[type];
  if (type === 'car')   return localCar(t);
  if (type === 'truck') return localTruck(t);
  if (type === 'ped')   return localPed(t);
  if (type === 'bike')  return localBike(t);
  // fallback
  const arr = new Float32Array(t.density * 3);
  sBoxFace(arr, 0, 0, 0, 0, t.sx, t.sy, t.sz, t.density, 0.04);
  return arr;
}

const MAX_TRACKED = 8;
const MOVING_CAPACITY = Math.max(...Object.values(TYPES).map(t=>t.density)) * MAX_TRACKED;
const mPos = new Float32Array(MOVING_CAPACITY * 3);
const mCol = new Float32Array(MOVING_CAPACITY * 3);
const mSize = new Float32Array(MOVING_CAPACITY);
const mIntensity = new Float32Array(MOVING_CAPACITY);

const movingGeom = new THREE.BufferGeometry();
movingGeom.setAttribute('position',  new THREE.BufferAttribute(mPos, 3));
movingGeom.setAttribute('color',     new THREE.BufferAttribute(mCol, 3));
movingGeom.setAttribute('size',      new THREE.BufferAttribute(mSize, 1));
movingGeom.setAttribute('intensity', new THREE.BufferAttribute(mIntensity, 1));
movingGeom.setDrawRange(0, 0);

// ---------- shader: v1 atmospheric aesthetic (additive, fog, sweep band, warm→cool) ----------
const sharedUniforms = {
  uTime:       { value: 0 },
  uPixelRatio: { value: renderer.getPixelRatio() },
  uSweep:      { value: 0 },
  uSweepWidth: { value: 0.06 },
  uFogNear:    { value: scene.fog.near },
  uFogFar:     { value: scene.fog.far },
};

const pointVS = /* glsl */`
  attribute float size;
  attribute float intensity;
  varying vec3 vColor;
  varying float vDepth;
  varying float vWorldZ;
  uniform float uTime;
  uniform float uPixelRatio;
  void main(){
    vColor = color;
    vec4 mv = modelViewMatrix * vec4(position, 1.0);
    vDepth = -mv.z;
    vWorldZ = position.z;
    gl_Position = projectionMatrix * mv;
    // Distance-attenuated size with a floor (max 1/depth-scaling), so that
    // near-camera points don't balloon into huge bright blobs that compete
    // with the hero text. Cap matters more than the 1/z curve here.
    float s = size * (180.0 / max(2.5, -mv.z));
    s = min(s, 6.5);
    s *= 0.88 + 0.22 * sin(uTime*3.0 + intensity*28.0);
    gl_PointSize = s * uPixelRatio;
  }
`;
const pointFSStatic = /* glsl */`
  varying vec3 vColor;
  varying float vDepth;
  varying float vWorldZ;
  uniform float uSweep;
  uniform float uSweepWidth;
  uniform float uFogNear;
  uniform float uFogFar;
  void main(){
    vec2 uv = gl_PointCoord - 0.5;
    float d = length(uv);
    if (d > 0.5) discard;
    float alpha = smoothstep(0.5, 0.0, d);
    float fog = 1.0 - smoothstep(uFogNear, uFogFar, vDepth);
    alpha *= fog;
    float zN = clamp((10.0 - vWorldZ) / 95.0, 0.0, 1.0);
    float band = smoothstep(uSweepWidth, 0.0, abs(zN - uSweep));
    vec3 col = vColor + vec3(0.45, 0.85, 0.75) * band * 0.5;
    // Static cloud at moderate brightness — bright enough to read as an active
    // sensor scene, but not so bright it competes with hero text (the copy
    // column has its own dim plate that absorbs the extra cloud brightness).
    gl_FragColor = vec4(col * 0.72, alpha * 0.92);
  }
`;
const pointFSMoving = /* glsl */`
  varying vec3 vColor;
  varying float vDepth;
  varying float vWorldZ;
  uniform float uSweep;
  uniform float uSweepWidth;
  uniform float uFogNear;
  uniform float uFogFar;
  void main(){
    vec2 uv = gl_PointCoord - 0.5;
    float d = length(uv);
    if (d > 0.5) discard;
    float alpha = smoothstep(0.5, 0.0, d);
    float fog = 1.0 - smoothstep(uFogNear, uFogFar, vDepth);
    alpha *= fog;
    // Tracked objects keep full color saturation so they remain the visual
    // anchor of the scene even as the static cloud falls back.
    gl_FragColor = vec4(vColor, alpha);
  }
`;
const staticMat = new THREE.ShaderMaterial({
  vertexShader: pointVS, fragmentShader: pointFSStatic,
  uniforms: sharedUniforms, vertexColors:true,
  transparent:true, depthWrite:false, blending:THREE.AdditiveBlending,
});
const movingMat = new THREE.ShaderMaterial({
  vertexShader: pointVS, fragmentShader: pointFSMoving,
  uniforms: sharedUniforms, vertexColors:true,
  transparent:true, depthWrite:false, blending:THREE.AdditiveBlending,
});

scene.add(new THREE.Points(staticGeom, staticMat));
scene.add(new THREE.Points(movingGeom, movingMat));

// ---------- corner-bracket bounding box (v1 style) ----------
function makeCornerBox(w, h, d, color){
  const positions = [];
  const f = 0.28;
  function addEdge(a, b){
    const dx=b[0]-a[0], dy=b[1]-a[1], dz=b[2]-a[2];
    positions.push(a[0],a[1],a[2], a[0]+dx*f, a[1]+dy*f, a[2]+dz*f);
    positions.push(b[0],b[1],b[2], b[0]-dx*f, b[1]-dy*f, b[2]-dz*f);
  }
  const x=w/2, y=h/2, z=d/2;
  const C = [
    [-x,-y,-z],[ x,-y,-z],[ x, y,-z],[-x, y,-z],
    [-x,-y, z],[ x,-y, z],[ x, y, z],[-x, y, z]
  ];
  const E = [[0,1],[1,2],[2,3],[3,0],[4,5],[5,6],[6,7],[7,4],[0,4],[1,5],[2,6],[3,7]];
  for (const [a,b] of E) addEdge(C[a], C[b]);
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.Float32BufferAttribute(positions, 3));
  const m = new THREE.LineBasicMaterial({ color, transparent:true, opacity:0 });
  const seg = new THREE.LineSegments(g, m);
  const wire = new THREE.LineSegments(
    new THREE.EdgesGeometry(new THREE.BoxGeometry(w,h,d)),
    new THREE.LineBasicMaterial({ color, transparent:true, opacity:0 })
  );
  const group = new THREE.Group();
  group.add(wire); group.add(seg);
  group.userData = { bracketMat: m, wireMat: wire.material };
  return group;
}

// ---------- canvas-textured billboard label (AV-style) ----------
function makeLabel(accent){
  const c = document.createElement('canvas');
  c.width = 256; c.height = 72;
  const ctx = c.getContext('2d');
  const tex = new THREE.CanvasTexture(c);
  tex.minFilter = THREE.LinearFilter; tex.magFilter = THREE.LinearFilter;
  const mat = new THREE.SpriteMaterial({ map: tex, transparent:true, depthTest:false, depthWrite:false, opacity:0 });
  const sp = new THREE.Sprite(mat);
  sp.scale.set(2.3, 0.65, 1);
  sp.userData = { canvas:c, ctx, texture:tex, accent };
  return sp;
}

function drawLabel(sprite, id, klass, speedMph, conf){
  const { canvas, ctx, texture, accent } = sprite.userData;
  ctx.clearRect(0,0,canvas.width,canvas.height);
  // corner brackets
  ctx.strokeStyle = accent; ctx.lineWidth = 2;
  ctx.beginPath();
  ctx.moveTo(2, 18); ctx.lineTo(2, 2); ctx.lineTo(20, 2);
  ctx.moveTo(canvas.width-2, 18); ctx.lineTo(canvas.width-2, 2); ctx.lineTo(canvas.width-20, 2);
  ctx.moveTo(2, canvas.height-18); ctx.lineTo(2, canvas.height-2); ctx.lineTo(20, canvas.height-2);
  ctx.moveTo(canvas.width-2, canvas.height-18); ctx.lineTo(canvas.width-2, canvas.height-2); ctx.lineTo(canvas.width-20, canvas.height-2);
  ctx.stroke();
  // ID line
  ctx.fillStyle = accent;
  ctx.font = "600 22px 'IBM Plex Mono', ui-monospace, monospace";
  ctx.textBaseline = 'top';
  ctx.fillText(id, 14, 10);
  // meta line
  ctx.fillStyle = '#cfd6e0';
  ctx.font = "500 15px 'IBM Plex Mono', ui-monospace, monospace";
  ctx.fillText(`${klass}  ·  ${speedMph.toFixed(0)} mph  ·  ${(conf*100).toFixed(0)}%`, 14, 40);
  texture.needsUpdate = true;
}

// ---------- tracked-object pool ----------
const slots = [];
for (let i=0;i<MAX_TRACKED;i++){
  slots.push({
    active:false, type:null, id:'', conf:0,
    pose: { x:0, y:0, z:0, yaw:0 },
    vel: { x:0, z:0 },
    // --- naturalistic motion state ---
    dir:0,                  // -1 or +1 along z (constant over lifetime)
    cruiseSpeed:0,          // long-term target speed (m/s, positive scalar)
    targetSpeed:0,          // current goal (transient, varies)
    currentSpeed:0,         // signed scalar speed; sign matches dir
    accelMax:0,             // max acceleration / braking magnitude
    lane:0,                 // desired lateral position (world x)
    wobbleAmp:0, wobbleFreq:0, wobblePhase:0,  // sideways drift
    targetChangeAt:0,       // time (sec) to roll the next target-speed change
    principalYaw:0,         // base heading (0 or π based on dir)
    yawTarget:0,            // smoothed yaw goal
    // ----------------------------------
    local:null, bufCount:0,
    box:null, label:null, trail:null,
    trailIndex:0, life:0,
  });
}

const TRAIL_LEN = 60;
function makeTrail(color){
  const positions = new Float32Array(TRAIL_LEN * 3);
  const colors = new Float32Array(TRAIL_LEN * 3);
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  g.setAttribute('color',    new THREE.BufferAttribute(colors, 3));
  const m = new THREE.LineBasicMaterial({ vertexColors:true, transparent:true, opacity:0.8, depthWrite:false });
  return { line: new THREE.Line(g, m), positions, colors, geom:g, baseColor:new THREE.Color(color) };
}

function attachVisuals(slot){
  const t = TYPES[slot.type];
  slot.box = makeCornerBox(t.sx, t.sy, t.sz, new THREE.Color(t.color));
  scene.add(slot.box);
  slot.label = makeLabel(t.color);
  scene.add(slot.label);
  slot.trail = makeTrail(t.color);
  scene.add(slot.trail.line);
  slot.trailIndex = 0;
  for (let i=0;i<TRAIL_LEN;i++){
    slot.trail.positions[i*3+0] = slot.pose.x;
    slot.trail.positions[i*3+1] = 0.05;
    slot.trail.positions[i*3+2] = slot.pose.z;
  }
}

// ---------- spawn ----------
let nextIdSeq = { vehicle: 100, cyclist: 40, pedestrian: 70 };

function rollType(){
  const r = Math.random();
  if (r < 0.55) return 'car';
  if (r < 0.62) return 'truck';
  if (r < 0.78) return 'bike';
  return 'ped';
}

// Per-type motion behavior parameters. These shape how each object accelerates,
// wanders within its lane, and varies its target speed over time.
//   cruiseMin/Max: range of long-term cruise speeds (m/s)
//   accel:         max accel + brake magnitude (m/s²)
//   speedJitter:   how far target speed can deviate from cruise per change
//   wobbleAmp:     max lateral drift from lane center (m)
//   wobbleFreq:    sideways oscillation frequency (Hz)
//   followGap:     min headway in seconds (vehicles only; bikes/peds: 0)
//   minGap:        absolute min spacing in meters (vehicles only)
const BEHAVIOR = {
  car:   { cruiseMin:10, cruiseMax:14, accel:1.8, speedJitter:2.0, wobbleAmp:0.12, wobbleFreq:0.18, followGap:1.5, minGap:4.0 },
  truck: { cruiseMin: 6, cruiseMax: 9, accel:1.0, speedJitter:1.2, wobbleAmp:0.15, wobbleFreq:0.12, followGap:1.9, minGap:5.5 },
  bike:  { cruiseMin: 4, cruiseMax: 6, accel:1.5, speedJitter:1.0, wobbleAmp:0.20, wobbleFreq:0.45, followGap:0,   minGap:0    },
  ped:   { cruiseMin:1.2,cruiseMax:1.6,accel:0.6, speedJitter:0.3, wobbleAmp:0.05, wobbleFreq:0.30, followGap:0,   minGap:0    },
};

function spawn(slot){
  const type = rollType();
  const t = TYPES[type];
  const beh = BEHAVIOR[type];
  slot.type = type;
  slot.local = makeLocalCluster(type);
  slot.bufCount = t.density;
  const prefix = type === 'ped' ? 'PED' : type === 'bike' ? 'CYC' : 'VEH';
  const klass = t.class;
  nextIdSeq[klass]++;
  slot.id = `${prefix}-${String(nextIdSeq[klass]).padStart(3,'0')}`;

  const dir = Math.random() < 0.5 ? -1 : 1;
  slot.dir = dir;
  slot.pose.yaw = dir < 0 ? Math.PI : 0;
  slot.principalYaw = slot.pose.yaw;
  slot.yawTarget = slot.pose.yaw;

  // Long-term cruise speed and initial target = cruise
  slot.cruiseSpeed = beh.cruiseMin + Math.random() * (beh.cruiseMax - beh.cruiseMin);
  slot.targetSpeed = slot.cruiseSpeed;
  slot.accelMax = beh.accel;

  // Lateral wobble parameters
  slot.wobbleAmp = beh.wobbleAmp;
  slot.wobbleFreq = beh.wobbleFreq * (0.7 + Math.random()*0.6);
  slot.wobblePhase = Math.random() * Math.PI * 2;

  // First target-speed change in 2-6 seconds
  slot.targetChangeAt = 2 + Math.random()*4;

  if (type === 'ped'){
    const side = Math.random() < 0.5 ? -1 : 1;
    slot.lane = side * (ROAD_HALF_W + WALK_W*0.5 + (Math.random()-0.5)*0.4);
    slot.pose.x = slot.lane;
    slot.pose.y = t.sy/2 + 0.12;
    slot.pose.z = dir < 0 ? Z_NEAR + 2 : Z_FAR - 2;
  } else if (type === 'bike'){
    slot.lane = dir < 0 ? -(ROAD_HALF_W - 0.4) : (ROAD_HALF_W - 0.4);
    slot.pose.x = slot.lane;
    slot.pose.y = t.sy/2 + 0.05;
    slot.pose.z = dir < 0 ? Z_NEAR + 2 : Z_FAR - 2;
  } else {
    slot.lane = dir < 0 ? -1.9 : 1.9;
    slot.pose.x = slot.lane;
    slot.pose.y = t.sy/2 + 0.05;
    slot.pose.z = dir < 0 ? Z_NEAR + 4 : Z_FAR - 4;
  }

  // Start at cruise speed so there's no jarring zero-velocity entry
  slot.currentSpeed = slot.cruiseSpeed;
  slot.vel.x = 0;
  slot.vel.z = dir * slot.currentSpeed;

  slot.active = true;
  slot.conf = 0;
  slot.life = 0;
  attachVisuals(slot);
}

function disposeSlot(slot){
  if (slot.box){
    scene.remove(slot.box);
    slot.box.traverse(c=>{ c.geometry?.dispose?.(); c.material?.dispose?.(); });
    slot.box = null;
  }
  if (slot.label){
    scene.remove(slot.label);
    slot.label.material.map.dispose();
    slot.label.material.dispose();
    slot.label = null;
  }
  if (slot.trail){
    scene.remove(slot.trail.line);
    slot.trail.geom.dispose();
    slot.trail.line.material.dispose();
    slot.trail = null;
  }
  slot.active = false;
}

function despawnIfOutOfRange(slot){
  if (slot.pose.z > Z_NEAR + 4 || slot.pose.z < Z_FAR - 4) disposeSlot(slot);
}

// ---------- naturalistic motion update ----------
// Called once per slot per frame. Updates targetSpeed, currentSpeed, and
// computes lateral velocity from the lane + wobble + (TODO) avoidance terms.
// Then commits to slot.vel.{x,z}. The actual pose integration is done by the
// existing tick loop afterward.
function behave(slot, t, dt){
  const beh = BEHAVIOR[slot.type];

  // --- (1) periodic target-speed change ---
  if (t >= slot.targetChangeAt){
    // pick a new target speed: gaussian-ish around cruise, clipped to ±jitter
    const jitter = (Math.random() - Math.random()) * beh.speedJitter;
    slot.targetSpeed = Math.max(0.2, slot.cruiseSpeed + jitter);
    // next change in 3-9 seconds
    slot.targetChangeAt = t + 3 + Math.random()*6;
  }

  // --- (2) following: vehicles slow for leaders in the same lane ---
  if (beh.followGap > 0){
    let nearestLead = null;
    let nearestLeadGap = Infinity;
    for (const other of slots){
      if (other === slot || !other.active) continue;
      if (other.dir !== slot.dir) continue;
      // same lane? (within 1.2m laterally)
      if (Math.abs(other.lane - slot.lane) > 1.2) continue;
      // is it ahead?  "ahead" means farther along the direction of motion.
      // dir=+1 → moving toward +Z; ahead means other.z > self.z.
      // dir=-1 → moving toward -Z; ahead means other.z < self.z.
      const dz = (other.pose.z - slot.pose.z) * slot.dir;
      if (dz <= 0) continue;
      if (dz < nearestLeadGap){ nearestLeadGap = dz; nearestLead = other; }
    }
    if (nearestLead){
      const desiredGap = Math.abs(slot.currentSpeed) * beh.followGap + beh.minGap;
      if (nearestLeadGap < desiredGap){
        // Want to match leader's speed minus a margin proportional to how close we are
        const leadSpeed = Math.abs(nearestLead.currentSpeed);
        const closeness = 1 - Math.max(0, nearestLeadGap / desiredGap);  // 0..1
        // Aggressive braking when very close
        slot.targetSpeed = Math.max(0, leadSpeed - closeness * 2.0);
      }
    }
  }

  // --- (3) accelerate / decelerate toward targetSpeed ---
  const speedErr = slot.targetSpeed - Math.abs(slot.currentSpeed);
  const maxStep = beh.accel * dt;
  const step = Math.max(-maxStep, Math.min(maxStep, speedErr));
  const newSpeedMag = Math.max(0, Math.abs(slot.currentSpeed) + step);
  slot.currentSpeed = newSpeedMag * slot.dir;

  // --- (4) lateral motion: lane wobble + drift back toward lane center ---
  // wobble = low-freq sine around lane center
  const wobbleOffset = slot.wobbleAmp * Math.sin(t * slot.wobbleFreq * 2 * Math.PI + slot.wobblePhase);
  const desiredX = slot.lane + wobbleOffset;
  // smoothly approach desiredX with a critically-damped feel
  const dx = desiredX - slot.pose.x;
  // smooth approach: lateral speed is proportional to error, capped
  const lateralVel = Math.max(-0.6, Math.min(0.6, dx * 2.5));
  slot.vel.x = lateralVel;
  slot.vel.z = slot.currentSpeed;

  // --- (5) yaw aligns with velocity, lightly smoothed ---
  // World-space heading from velocity. For dir=-1, vz<0 → atan2(vx, vz) gives ~π
  // even with small vx, which is what we want (object pointing -Z with slight lean).
  if (Math.abs(slot.vel.z) > 0.05){
    const desiredYaw = Math.atan2(slot.vel.x, slot.vel.z);
    // shortest-arc lerp toward desiredYaw
    let delta = desiredYaw - slot.yawTarget;
    while (delta >  Math.PI) delta -= 2*Math.PI;
    while (delta < -Math.PI) delta += 2*Math.PI;
    slot.yawTarget += delta * Math.min(1, dt * 4);
    // soften further so the box doesn't twitch each frame
    let pdelta = slot.yawTarget - slot.pose.yaw;
    while (pdelta >  Math.PI) pdelta -= 2*Math.PI;
    while (pdelta < -Math.PI) pdelta += 2*Math.PI;
    slot.pose.yaw += pdelta * Math.min(1, dt * 6);
  }
}

// ---------- frame loop ----------
const clock = new THREE.Clock();
let frameCount = 0;
let spawnCooldown = 0;

function tick(){
  const dt = Math.min(0.05, clock.getDelta());
  const t = clock.elapsedTime;

  spawnCooldown -= dt;
  let activeCount = 0;
  for (const s of slots) if (s.active) activeCount++;
  if (spawnCooldown <= 0 && activeCount < 5){
    const free = slots.find(s=>!s.active);
    if (free){ spawn(free); spawnCooldown = 0.6 + Math.random()*1.4; }
  }

  let writeIdx = 0;

  for (const slot of slots){
    if (!slot.active) continue;
    slot.life += dt;

    // update naturalistic motion (sets vel.x, vel.z, pose.yaw)
    behave(slot, t, dt);

    // integrate position from the (now-natural) velocity
    slot.pose.x += slot.vel.x * dt;
    slot.pose.z += slot.vel.z * dt;

    despawnIfOutOfRange(slot);
    if (!slot.active) continue;

    slot.conf = Math.min(0.92, slot.life * 1.6) + Math.sin(t*4 + slot.pose.z)*0.02;

    const t_ = TYPES[slot.type];
    const c = new THREE.Color(t_.color);
    const cosY = Math.cos(slot.pose.yaw), sinY = Math.sin(slot.pose.yaw);

    const local = slot.local;
    const n = slot.bufCount;
    for (let i=0;i<n;i++){
      const lx = local[i*3+0], ly = local[i*3+1], lz = local[i*3+2];
      const wx = slot.pose.x + (lx*cosY + lz*sinY);
      const wy = slot.pose.y + ly;
      const wz = slot.pose.z + (-lx*sinY + lz*cosY);
      const o = (writeIdx + i) * 3;
      mPos[o+0] = wx; mPos[o+1] = wy; mPos[o+2] = wz;
      const sh = 0.8 + (i*0.013 % 0.5);
      mCol[o+0] = c.r * sh;
      mCol[o+1] = c.g * sh;
      mCol[o+2] = c.b * sh;
      mSize[writeIdx + i] = 1.35;
      mIntensity[writeIdx + i] = (i * 0.137) % 1.0;
    }
    writeIdx += n;

    if (slot.box){
      slot.box.position.set(slot.pose.x, slot.pose.y, slot.pose.z);
      slot.box.rotation.y = slot.pose.yaw;
      const opacity = Math.min(1, slot.life * 1.4);
      slot.box.userData.bracketMat.opacity = 0.95 * opacity;
      slot.box.userData.wireMat.opacity    = 0.18 * opacity;
    }
    if (slot.label){
      slot.label.position.set(slot.pose.x, slot.pose.y + t_.sy/2 + 0.5, slot.pose.z);
      slot.label.material.opacity = Math.min(1, slot.life * 1.2);
      if (frameCount % 8 === 0){
        const speedMs = Math.hypot(slot.vel.x, slot.vel.z);
        const speedMph = speedMs * 2.23694;
        drawLabel(slot.label, slot.id, t_.class, speedMph, slot.conf);
      }
    }

    if (slot.trail && frameCount % 3 === 0){
      const tr = slot.trail;
      const k = slot.trailIndex % TRAIL_LEN;
      tr.positions[k*3+0] = slot.pose.x;
      tr.positions[k*3+1] = 0.05;
      tr.positions[k*3+2] = slot.pose.z;
      slot.trailIndex++;

      const ordered = new Float32Array(TRAIL_LEN * 3);
      const ocol    = new Float32Array(TRAIL_LEN * 3);
      const total = Math.min(slot.trailIndex, TRAIL_LEN);
      for (let i=0;i<total;i++){
        const idx = (slot.trailIndex - total + i + TRAIL_LEN) % TRAIL_LEN;
        ordered[i*3+0] = tr.positions[idx*3+0];
        ordered[i*3+1] = tr.positions[idx*3+1];
        ordered[i*3+2] = tr.positions[idx*3+2];
        const f = total > 1 ? i / (total - 1) : 1;
        ocol[i*3+0] = tr.baseColor.r * f;
        ocol[i*3+1] = tr.baseColor.g * f;
        ocol[i*3+2] = tr.baseColor.b * f;
      }
      tr.geom.setAttribute('position', new THREE.BufferAttribute(ordered, 3));
      tr.geom.setAttribute('color',    new THREE.BufferAttribute(ocol, 3));
      tr.geom.setDrawRange(0, total);
    }
  }

  movingGeom.attributes.position.needsUpdate  = true;
  movingGeom.attributes.color.needsUpdate     = true;
  movingGeom.attributes.size.needsUpdate      = true;
  movingGeom.attributes.intensity.needsUpdate = true;
  movingGeom.setDrawRange(0, writeIdx);

  sharedUniforms.uTime.value  = t;
  sharedUniforms.uSweep.value = (t * 0.085) % 1.0;

  // gentle camera breathing
  camera.position.x = CAM_BASE.x + Math.sin(t*0.11) * 0.06;
  camera.position.y = CAM_BASE.y + Math.sin(t*0.17) * 0.03;
  camera.lookAt(0, 1.2, -22);

  renderer.render(scene, camera);
  frameCount++;
  requestAnimationFrame(tick);
}

let running = true;
document.addEventListener('visibilitychange', ()=>{
  if (document.hidden){ running=false; }
  else if (!running){ running=true; clock.start(); requestAnimationFrame(tick); }
});

requestAnimationFrame(tick);
