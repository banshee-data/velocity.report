<script lang="ts">
	/**
	 * SiteScene3D — the site viewer's 3D view of what Gerardus serves for a
	 * site, in its site frame: x east, y north, z up, metres from its origin.
	 *
	 * Everything drawn is built as plain arrays by lib/gerardus/scene3d (the
	 * ground mesh, raised boxes, wall planes, draped surfaces, lines at their
	 * heights, lane arrows); this component only turns them into three.js
	 * objects, orbits a camera about them and draws on demand. A new site's
	 * objects replace the old ones, whose geometries and materials are
	 * disposed first.
	 */
	import type { GroundGrid, ScanCloud, SiteFrameFeature } from '#lib/gerardus/client.js';
	import {
		boundsOfArrays,
		buildLayerScenes,
		groundMesh,
		heightColours,
		homeView,
		screenNorth,
		type MeshData
	} from '#lib/gerardus/scene3d.js';
	import * as THREE from 'three';
	import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
	import { onDestroy, onMount } from 'svelte';

	/** Site-frame features of every layer to draw. */
	export let features: SiteFrameFeature[] = [];
	export let ground: GroundGrid | null = null;
	export let scans: ScanCloud | null = null;
	/** Which layers are shown, by layer name, plus `ground` and `scans`. Absent means shown. */
	export let visible: Record<string, boolean> = {};
	/** Changes when another site is loaded: the camera goes back to its home view. */
	export let siteKey = '';

	let container: HTMLDivElement;
	let canvas: HTMLCanvasElement;
	let failure: string | null = null;
	let northDeg = 0;

	let renderer: THREE.WebGLRenderer | null = null;
	let scene: THREE.Scene | null = null;
	let camera: THREE.PerspectiveCamera | null = null;
	let controls: OrbitControls | null = null;
	let resizeObserver: ResizeObserver | null = null;
	let frame = 0;

	// The site's objects, by layer, and the scan points apart, since they load on their own.
	let content: THREE.Group | null = null;
	let scanGroup: THREE.Group | null = null;
	// Not reactive state: three.js objects, read only by the code that draws them.
	let layerGroups = new Map<string, THREE.Group>(); // eslint-disable-line svelte/prefer-svelte-reactivity
	// The home view of the site last framed, so Reset goes back to it.
	let home = homeView(null);
	let framedKey: string | null = null;

	onMount(() => {
		try {
			renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
		} catch (e) {
			failure = `This browser could not start WebGL for the 3D view${e instanceof Error ? `: ${e.message}` : ''}.`;
			return;
		}
		renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
		renderer.setClearColor('#0b0f14');
		scene = new THREE.Scene();
		camera = new THREE.PerspectiveCamera(50, 1, 0.5, 6000);
		// The site frame is z up; so is the camera.
		camera.up.set(0, 0, 1);
		scene.add(new THREE.AmbientLight('#ffffff', 0.65));
		const sun = new THREE.DirectionalLight('#ffffff', 0.9);
		sun.position.set(-0.4, -0.6, 1);
		scene.add(sun);
		controls = new OrbitControls(camera, canvas);
		controls.screenSpacePanning = true;
		controls.maxPolarAngle = Math.PI * 0.495;
		controls.minDistance = 2;
		controls.maxDistance = 3000;
		controls.addEventListener('change', requestRender);
		resizeObserver = new ResizeObserver(resize);
		resizeObserver.observe(container);
		resize();
		rebuild(features, ground);
		rebuildScans(scans);
		applyVisibility(visible);
	});

	onDestroy(() => {
		cancelAnimationFrame(frame);
		resizeObserver?.disconnect();
		controls?.removeEventListener('change', requestRender);
		controls?.dispose();
		if (content) disposeTree(content);
		if (scanGroup) disposeTree(scanGroup);
		renderer?.dispose();
		renderer = null;
	});

	$: if (scene) rebuild(features, ground);
	$: if (scene) rebuildScans(scans);
	$: if (scene) applyVisibility(visible);

	function resize() {
		if (!renderer || !camera || !container) return;
		const w = container.clientWidth;
		const h = container.clientHeight;
		// A hidden tab measures nothing; keep the last size until it is shown.
		if (w === 0 || h === 0) return;
		renderer.setSize(w, h, false);
		camera.aspect = w / h;
		camera.updateProjectionMatrix();
		requestRender();
	}

	function requestRender() {
		if (frame) return;
		frame = requestAnimationFrame(() => {
			frame = 0;
			render();
		});
	}

	function render() {
		if (!renderer || !scene || !camera || !controls) return;
		renderer.render(scene, camera);
		// The north arrow follows where a point a metre north of the target lands on screen.
		const t = controls.target.clone().project(camera);
		const n = new THREE.Vector3(
			controls.target.x,
			controls.target.y + 1,
			controls.target.z
		).project(camera);
		// Normalised device coordinates to pixels, y down, so the aspect does not skew the angle.
		const w = container.clientWidth / 2;
		const h = container.clientHeight / 2;
		northDeg = screenNorth([t.x * w, -t.y * h], [n.x * w, -n.y * h]);
	}

	// ---- Building ------------------------------------------------------------------

	function disposeTree(root: THREE.Object3D) {
		root.traverse((o) => {
			const drawn = o as Partial<THREE.Mesh>;
			drawn.geometry?.dispose();
			drawn.material?.dispose();
		});
		root.parent?.remove(root);
	}

	function geometryOf(positions: Float32Array, indices?: Uint32Array): THREE.BufferGeometry {
		const g = new THREE.BufferGeometry();
		g.setAttribute('position', new THREE.BufferAttribute(positions, 3));
		if (indices) g.setIndex(new THREE.BufferAttribute(indices, 1));
		g.computeBoundingSphere();
		return g;
	}

	function meshOf(m: MeshData, material: THREE.Material): THREE.Mesh {
		const g = geometryOf(m.positions, m.indices);
		g.computeVertexNormals();
		return new THREE.Mesh(g, material);
	}

	function rebuild(fs: SiteFrameFeature[], grid: GroundGrid | null) {
		if (!scene) return;
		if (content) disposeTree(content);
		content = new THREE.Group();
		layerGroups = new Map();
		const extents: Float32Array[] = [];

		if (grid) {
			const data = groundMesh(grid);
			if (data.indices.length > 0) {
				const group = new THREE.Group();
				group.add(
					meshOf(
						data,
						new THREE.MeshLambertMaterial({
							color: '#3b4656',
							side: THREE.DoubleSide,
							// Behind what is draped on it.
							polygonOffset: true,
							polygonOffsetFactor: 1,
							polygonOffsetUnits: 1
						})
					)
				);
				layerGroups.set('ground', group);
				content.add(group);
				extents.push(data.positions);
			}
		}

		for (const layer of buildLayerScenes(fs)) {
			const group = new THREE.Group();
			group.name = layer.layer;
			for (const m of layer.meshes) {
				const mesh = meshOf(
					m.mesh,
					new THREE.MeshLambertMaterial({
						color: m.colour,
						transparent: m.opacity < 1,
						opacity: m.opacity,
						depthWrite: m.opacity >= 1,
						side: m.doubleSided ? THREE.DoubleSide : THREE.FrontSide
					})
				);
				mesh.renderOrder = 1;
				group.add(mesh);
				extents.push(m.mesh.positions);
			}
			for (const l of layer.lines) {
				const line = new THREE.LineSegments(
					geometryOf(l.positions),
					new THREE.LineBasicMaterial({ color: l.colour })
				);
				line.renderOrder = 2;
				group.add(line);
				extents.push(l.positions);
			}
			for (const p of layer.points) {
				group.add(
					new THREE.Points(
						geometryOf(p.positions),
						new THREE.PointsMaterial({ color: p.colour, size: 7, sizeAttenuation: false })
					)
				);
				extents.push(p.positions);
			}
			layerGroups.set(layer.layer, group);
			content.add(group);
		}
		scene.add(content);
		if (scanGroup) layerGroups.set('scans', scanGroup);
		applyVisibility(visible);

		home = homeView(boundsOfArrays(extents));
		// Frame a site once, when the first of its content arrives; a reload of
		// the same site (another layer allowed, say) leaves the camera where it is.
		if (extents.length === 0) {
			resetView();
		} else if (framedKey !== siteKey) {
			framedKey = siteKey;
			resetView();
		}
		requestRender();
	}

	function rebuildScans(cloud: ScanCloud | null) {
		if (!scene) return;
		if (scanGroup) disposeTree(scanGroup);
		scanGroup = null;
		layerGroups.delete('scans');
		if (cloud && cloud.count > 0) {
			const g = geometryOf(cloud.points);
			g.setAttribute('color', new THREE.BufferAttribute(heightColours(cloud.points), 3));
			scanGroup = new THREE.Group();
			scanGroup.add(
				new THREE.Points(
					g,
					new THREE.PointsMaterial({ size: 2, sizeAttenuation: false, vertexColors: true })
				)
			);
			scene.add(scanGroup);
			layerGroups.set('scans', scanGroup);
			applyVisibility(visible);
		}
		requestRender();
	}

	function applyVisibility(shown: Record<string, boolean>) {
		for (const [layer, group] of layerGroups) group.visible = shown[layer] !== false;
		requestRender();
	}

	/** Back to the site's home view: from the south, looking north and down. */
	export function resetView() {
		if (!camera || !controls) return;
		controls.target.set(...home.target);
		camera.position.set(...home.position);
		camera.lookAt(controls.target);
		controls.update();
		requestRender();
	}
