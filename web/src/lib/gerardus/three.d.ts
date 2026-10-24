/**
 * Types for the parts of three.js the site viewer's 3D view uses.
 *
 * three ships no declarations and the web build has no @types/three, so, as
 * for the shared scene player (src/lib/scene/scene-reader.d.ts), only the
 * surface this code calls is declared. Extend it here when the view needs
 * more; the real modules are resolved by Vite at build time.
 */
declare module 'three' {
	export const DoubleSide: number;
	export const FrontSide: number;
	export const SRGBColorSpace: string;

	export class Vector3 {
		constructor(x?: number, y?: number, z?: number);
		x: number;
		y: number;
		z: number;
		set(x: number, y: number, z: number): this;
		copy(v: Vector3): this;
		clone(): Vector3;
		project(camera: Camera): this;
	}

	export class Color {
		constructor(color?: string | number);
	}

	export class EventDispatcher {
		addEventListener(type: string, listener: (event: unknown) => void): void;
		removeEventListener(type: string, listener: (event: unknown) => void): void;
	}

	export class Object3D extends EventDispatcher {
		name: string;
		visible: boolean;
		renderOrder: number;
		position: Vector3;
		up: Vector3;
		children: Object3D[];
		parent: Object3D | null;
		userData: Record<string, unknown>;
		add(...objects: Object3D[]): this;
		remove(...objects: Object3D[]): this;
		clear(): this;
		lookAt(x: number | Vector3, y?: number, z?: number): void;
		traverse(callback: (object: Object3D) => void): void;
	}

	export class Scene extends Object3D {
		background: Color | null;
	}

	export class Group extends Object3D {}

	export class Camera extends Object3D {
		updateMatrixWorld(force?: boolean): void;
	}

	export class PerspectiveCamera extends Camera {
		constructor(fov?: number, aspect?: number, near?: number, far?: number);
		aspect: number;
		near: number;
		far: number;
		updateProjectionMatrix(): void;
	}

	export class BufferAttribute {
		constructor(array: ArrayLike<number>, itemSize: number, normalized?: boolean);
		count: number;
	}

	export class BufferGeometry extends EventDispatcher {
		setAttribute(name: string, attribute: BufferAttribute): this;
		getAttribute(name: string): BufferAttribute | undefined;
		setIndex(index: BufferAttribute | number[] | null): this;
		computeVertexNormals(): void;
		computeBoundingSphere(): void;
		dispose(): void;
	}

	export interface MaterialParameters {
		color?: string | number | Color;
		transparent?: boolean;
		opacity?: number;
		side?: number;
		depthWrite?: boolean;
		vertexColors?: boolean;
		polygonOffset?: boolean;
		polygonOffsetFactor?: number;
		polygonOffsetUnits?: number;
	}

	export class Material extends EventDispatcher {
		transparent: boolean;
		opacity: number;
		dispose(): void;
	}

	export class MeshLambertMaterial extends Material {
		constructor(parameters?: MaterialParameters);
	}

	export class MeshBasicMaterial extends Material {
		constructor(parameters?: MaterialParameters);
	}

	export class LineBasicMaterial extends Material {
		constructor(parameters?: MaterialParameters & { linewidth?: number });
	}

	export class PointsMaterial extends Material {
		constructor(parameters?: MaterialParameters & { size?: number; sizeAttenuation?: boolean });
	}

	export class Mesh extends Object3D {
		constructor(geometry?: BufferGeometry, material?: Material);
		geometry: BufferGeometry;
		material: Material;
	}

	export class LineSegments extends Object3D {
		constructor(geometry?: BufferGeometry, material?: Material);
		geometry: BufferGeometry;
		material: Material;
	}

	export class Points extends Object3D {
		constructor(geometry?: BufferGeometry, material?: Material);
		geometry: BufferGeometry;
		material: Material;
	}

	export class Light extends Object3D {
		intensity: number;
	}

	export class AmbientLight extends Light {
		constructor(color?: string | number, intensity?: number);
	}

	export class DirectionalLight extends Light {
		constructor(color?: string | number, intensity?: number);
	}

	export class WebGLRenderer {
		constructor(parameters?: { canvas?: HTMLCanvasElement; antialias?: boolean; alpha?: boolean });
		domElement: HTMLCanvasElement;
		outputColorSpace: string;
		setPixelRatio(ratio: number): void;
		setSize(width: number, height: number, updateStyle?: boolean): void;
		setClearColor(color: string | number, alpha?: number): void;
		render(scene: Object3D, camera: Camera): void;
		dispose(): void;
	}
}

declare module 'three/examples/jsm/controls/OrbitControls.js' {
	import type { Camera, EventDispatcher, Vector3 } from 'three';

	export class OrbitControls extends EventDispatcher {
		constructor(camera: Camera, domElement?: HTMLElement);
		target: Vector3;
		enableDamping: boolean;
		screenSpacePanning: boolean;
		maxPolarAngle: number;
		minDistance: number;
		maxDistance: number;
		update(): boolean;
		dispose(): void;
	}
}

declare module 'three/src/extras/lib/earcut.js' {
	/**
	 * mapbox/earcut, as three.js vendors it: triangles of a flat polygon given
	 * as `dim` numbers per vertex, holes starting at `holeIndices`, as three
	 * vertex indices each.
	 */
	export default function earcut(
		data: ArrayLike<number>,
		holeIndices?: ArrayLike<number>,
		dim?: number
	): number[];
}
