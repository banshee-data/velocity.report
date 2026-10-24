import type { SiteFrameFeature } from './client';
import {
	boundsOfArrays,
	buildLayerScenes,
	extrudeBox,
	groundMesh,
	heightColours,
	homeView,
	laneArrowSegments,
	lineSegments,
	mergeMeshes,
	planeMesh,
	ringNormal,
	screenNorth,
	surfaceMesh,
	triangulate,
	verticalSegments,
	type MeshData,
	type Vec3
} from './scene3d';

/** Vertex i of a mesh, as [x, y, z]. */
const vertex = (m: { positions: Float32Array }, i: number): Vec3 => [
	m.positions[i * 3],
	m.positions[i * 3 + 1],
	m.positions[i * 3 + 2]
];

/** Each triangle's normal, unnormalised. */
function normals(m: MeshData): Vec3[] {
	const out: Vec3[] = [];
	for (let t = 0; t < m.indices.length; t += 3) {
		const [a, b, c] = [0, 1, 2].map((k) => vertex(m, m.indices[t + k]));
		const u = [b[0] - a[0], b[1] - a[1], b[2] - a[2]];
		const v = [c[0] - a[0], c[1] - a[1], c[2] - a[2]];
		out.push([u[1] * v[2] - u[2] * v[1], u[2] * v[0] - u[0] * v[2], u[0] * v[1] - u[1] * v[0]]);
	}
	return out;
}

/** The area of a mesh's triangles. */
function area(m: MeshData): number {
	return normals(m).reduce((s, n) => s + Math.hypot(...n) / 2, 0);
}

const square = (z = 0, size = 10): Vec3[] => [
	[0, 0, z],
	[size, 0, z],
	[size, size, z],
	[0, size, z]
];

describe('the ground mesh', () => {
	it('puts a vertex at the centre of every cell with a height', () => {
		const m = groundMesh({
			x0: 100,
			y0: 200,
			cellM: 2,
			nx: 2,
			ny: 2,
			heights: [1, 2, 3, 4]
		});
		expect(Array.from(m.positions)).toEqual([101, 201, 1, 103, 201, 2, 101, 203, 3, 103, 203, 4]);
		// Two triangles for the one square, counter-clockwise from above.
		expect(Array.from(m.indices)).toEqual([0, 1, 3, 0, 3, 2]);
		for (const n of normals(m)) expect(n[2]).toBeGreaterThan(0);
	});

	it('leaves empty cells out and does not bridge them', () => {
		// Row-major, y outer: the middle of the bottom row is empty.
		const m = groundMesh({
			x0: 0,
			y0: 0,
			cellM: 1,
			nx: 3,
			ny: 2,
			heights: [0, null, 0, 0, 0, 0]
		});
		expect(m.positions.length / 3).toBe(5);
		// Each square misses one corner: one triangle each, of the three it has.
		expect(m.indices.length).toBe(6);
		expect(Array.from(m.indices.slice(0, 3))).toEqual([0, 3, 2]);
		for (const n of normals(m)) expect(n[2]).toBeGreaterThan(0);
	});

	it('has nothing to draw with fewer than three neighbours', () => {
		const sparse = groundMesh({
			x0: 0,
			y0: 0,
			cellM: 1,
			nx: 2,
			ny: 2,
			heights: [5, null, null, 5]
		});
		expect(sparse.positions.length).toBe(6);
		expect(sparse.indices.length).toBe(0);
		expect(groundMesh({ x0: 0, y0: 0, cellM: 1, nx: 0, ny: 0, heights: [] }).indices.length).toBe(
			0
		);
	});
});

