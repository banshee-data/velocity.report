import * as THREE from "../vendor/three/three.core.js";
import { carveTreeCuts, treeCuts } from "./sf-tree-clearance.js";
import { treeModel } from "./sf-mesh-steps.js";
import {
  treeRadius,
  treeFallAngle,
  treeFallProgress,
} from "./sf-tree-damage.js";

// Keep intact trees instanced. Only trees which could intersect pavement
// enter the clip queue. Small batches bound both transfers and GPU uploads.
// Bucket by baked row so a road change replaces one bounded GPU buffer.
const batchSize = 256;
export function addTrees(grp, tile, gz, C, reveal, deckIndex, queue) {
  if (!tile.tr.length) return;
  const [ox, oy] = tile.o;
  const instances = [C.tree, C.trunk].map((color, k) => {
    const mesh = new THREE.InstancedMesh(
      treeModel(k),
      new THREE.MeshLambertMaterial({ color }),
      tile.tr.length,
    );
    reveal.enableOn(mesh.material);
    return mesh;
  });
  const [crown, trunk] = instances;
  const mtx = new THREE.Matrix4(),
    qn = new THREE.Quaternion(),
    prior = new THREE.Quaternion();
  const s = new THREE.Vector3(),
    p = new THREE.Vector3(),
    axis = new THREE.Vector3(),
    base = new THREE.Vector3();
  const hiddenMatrix = new THREE.Matrix4().makeScale(0, 0, 0);
  const poses = new Map(),
    groups = new Map(),
    dirty = new Set(),
    moving = new Map();
  const bounds = new THREE.Box3();

  function discard(mesh) {
    if (!mesh) return;
    grp.remove(mesh);
    mesh.geometry.dispose();
  }

  function groupFor(k, i) {
    const key = Math.floor(i / batchSize) * 2 + k;
    let group = groups.get(key);
    if (group) return group;
    group = {
      rows: new Map(),
      mesh: null,
      prepare() {
        const cutIds = new Map(),
          planes = [],
          rows = [];
        for (const [index, row] of group.rows) {
          const ids = [];
          for (const cut of row.cuts) {
            if (!cutIds.has(cut)) {
              cutIds.set(cut, planes.length);
              planes.push(
                cut.planes.map(({ normal: { x, y, z }, constant }) => [
                  x,
                  y,
                  z,
                  constant,
                ]),
              );
            }
            ids.push(cutIds.get(cut));
          }
          rows.push({ index, ids, transform: row.matrix.elements });
        }
        return { trunk: k === 1, rows, planes };
      },
      fail(error) {
        console.warn("Tree mesh batch failed", error);
      },
      apply({ position, normal, uncut }) {
        const instance = instances[k];
        for (const i of uncut)
          instance.setMatrixAt(i, group.rows.get(i).matrix);
        if (uncut.length) instance.instanceMatrix.needsUpdate = true;
        if (!position.length) return;
        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute(
          "position",
          new THREE.BufferAttribute(position, 3),
        );
        geometry.setAttribute("normal", new THREE.BufferAttribute(normal, 3));
        group.mesh = new THREE.Mesh(geometry, instance.material);
        grp.add(group.mesh);
      },
    };
    groups.set(key, group);
    return group;
  }
  function flushGroups() {
    for (const group of dirty) {
      discard(group.mesh);
      group.mesh = null;
      if (group.rows.size) queue.update(group);
      else queue.cancel(group);
    }
    dirty.clear();
  }
  function setPart(k, i, event) {
    const instance = instances[k],
      group = groupFor(k, i);
    const cuts = treeCuts(instance.geometry, mtx, base.y, deckIndex);
    const previous = group.rows.get(i);
    if (event || !cuts.size) {
      if (group.rows.delete(i)) dirty.add(group);
    } else {
      // Several arriving roads may invalidate the same tile bounds. A part
      // with the same actual road triangles already has the correct cut.
      if (
        previous &&
        previous.cuts.size === cuts.size &&
        [...cuts].every((cut) => previous.cuts.has(cut))
      )
        return;
      group.rows.set(i, { matrix: mtx.clone(), cuts });
      dirty.add(group);
    }
    const key = i * 2 + k;
    discard(moving.get(key));
    moving.delete(key);
    if (event && cuts.size) {
      // Only the one animated tree needs a new mesh for this replay pose.
      const geometry = carveTreeCuts(instance.geometry, mtx, cuts);
      if (geometry?.attributes.position.count) {
        const part = new THREE.Mesh(geometry, instance.material);
        moving.set(key, part);
        grp.add(part);
      }
      instance.setMatrixAt(i, geometry ? hiddenMatrix : mtx);
    } else instance.setMatrixAt(i, cuts.size ? hiddenMatrix : mtx);
  }
  function setTreePose(i, event, elapsed = 1) {
    const [x, y, h, r] = tile.tr[i];
    base.set(ox + x / 10, gz(x / 10, y / 10), -oy - y / 10);
    qn.identity();
    if (event) {
      // Rotate the trunk and canopy about the same ground anchor, away from impact.
      axis.set(-event.north, 0, -event.east).normalize();
      if (event.priorLean) {
        qn.setFromAxisAngle(axis, treeFallAngle(event, 1));
        const [east, north] = event.priorLean;
        axis.set(-north, 0, -east).normalize();
        prior.setFromAxisAngle(axis, treeFallAngle({ stage: "cracked" }, 1));
        qn.copy(prior.slerp(qn, treeFallProgress(elapsed)));
      } else {
        qn.setFromAxisAngle(axis, treeFallAngle(event, elapsed));
      }
    }
    const ch = Math.max(1.5, Math.min(h * 0.6, 2.2 * r));
    p.set(0, h - ch / 2, 0)
      .applyQuaternion(qn)
      .add(base);
    s.set(r, ch / 2, r);
    mtx.compose(p, qn, s);
    setPart(0, i, event);
    const tw = treeRadius(h);
    p.set(0, (h - ch) / 2, 0)
      .applyQuaternion(qn)
      .add(base);
    s.set(tw, Math.max(0.5, h - ch), tw);
    mtx.compose(p, qn, s);
    setPart(1, i, event);
  }

  tile.tr.forEach(([, , h, r], i) => {
    setTreePose(i);
    const reach = h + r + 1;
    bounds.expandByPoint(p.copy(base).addScalar(reach));
    bounds.expandByPoint(p.copy(base).addScalar(-reach));
  });
  // Include future poses even when a batch is empty for its first render.
  for (const model of instances)
    model.boundingSphere = bounds.getBoundingSphere(new THREE.Sphere());
  flushGroups();
  grp.userData.drawTree = (i, event, seconds) => {
    if (!tile.tr[i]) return;
    if (event) poses.set(i, [event, seconds]);
    else poses.delete(i);
    setTreePose(i, event, seconds);
    flushGroups();
    crown.instanceMatrix.needsUpdate = trunk.instanceMatrix.needsUpdate = true;
  };
  grp.userData.refreshTreeCuts = (bounds) => {
    tile.tr.forEach(([x, y, h, r], i) => {
      const east = ox + x / 10,
        south = -oy - y / 10;
      const extent = poses.get(i)?.[0] ? h + r : r;
      if (
        !bounds.some(
          (b) =>
            east + extent >= b.min.x &&
            east - extent <= b.max.x &&
            south + extent >= b.min.z &&
            south - extent <= b.max.z,
        )
      )
        return;
      setTreePose(i, ...(poses.get(i) || []));
    });
    flushGroups();
    crown.instanceMatrix.needsUpdate = trunk.instanceMatrix.needsUpdate = true;
  };

  grp.userData.cancelTreeCuts = () => {
    for (const group of groups.values()) queue.cancel(group);
  };
  grp.add(crown, trunk);
}
