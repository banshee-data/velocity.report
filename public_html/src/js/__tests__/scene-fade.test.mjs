import { test } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";
import {
  sceneFade,
  sceneRangeOpacity,
  FADE_NEAR_M,
  FADE_FAR_M,
} from "../scene-fade.js";
test("range fade applies to meshes, lines and points in sensor-centred world coordinates", () => {
  assert.equal(FADE_NEAR_M, 150);
  assert.equal(FADE_FAR_M, 170);
  for (const name of ["basic", "points", "dashed"]) {
    const shader = {
      vertexShader: THREE.ShaderLib[name].vertexShader,
      fragmentShader: THREE.ShaderLib[name].fragmentShader,
    };
    const material = sceneFade(new THREE.MeshBasicMaterial());
    material.onBeforeCompile(shader);
    assert.match(
      shader.vertexShader,
      /modelMatrix \* vec4\(transformed, 1.0\)/,
    );
    assert.match(
      shader.fragmentShader,
      /smoothstep\(150.0, 170.0, length\(vSceneRangePosition\)\)/,
    );
    assert.equal(material.transparent, true);
  }
});

test("DOM labels follow the same smooth range fade as their boxes", () => {
  assert.equal(sceneRangeOpacity(140), 1);
  assert.equal(sceneRangeOpacity(150), 1);
  assert.equal(sceneRangeOpacity(160), 0.5);
  assert.equal(sceneRangeOpacity(170), 0);
  assert.equal(sceneRangeOpacity(180), 0);
});