describe('triangulation', () => {
	it('finds the normal of a ring by its winding', () => {
		expect(ringNormal(square())).toEqual([0, 0, 200]);
		expect(ringNormal([...square()].reverse())[2]).toBe(-200);
	});

	it('cuts a flat polygon facing up, however it is wound', () => {
		for (const ring of [square(), [...square()].reverse()]) {
			const tris = triangulate(ring, [], [0, 0, 1]);
			expect(tris).toHaveLength(6);
			const m = { positions: new Float32Array(ring.flat()), indices: new Uint32Array(tris) };
			for (const n of normals(m)) expect(n[2]).toBeGreaterThan(0);
			expect(area(m)).toBeCloseTo(100);
		}
	});

	it('leaves holes open', () => {
		const hole: Vec3[] = [
			[4, 4, 0],
			[6, 4, 0],
			[6, 6, 0],
			[4, 6, 0]
		];
		const tris = triangulate(square(), [hole], [0, 0, 1]);
		const m = {
			positions: new Float32Array([...square(), ...hole].flat()),
			indices: new Uint32Array(tris)
		};
		expect(area(m)).toBeCloseTo(96);
	});

	it('cuts an upright polygon in its own plane', () => {
		const wall: Vec3[] = [
			[0, 5, 0],
			[8, 5, 0],
			[8, 5, 3],
			[0, 5, 3]
		];
		const m = planeMesh(wall);
		expect(m.indices.length).toBe(6);
		expect(area(m)).toBeCloseTo(24);
		// Facing its own normal, which for this winding is −y.
		for (const n of normals(m)) expect(n[1]).toBeLessThan(0);
		const across: Vec3[] = [
			[2, 0, 0],
			[2, 4, 0],
			[2, 4, 3],
			[2, 0, 3]
		];
		expect(area(planeMesh(across))).toBeCloseTo(12);
	});

	it('has nothing for a ring without area', () => {
		const line: Vec3[] = [
			[0, 0, 0],
			[1, 1, 1],
			[2, 2, 2]
		];
		expect(triangulate(line)).toEqual([]);
		expect(triangulate(square().slice(0, 2))).toEqual([]);
		expect(planeMesh(line).indices.length).toBe(0);
		expect(surfaceMesh(line).indices.length).toBe(0);
	});
});

describe('surfaces', () => {
	it('drapes on the vertices’ own heights, lifted, facing up', () => {
		const slope: Vec3[] = [
			[0, 0, 1],
			[10, 0, 2],
			[10, 4, 2.5],
			[0, 4, 1.5]
		];
		const m = surfaceMesh(slope, [], 0.02);
		expect(m.positions.length).toBe(12);
		expect(vertex(m, 0)[2]).toBeCloseTo(1.02);
		expect(vertex(m, 2)[2]).toBeCloseTo(2.52);
		expect(m.indices.length).toBe(6);
		for (const n of normals(m)) expect(n[2]).toBeGreaterThan(0);
	});
});

describe('box extrusion', () => {
	it('raises the footprint from its base z by its height', () => {
		const m = extrudeBox(square(2), 3);
		// Four walls of four vertices each, then the top's four.
		expect(m.positions.length / 3).toBe(20);
		expect(m.indices.length).toBe(4 * 6 + 6);
		const zs = new Set<number>();
		for (let i = 0; i < 20; i++) zs.add(vertex(m, i)[2]);
		expect([...zs].sort()).toEqual([2, 5]);
		expect(area(m)).toBeCloseTo(4 * 10 * 3 + 100);
	});

	it('faces every wall out and the top up, whichever way the footprint runs', () => {
		for (const ring of [square(0), [...square(0)].reverse()]) {
			const m = extrudeBox(ring, 3);
			const ns = normals(m);
			const centre = [5, 5];
			for (let t = 0; t < 8; t++) {
				// A wall triangle's normal points away from the middle of the box.
				const a = vertex(m, m.indices[t * 3]);
				const b = vertex(m, m.indices[t * 3 + 1]);
				const mid = [(a[0] + b[0]) / 2 - centre[0], (a[1] + b[1]) / 2 - centre[1]];
				expect(ns[t][0] * mid[0] + ns[t][1] * mid[1]).toBeGreaterThan(0);
				expect(ns[t][2]).toBeCloseTo(0);
			}
			for (const n of ns.slice(8)) expect(n[2]).toBeGreaterThan(0);
		}
	});

	it('walls a hole facing into it', () => {
		const hole: Vec3[] = [
			[4, 4, 0],
			[4, 6, 0],
			[6, 6, 0],
			[6, 4, 0]
		];
		const m = extrudeBox(square(0), 2, [hole]);
		const ns = normals(m);
		// The hole's walls come after the outer ring's four.
		for (let w = 4; w < 8; w++) {
			const a = vertex(m, m.indices[w * 6]);
			const b = vertex(m, m.indices[w * 6 + 1]);
			const mid = [(a[0] + b[0]) / 2 - 5, (a[1] + b[1]) / 2 - 5];
			const n = ns[w * 2];
			expect(n[0] * mid[0] + n[1] * mid[1]).toBeLessThan(0);
		}
		// Top: 100 less the hole's 4.
		const top = { positions: m.positions, indices: m.indices.slice(8 * 6) };
		expect(area(top)).toBeCloseTo(96);
	});

	it('raises nothing without a height or an area', () => {
		expect(extrudeBox(square(), 0).indices.length).toBe(0);
		expect(extrudeBox(square(), NaN).indices.length).toBe(0);
		expect(extrudeBox(square().slice(0, 2), 2).indices.length).toBe(0);
		expect(
			extrudeBox(
				[
					[0, 0, 0],
					[1, 0, 0],
					[2, 0, 0]
				],
				2
			).indices.length
		).toBe(0);
	});
});

