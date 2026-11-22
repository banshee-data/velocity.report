// Verify the versioned beacon shader against the live preview's local depth.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");

(async () => {
  const host = process.env.SF_BASE_URL || "http://localhost:8090";
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    const page = await browser.newPage();
    const response = await page.request.get(`${host}/sf/`);
    assert.ok(response.ok());
    const modules = (await response.text()).match(
      /import\("([^"]+\/)sf-drive\.js"\)/,
    )?.[1];
    assert.ok(modules);
    const fixture = `${host}/sf/beacon-depth-check`;
    await page.route(fixture, (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<title>Survey beacon depth check</title>",
      }),
    );
    await page.goto(fixture);
    const pixels = await page.evaluate(async (modules) => {
      const { surveyBeacons } = await import(`${modules}sf-survey-beacons.js`);
      const THREE = await import(`${modules}../vendor/three/three.module.js`);
      const renderer = new THREE.WebGLRenderer({ antialias: false });
      renderer.setSize(64, 64);
      renderer.setClearColor(0);
      const local = new THREE.PerspectiveCamera(62, 1, 0.4, 360);
      const distant = new THREE.PerspectiveCamera(62, 1, 0.4, 6500);
      const scene = new THREE.Scene();
      const wall = new THREE.Mesh(
        new THREE.PlaneGeometry(20, 20),
        new THREE.MeshBasicMaterial({ color: 0xff0000 }),
      );
      scene.add(wall);
      const columns = new THREE.Scene();
      function draw(distance, wallDistance) {
        wall.visible = wallDistance !== null;
        wall.position.z = -wallDistance;
        const site = {
          x: 0,
          y: distance - 58,
          transform: [[], [], [0, 0, 1, -360]],
        };
        const mesh = surveyBeacons([site], local);
        columns.add(mesh);
        renderer.render(scene, local);
        renderer.autoClear = false;
        renderer.render(columns, distant);
        renderer.autoClear = true;
        const gl = renderer.getContext();
        const pixel = new Uint8Array(4);
        gl.readPixels(32, 32, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, pixel);
        columns.remove(mesh);
        mesh.geometry.dispose();
        mesh.material.dispose();
        return Array.from(pixel);
      }
      return {
        occluded: draw(120, 100),
        inFront: draw(120, 180),
        distantSky: draw(1000, null),
        distantOccluded: draw(1000, 100),
      };
    }, modules);
    assert.deepEqual(pixels.occluded, [255, 0, 0, 255]);
    assert.deepEqual(pixels.distantOccluded, [255, 0, 0, 255]);
    assert.ok(pixels.inFront[2] > 0, JSON.stringify(pixels));
    assert.ok(pixels.distantSky[2] > 0, JSON.stringify(pixels));
    console.log(
      "Local buildings occlude the beacons, including distant columns. Unobstructed columns remain visible.",
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