</script>

<div bind:this={container} class="relative h-full w-full overflow-hidden">
	<canvas bind:this={canvas} class="block h-full w-full" aria-label="3D view of the site"></canvas>
	{#if failure}
		<div
			class="absolute inset-0 flex items-center justify-center p-6 text-center text-sm text-red-300"
		>
			{failure}
		</div>
	{:else}
		<svg
			class="pointer-events-none absolute top-3 right-3 h-14 w-14 drop-shadow"
			viewBox="-28 -28 56 56"
			role="img"
			aria-label="North arrow"
		>
			<circle r="26" fill="rgba(11,15,20,0.75)" stroke="#475569" stroke-width="1.5" />
			<g transform="rotate({northDeg})">
				<path d="M0 -13 L5 0 L-5 0 Z" fill="#ef4444" />
				<path d="M0 13 L5 0 L-5 0 Z" fill="#94a3b8" />
				<text y="-16" text-anchor="middle" font-size="9" font-weight="700" fill="#f8fafc">N</text>
			</g>
		</svg>
		<div class="absolute right-3 bottom-3">
			<button
				type="button"
				class="rounded border border-neutral-600 bg-neutral-900/80 px-2 py-1 text-xs text-neutral-100 hover:bg-neutral-800"
				on:click={resetView}>Reset view</button
			>
		</div>
		<div
			class="pointer-events-none absolute bottom-3 left-3 max-w-[calc(100%-7rem)] rounded bg-neutral-900/70 px-2 py-1 text-[11px] text-neutral-300"
		>
			Drag to orbit · right-drag or Shift-drag to pan · scroll to zoom
		</div>
	{/if}
</div>