describe('merging', () => {
	it('offsets each mesh’s indices past the vertices before it', () => {
		const a = surfaceMesh(square(0, 1));
		const b = surfaceMesh(square(5, 1));
		const m = mergeMeshes([a, { positions: new Float32Array(0), indices: new Uint32Array(0) }, b]);
		expect(m.positions.length).toBe(24);
		expect(Array.from(m.indices.slice(6))).toEqual(Array.from(b.indices).map((i) => i + 4));
		expect(mergeMeshes([a])).toBe(a);
		expect(mergeMeshes([]).indices.length).toBe(0);
	});
});

describe('lines and points', () => {
	it('makes segment pairs, closing rings, at their own heights', () => {
		const out = lineSegments(
			[
				{
					points: [
						[0, 0, 1],
						[1, 0, 2],
						[1, 1, 3]
					],
					closed: false
				},
				{ points: square(4, 1), closed: true },
				{ points: [[9, 9, 9]], closed: false }
			],
			0.5
		);
		// Two segments, then four closing the ring; a lone point has none.
		expect(out.length).toBe((2 + 4) * 6);
		expect(Array.from(out.slice(0, 6))).toEqual([0, 0, 1.5, 1, 0, 2.5]);
		expect(Array.from(out.slice(-6))).toEqual([0, 1, 4.5, 0, 0, 4.5]);
	});

	it('points lane arrows in the direction of travel, at the lane’s height', () => {
		const lane: Vec3[] = [
			[0, 0, 0],
			[0, 16, 4]
		];
		const out = laneArrowSegments(lane, 8, 1);
		// Two arrows, of two segments each.
		expect(out.length).toBe(2 * 2 * 6);
		// The first arrow's tip, two metres in, and its height there.
		expect(out[3]).toBeCloseTo(0);
		expect(out[4]).toBeCloseTo(4);
		expect(out[5]).toBeCloseTo(1);
		// Its tail is behind the tip: south of it, for a lane running north.
		expect(out[1]).toBeLessThan(out[4]);
		expect(laneArrowSegments([[0, 0, 0]])).toHaveLength(0);
	});

	it('stands a pole up from its base', () => {
		expect(Array.from(verticalSegments([[1, 2, 3]], 5))).toEqual([1, 2, 3, 1, 2, 8]);
	});

	it('colours scan points by height', () => {
		const c = heightColours(new Float32Array([0, 0, 0, 0, 0, 10, 0, 0, 5]));
		// Low is blue, high is warm.
		expect(c[2]).toBeGreaterThan(c[0]);
		expect(c[3]).toBeGreaterThan(c[5]);
		const flat = heightColours(new Float32Array([0, 0, 2, 1, 1, 2]));
		expect(Array.from(flat.slice(0, 3))).toEqual(Array.from(flat.slice(3)));
	});
});

const feature = (
	layer: string,
	kind: string,
	geometry: SiteFrameFeature['geometry'],
	tags: Record<string, string> = {}
): SiteFrameFeature => ({ layer, id: `${layer}:${kind}`, kind, tags, geometry });

describe('a site’s layers', () => {
	const box = feature(
		'boxes',
		'building',
		{ type: 'Polygon', coordinates: [[...square(0), [0, 0, 0]]] },
		{ height_m: '6' }
	);
	const flatBox = feature('boxes', 'building', { type: 'Polygon', coordinates: [square(0)] });
	const wall = feature(
		'wall_planes',
		'wall_plane',
		{
			type: 'Polygon',
			coordinates: [
				[
					[0, 0, 0],
					[5, 0, 0],
					[5, 0, 4],
					[0, 0, 4]
				]
			]
		},
		{ role: 'recess' }
	);
	const sidewalk = feature(
		'street_use',
		'surface',
		{ type: 'Polygon', coordinates: [square(0)] },
		{ surface: 'sidewalk' }
	);
	const verge = feature(
		'street_use',
		'surface',
		{ type: 'MultiPolygon', coordinates: [[square(0)], [square(1)]] },
		{ surface: 'verge' }
	);
	const space = feature(
		'street_use',
		'parking_space',
		{ type: 'Polygon', coordinates: [square(0, 2)] },
		{ space_type: 'loading' }
	);
	const lane = feature(
		'street_use',
		'travel_lane',
		{
			type: 'LineString',
			coordinates: [
				[0, 0, 0],
				[0, 20, 0]
			]
		},
		{ lane_type: 'bus' }
	);
	const kerb = feature('kerbs', 'kerb', {
		type: 'LineString',
		coordinates: [
			[0, 0, 0.15],
			[5, 0, 0.15]
		]
	});
	const pole = feature(
		'observed',
		'pole',
		{ type: 'Point', coordinates: [1, 1, 0] },
		{ height_m: '4' }
	);
	const sensor = feature('observed', 'sensor', { type: 'Point', coordinates: [2, 2, 1] });

	it('groups by layer, in the order layers first appear', () => {
		const layers = buildLayerScenes([kerb, box, sidewalk, wall, pole, lane]).map((l) => l.layer);
		expect(layers).toEqual(['kerbs', 'boxes', 'street_use', 'wall_planes', 'observed']);
	});

	it('raises boxes by their height, and counts one with none as undrawn', () => {
		const [boxes] = buildLayerScenes([box, flatBox]);
		expect(boxes.meshes).toHaveLength(1);
		expect(boxes.meshes[0]).toMatchObject({ colour: '#f59e0b', doubleSided: false });
		expect(boxes.meshes[0].mesh.positions.length / 3).toBe(20);
		// Both footprints are outlined.
		expect(boxes.lines[0].positions.length).toBe(2 * 4 * 6);
		expect(boxes.undrawn).toBe(1);
	});

	it('colours wall planes by role and surfaces by what they are', () => {
		const [walls] = buildLayerScenes([wall]);
		expect(walls.meshes[0]).toMatchObject({ colour: '#3b82f6', doubleSided: true });
		const [use] = buildLayerScenes([sidewalk, verge, space, lane]);
		const colours = use.meshes.map((m) => m.colour);
		expect(colours).toEqual(['#d6d3d1', '#22c55e', '#eab308']);
		// The verge's two polygons are one fill.
		expect(use.meshes[1].mesh.indices.length).toBe(12);
		expect(use.meshes[0].opacity).toBeLessThan(1);
		// Surfaces are draped a little above the ground; spaces a little more.
		expect(use.meshes[0].mesh.positions[2]).toBeCloseTo(0.02);
		expect(use.meshes[2].mesh.positions[2]).toBeCloseTo(0.04);
		// The space is outlined; the bus lane is a line with arrows, in red.
		const bus = use.lines.find((l) => l.colour === '#ef4444')!;
		// One segment, then two arrows (4 m and 12 m in) of two segments each.
		expect(bus.positions.length).toBe(6 + 2 * 2 * 6);
		expect(use.undrawn).toBe(0);
	});

	it('draws points as dots, with an upright where a height is set', () => {
		const [observed] = buildLayerScenes([pole, sensor]);
		expect(observed.points.map((p) => p.colour)).toEqual(['#fb923c', '#ef4444']);
		expect(Array.from(observed.lines[0].positions)).toEqual([1, 1, 0, 1, 1, 4]);
	});

	it('counts a polygon with no area as undrawn', () => {
		const sliver = feature(
			'street_use',
			'surface',
			{
				type: 'Polygon',
				coordinates: [
					[
						[0, 0, 0],
						[1, 0, 0],
						[2, 0, 0]
					]
				]
			},
			{ surface: 'median' }
		);
		const flatWall = { ...wall, geometry: sliver.geometry };
		expect(buildLayerScenes([sliver])[0].undrawn).toBe(1);
		expect(buildLayerScenes([flatWall])[0].undrawn).toBe(1);
	});
});

describe('the camera', () => {
	it('boxes every finite triplet', () => {
		expect(
			boundsOfArrays([new Float32Array([1, 2, 3, -1, 5, NaN]), new Float32Array([0, 0, 9])])
		).toEqual({ min: [-1, 0, 3], max: [1, 5, 9] });
		expect(boundsOfArrays([new Float32Array(0)])).toBeNull();
	});

	it('starts south of the middle, looking north and down', () => {
		const { target, position } = homeView({ min: [-50, -30, 0], max: [50, 30, 10] });
		expect(target).toEqual([0, 0, 5]);
		expect(position[0]).toBe(0);
		expect(position[1]).toBeLessThan(-50);
		expect(position[2]).toBeGreaterThan(30);
		expect(homeView(null)).toEqual({ target: [0, 0, 0], position: [0, -48, 36] });
		// A small site is still seen from far enough back.
		expect(homeView({ min: [0, 0, 0], max: [1, 1, 0] }).position[2]).toBeGreaterThan(5);
	});

	it('turns the north arrow by where north lands on screen', () => {
		expect(screenNorth([100, 100], [100, 50])).toBeCloseTo(0);
		expect(screenNorth([100, 100], [150, 100])).toBeCloseTo(90);
		expect(screenNorth([100, 100], [100, 150])).toBeCloseTo(180);
		expect(screenNorth([100, 100], [50, 100])).toBeCloseTo(270);
		expect(screenNorth([100, 100], [100, 100])).toBe(0);
	});
});
