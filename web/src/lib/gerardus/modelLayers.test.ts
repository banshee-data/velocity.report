/**
 * Gerardus's W4–W7 layers in the site viewer, without WebGL: their 2D styles,
 * the property panel, and the 3D builders (building parts as outlines beside
 * their wall surfaces, or raised from min_height to height when a Gerardus
 * sends none, roof planes triangulated at their z, markings on the road,
 * poles as cylinders, signs as boxes, trees as a trunk and a crown).
 */
import type { SiteFeature, SiteFrameFeature } from './client';
import { MODEL_LAYERS } from './client';
import { isUpright } from './geometry';
import {
	boxMesh,
	buildLayerScenes,
	cylinderMesh,
	DEFAULT_DBH_M,
	DEFAULT_POLE_DIAMETER_M,
	ellipsoidMesh,
	partRise,
	poleMesh,
	signMesh,
	treeMeshes,
	treeShape,
	uprightOf,
	type MeshData,
	type Vec3
} from './scene3d';
import {
	crownRadius,
	DEFAULT_CROWN_RADIUS_M,
	featureColour,
	FURNITURE_COLOURS,
	LAYER_COLOURS,
	layerLabel,
	mapStyle,
	MARKING_COLOURS,
	propertyRows,
	ROOF_SHAPE_COLOURS,
	ROOF_SHAPE_UNKNOWN,
	roofShapeColour,
	roofSlopeColour,
	tagNumber,
	TREE_COLOURS
} from './style';

const vertex = (m: MeshData, i: number): Vec3 => [
	m.positions[i * 3],
	m.positions[i * 3 + 1],
	m.positions[i * 3 + 2]
];

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

function centroid(m: MeshData, t: number): Vec3 {
	const [a, b, c] = [0, 1, 2].map((k) => vertex(m, m.indices[t * 3 + k]));
	return [(a[0] + b[0] + c[0]) / 3, (a[1] + b[1] + c[1]) / 3, (a[2] + b[2] + c[2]) / 3];
}

/** Whether every triangle with area faces away from `inside`. */
function facesOut(m: MeshData, inside: Vec3): boolean {
	return normals(m).every((n, t) => {
		if (Math.hypot(...n) < 1e-9) return true;
		const c = centroid(m, t);
		return n[0] * (c[0] - inside[0]) + n[1] * (c[1] - inside[1]) + n[2] * (c[2] - inside[2]) > 0;
	});
}

function area(m: MeshData): number {
	return normals(m).reduce((s, n) => s + Math.hypot(...n) / 2, 0);
}

function zRange(positions: Float32Array): [number, number] {
	let lo = Infinity;
	let hi = -Infinity;
	for (let i = 2; i < positions.length; i += 3) {
		lo = Math.min(lo, positions[i]);
		hi = Math.max(hi, positions[i]);
	}
	return [lo, hi];
}

const frame = (
	layer: string,
	kind: string,
	geometry: SiteFrameFeature['geometry'],
	tags: Record<string, string> = {}
): SiteFrameFeature => ({ layer, id: `${layer}/1`, kind, tags, geometry });

const square = (z: number, size = 4): Vec3[] => [
	[0, 0, z],
	[size, 0, z],
	[size, size, z],
	[0, size, z],
	[0, 0, z]
];

// A bay over a garage: its outline 3 m up, rising to 8 m.
const bay = frame(
	'building_parts',
	'building_part',
	{ type: 'Polygon', coordinates: [square(3)] },
	{ height: '8', min_height: '3', 'roof:shape': 'flat' }
);
const roof = frame(
	'roof_planes',
	'roof_plane',
	{
		type: 'Polygon',
		coordinates: [
			[
				[0, 0, 10],
				[10, 0, 10],
				[10, 5, 12],
				[0, 5, 12],
				[0, 0, 10]
			]
		]
	},
	{ part: 'gms:part/main', slope_deg: '21.8', azimuth_deg: '180', role: 'roof' }
);
const pole = frame(
	'street_furniture',
	'pole',
	{
		type: 'LineString',
		coordinates: [
			[3, 3, 0],
			[3, 3, 6]
		]
	},
	{ pole_type: 'streetlight', diameter_m: '0.3' }
);
const sign = frame(
	'street_furniture',
	'sign',
	{
		type: 'LineString',
		coordinates: [
			[3.2, 3, 2.3],
			[3.2, 3, 3.1]
		]
	},
	{ sign_type: 'R1-1', pole: 'gms:pole/1' }
);
const tree = frame(
	'trees',
	'tree',
	{
		type: 'LineString',
		coordinates: [
			[5, -3, 0],
			[5, -3, 2.5]
		]
	},
	{ dbh_m: '0.4', height_m: '9', crown_base_m: '2.5', crown_radius_m: '3', species: 'Plane' }
);
const marking = (tags: Record<string, string>, closed = false): SiteFrameFeature =>
	frame(
		'lane_markings',
		'lane_marking',
		closed
			? { type: 'Polygon', coordinates: [square(0.1, 3)] }
			: {
					type: 'LineString',
					coordinates: [
						[-20, 0, 0.1],
						[20, 0, 0.2]
					]
				},
		tags
	);

// ---- Styles ----------------------------------------------------------------------

describe('the W4–W7 styles', () => {
	const f = (layer: string, kind = '', tags: Record<string, string> = {}) => ({
		layer,
		kind,
		tags
	});

	it('gives each layer a label and a colour of its own', () => {
		expect(MODEL_LAYERS.map(layerLabel)).toEqual([
			'building parts',
			'roof planes',
			'lane markings',
			'poles and signs',
			'trees'
		]);
		const colours = MODEL_LAYERS.map((l) => LAYER_COLOURS[l]);
		expect(colours.every(Boolean)).toBe(true);
		expect(new Set(colours).size).toBe(MODEL_LAYERS.length);
	});

	it('colours building parts by roof:shape and roof planes by slope', () => {
		expect(featureColour(f('building_parts', 'building_part', { 'roof:shape': 'gabled' }))).toBe(
			ROOF_SHAPE_COLOURS.gabled
		);
		expect(
			featureColour(f('building_parts', 'building_part', { 'gms:roof_shape': 'unknown' }))
		).toBe(ROOF_SHAPE_UNKNOWN);
		expect(roofShapeColour('many')).toBe(ROOF_SHAPE_UNKNOWN);
		expect(new Set(Object.values(ROOF_SHAPE_COLOURS)).size).toBe(
			Object.keys(ROOF_SHAPE_COLOURS).length
		);
		expect(Object.values(ROOF_SHAPE_COLOURS)).not.toContain(ROOF_SHAPE_UNKNOWN);

		expect(roofSlopeColour(0)).toBe('#60a5fa');
		expect(roofSlopeColour('22.5')).toBe('#facc15');
		expect(roofSlopeColour(45)).toBe('#ef4444');
		expect(roofSlopeColour(70)).toBe('#ef4444');
		expect(roofSlopeColour(-5)).toBe('#60a5fa');
		// Between the stops, between their colours.
		expect(roofSlopeColour(11.25)).toMatch(/^#[0-9a-f]{6}$/);
		expect(roofSlopeColour(11.25)).not.toBe(roofSlopeColour(0));
		expect(roofSlopeColour(undefined)).toBe(LAYER_COLOURS.roof_planes);
		expect(roofSlopeColour('steep')).toBe(LAYER_COLOURS.roof_planes);
		expect(featureColour(f('roof_planes', 'roof_plane', { slope_deg: '45' }))).toBe('#ef4444');
	});

	it('colours markings by their colour, furniture by kind, trees by their crown', () => {
		expect(featureColour(f('lane_markings', 'lane_marking', { colour: 'yellow' }))).toBe(
			MARKING_COLOURS.yellow
		);
		expect(featureColour(f('lane_markings', 'lane_marking', { colour: 'white' }))).toBe(
			MARKING_COLOURS.white
		);
		expect(featureColour(f('lane_markings', 'lane_marking'))).toBe(LAYER_COLOURS.lane_markings);
		expect(featureColour(f('street_furniture', 'pole'))).toBe(FURNITURE_COLOURS.pole);
		expect(featureColour(f('street_furniture', 'sign'))).toBe(FURNITURE_COLOURS.sign);
		expect(featureColour(f('street_furniture', 'bench'))).toBe(LAYER_COLOURS.street_furniture);
		expect(featureColour(f('trees', 'tree'))).toBe(TREE_COLOURS.crown);
		expect(TREE_COLOURS.trunk).not.toBe(TREE_COLOURS.crown);
	});

	it('draws them on the 2D map: parts under roofs, markings dashed as painted', () => {
		const part = mapStyle(f('building_parts', 'building_part'));
		const roofs = mapStyle(f('roof_planes', 'roof_plane'));
		expect(part.rank).toBeLessThan(roofs.rank);
		expect(roofs.fillOpacity).toBeGreaterThan(part.fillOpacity);
		expect(mapStyle(f('lane_markings', 'lane_marking', { pattern: 'dashed' })).dashArray).toBe(
			'6 6'
		);
		expect(mapStyle(f('lane_markings', 'lane_marking', { pattern: 'solid' })).dashArray).toBe(
			undefined
		);
		expect(
			mapStyle(f('lane_markings', 'lane_marking', { pattern: 'double' })).weight
		).toBeGreaterThan(mapStyle(f('lane_markings', 'lane_marking')).weight);
		expect(mapStyle(f('trees', 'tree')).fillOpacity).toBeGreaterThan(0);
		expect(mapStyle(f('street_furniture', 'sign')).radius).toBeLessThan(
			mapStyle(f('street_furniture', 'pole')).radius
		);
	});

	it('reads tag numbers, and a crown radius with its default', () => {
		expect(tagNumber({ height: '12' }, 'height')).toBe(12);
		expect(tagNumber({ height: '12 m' }, 'height')).toBe(12);
		expect(tagNumber({}, 'height')).toBeNaN();
		expect(crownRadius({ crown_radius_m: '3.5' })).toBe(3.5);
		expect(crownRadius({ crown_radius_m: '0' })).toBe(DEFAULT_CROWN_RADIUS_M);
		expect(crownRadius({})).toBe(DEFAULT_CROWN_RADIUS_M);
	});

	it('shows a part’s roof:shape, model and state in the property panel', () => {
		const feature: SiteFeature = {
			layer: 'building_parts',
			id: 'gms:part/bay',
			kind: 'building_part',
			tags: { 'roof:shape': 'hipped', height: '8', min_height: '3' },
			modelId: 'sha256:buildings',
			state: 'proposed',
			geometry: { type: 'Polygon', coordinates: [] }
		};
		expect(propertyRows(feature)).toEqual([
			['layer', 'building_parts'],
			['id', 'gms:part/bay'],
			['kind', 'building_part'],
			['model_id', 'sha256:buildings'],
			['state', 'proposed'],
			['height', '8'],
			['min_height', '3'],
			['roof:shape', 'hipped'],
			['geometry', 'Polygon']
		]);
	});

	it('tells an upright line, seen from above a point, from one on the ground', () => {
		expect(
			isUpright([
				[3, 3, 0],
				[3, 3, 6]
			])
		).toBe(true);
		expect(
			isUpright([
				[-122.4, 37.8, 1],
				[-122.4, 37.8, 7]
			])
		).toBe(true);
		expect(
			isUpright([
				[0, 0, 0],
				[1, 0, 0]
			])
		).toBe(false);
		expect(isUpright([[3, 3, 0]])).toBe(false);
	});
});

// ---- Building parts and roofs --------------------------------------------------------

describe('building parts', () => {
	it('rise from min_height to height above the ground', () => {
		expect(partRise({ height: '8', min_height: '3' })).toBe(5);
		expect(partRise({ height: '12' })).toBe(12);
		// A part whose base is above its top has no height.
		expect(partRise({ height: '3', min_height: '5' })).toBe(0);
		// height_m stands in when there is no height.
		expect(partRise({ height_m: '6' })).toBe(6);
		expect(partRise({})).toBe(0);
	});

	it('raise the outline at its base, min_height up, to height', () => {
		const [scene] = buildLayerScenes([bay]);
		expect(scene.layer).toBe('building_parts');
		expect(scene.undrawn).toBe(0);
		expect(scene.meshes).toHaveLength(1);
		expect(scene.meshes[0].colour).toBe(ROOF_SHAPE_COLOURS.flat);
		const mesh = scene.meshes[0].mesh;
		// The outline is at 3 m (min_height above the ground at 0): the part
		// stands from there to 8 m.
		expect(zRange(mesh.positions)).toEqual([3, 8]);
		// Four walls of two triangles, and a top of two.
		expect(mesh.indices.length / 3).toBe(4 * 2 + 2);
		expect(facesOut(mesh, [2, 2, 5.5])).toBe(true);
		// The outline at the base and again at the top.
		const lines = scene.lines[0].positions;
		expect(lines.length / 6).toBe(8);
		expect(zRange(lines)).toEqual([3, 8]);
	});

	it('are outlines only when wall surfaces arrive, and their walls stop at the roofline', () => {
		// The bay's height tag says 8 m (the top of its roof): raised to it, its
		// walls would stand over the eave at 7 m.
		const eave = frame(
			'wall_surfaces',
			'wall_surface',
			{
				type: 'Polygon',
				coordinates: [
					[
						[0, 0, 3],
						[4, 0, 3],
						[4, 0, 7],
						[0, 0, 7],
						[0, 0, 3]
					]
				]
			},
			{ part: 'building_parts/1', edge: '0', role: 'wall', top: 'roofline' }
		);
		// The gable end: its top rises to the ridge in the middle.
		const gable = frame(
			'wall_surfaces',
			'wall_surface',
			{
				type: 'Polygon',
				coordinates: [
					[
						[4, 0, 3],
						[4, 4, 3],
						[4, 4, 7],
						[4, 2, 8],
						[4, 0, 7],
						[4, 0, 3]
					]
				]
			},
			{ part: 'building_parts/1', edge: '1', role: 'gable', top: 'roofline' }
		);
		const scenes = buildLayerScenes([bay, eave, gable]);
		expect(scenes.map((s) => s.layer)).toEqual(['building_parts', 'wall_surfaces']);
		const [parts, walls] = scenes;
		// The part: its base outline, no fill and nothing at its height.
		expect(parts.meshes).toHaveLength(0);
		expect(parts.undrawn).toBe(0);
		expect(parts.lines[0].positions.length / 6).toBe(4);
		expect(zRange(parts.lines[0].positions)).toEqual([3, 3]);
		// The walls as they come: a quad and a five-sided gable, filled in their
		// own planes and outlined.
		expect(walls.undrawn).toBe(0);
		expect(walls.meshes).toHaveLength(1);
		expect(walls.meshes[0]).toMatchObject({
			colour: LAYER_COLOURS.wall_surfaces,
			doubleSided: true
		});
		const mesh = walls.meshes[0].mesh;
		expect(mesh.indices.length / 3).toBe(2 + 3);
		// The eave wall's 4 × 4 m, and the gable's 4 × 4 m with 2 m² above it.
		expect(area(mesh)).toBeCloseTo(16 + 18, 4);
		expect(zRange(mesh.positions)).toEqual([3, 8]);
		expect(walls.lines[0].positions.length / 6).toBe(4 + 5);
	});

	it('draw a part without a height as its outline, and count it undrawn', () => {
		const flat = { ...bay, tags: { 'roof:shape': 'flat' } };
		const [scene] = buildLayerScenes([flat]);
		expect(scene.meshes).toHaveLength(0);
		expect(scene.lines[0].positions.length / 6).toBe(4);
		expect(scene.undrawn).toBe(1);
	});

	it('triangulate a roof plane at its z, coloured by slope', () => {
		const [scene] = buildLayerScenes([roof]);
		expect(scene.meshes).toHaveLength(1);
		const spec = scene.meshes[0];
		expect(spec.colour).toBe(roofSlopeColour(21.8));
		expect(spec.doubleSided).toBe(true);
		expect(spec.opacity).toBeLessThan(1);
		expect(spec.mesh.indices.length / 3).toBe(2);
		expect(zRange(spec.mesh.positions)).toEqual([10, 12]);
		// The tilted face's whole area: 10 m by √(5² + 2²).
		expect(area(spec.mesh)).toBeCloseTo(10 * Math.hypot(5, 2), 4);
		expect(scene.lines[0].positions.length / 6).toBe(4);
	});
});

// ---- Markings ------------------------------------------------------------------------

describe('lane markings', () => {
	it('lie on the road at its z, just above the surfaces', () => {
		const [scene] = buildLayerScenes([marking({ colour: 'yellow' })]);
		expect(scene.lines).toHaveLength(1);
		expect(scene.lines[0].colour).toBe(MARKING_COLOURS.yellow);
		const [lo, hi] = zRange(scene.lines[0].positions);
		expect(lo).toBeCloseTo(0.15, 5);
		expect(hi).toBeCloseTo(0.25, 5);
	});

	it('fill an area marking, such as a crosswalk', () => {
		const [scene] = buildLayerScenes([marking({ marking: 'crosswalk', colour: 'white' }, true)]);
		expect(scene.meshes).toHaveLength(1);
		expect(scene.meshes[0].colour).toBe(MARKING_COLOURS.white);
		expect(area(scene.meshes[0].mesh)).toBeCloseTo(9, 4);
		expect(scene.lines[0].positions.length / 6).toBe(4);
	});
});

// ---- Poles and signs -------------------------------------------------------------------

describe('cylinders and boxes', () => {
	it('builds a cylinder from base to top, faced out, capped on top', () => {
		const m = cylinderMesh([0, 0, 0], [0, 0, 6], 0.15, 12);
		expect(m.positions.length / 3).toBe(12 * 2 + 1);
		// Two triangles per side quad and one per cap slice.
		expect(m.indices.length / 3).toBe(12 * 3);
		expect(zRange(m.positions)).toEqual([0, 6]);
		for (let i = 0; i < 24; i++) {
			const [x, y] = vertex(m, i);
			expect(Math.hypot(x, y)).toBeCloseTo(0.15, 5);
		}
		expect(facesOut(m, [0, 0, 3])).toBe(true);
		// Tilted, it still runs base to top.
		const leaning = cylinderMesh([0, 0, 0], [1, 0, 1], 0.1);
		expect(facesOut(leaning, [0.5, 0, 0.5])).toBe(true);
		expect(cylinderMesh([0, 0, 0], [0, 0, 0], 0.1).indices).toHaveLength(0);
		expect(cylinderMesh([0, 0, 0], [0, 0, 1], 0).indices).toHaveLength(0);
	});

	it('stands a pole as a cylinder of its diameter, 0.2 m when it does not say', () => {
		const m = poleMesh([3, 3, 0], [3, 3, 6], { diameter_m: '0.3' });
		const [x, y] = vertex(m, 0);
		expect(Math.hypot(x - 3, y - 3)).toBeCloseTo(0.15, 5);
		const bare = poleMesh([3, 3, 0], [3, 3, 6], {});
		const [bx, by] = vertex(bare, 0);
		expect(Math.hypot(bx - 3, by - 3)).toBeCloseTo(DEFAULT_POLE_DIAMETER_M / 2, 5);

		const [scene] = buildLayerScenes([pole]);
		expect(scene.layer).toBe('street_furniture');
		expect(scene.meshes).toHaveLength(1);
		expect(scene.meshes[0].colour).toBe(FURNITURE_COLOURS.pole);
		expect(scene.meshes[0].opacity).toBe(1);
		expect(zRange(scene.meshes[0].mesh.positions)).toEqual([0, 6]);
	});

	it('builds an axis-aligned box faced out', () => {
		const m = boxMesh([1, 2, 3], [2, 4, 6]);
		expect(m.indices.length / 3).toBe(12);
		expect(area(m)).toBeCloseTo(2 * (2 * 4 + 4 * 6 + 2 * 6), 4);
		expect(facesOut(m, [1, 2, 3])).toBe(true);
		expect(zRange(m.positions)).toEqual([0, 6]);
		expect(boxMesh([0, 0, 0], [1, 0, 1]).indices).toHaveLength(0);
	});

	it('draws a sign as a small box over its face', () => {
		const m = signMesh([3.2, 3, 2.3], [3.2, 3, 3.1]);
		const [lo, hi] = zRange(m.positions);
		expect(lo).toBeCloseTo(2.3, 5);
		expect(hi).toBeCloseTo(3.1, 5);
		const [scene] = buildLayerScenes([sign]);
		expect(scene.meshes[0].colour).toBe(FURNITURE_COLOURS.sign);
		expect(scene.meshes[0].mesh.indices.length / 3).toBe(12);
	});

	it('finds the upright of a feature, base first, and counts a pole with no top undrawn', () => {
		const flipped = {
			...pole,
			geometry: {
				type: 'LineString' as const,
				coordinates: [
					[3, 3, 6],
					[3, 3, 0]
				] as Vec3[]
			}
		};
		expect(uprightOf(flipped)).toEqual({ base: [3, 3, 0], top: [3, 3, 6] });
		const point = { ...pole, geometry: { type: 'Point' as const, coordinates: [3, 3, 0] as Vec3 } };
		expect(uprightOf(point)).toEqual({ base: [3, 3, 0], top: null });
		const [scene] = buildLayerScenes([point]);
		expect(scene.meshes).toHaveLength(0);
		expect(scene.undrawn).toBe(1);
	});
});

// ---- Trees ----------------------------------------------------------------------------

describe('trees', () => {
	it('read the trunk and crown from their tags, heights from the trunk’s base', () => {
		const shape = treeShape([5, -3, 10], tree.tags);
		expect(shape.trunkRadius).toBeCloseTo(0.2, 6);
		expect(shape.crownBottom).toBe(12.5);
		expect(shape.crownTop).toBe(19);
		expect(shape.crownRadius).toBe(3);
		// With no tags: the crown starts at the segment's top and is as tall as it is wide.
		const bare = treeShape([5, -3, 10], {}, [5, -3, 12]);
		expect(bare.trunkRadius).toBeCloseTo(DEFAULT_DBH_M / 2, 6);
		expect(bare.crownBottom).toBe(12);
		expect(bare.crownTop).toBe(12 + 2 * DEFAULT_CROWN_RADIUS_M);
	});

	it('build a trunk cylinder and a crown ellipsoid filling crown_base_m to height_m', () => {
		const { trunk, crown } = treeMeshes(treeShape([5, -3, 0], tree.tags));
		expect(zRange(trunk.positions)).toEqual([0, 2.5]);
		const [x, y] = vertex(trunk, 0);
		expect(Math.hypot(x - 5, y + 3)).toBeCloseTo(0.2, 5);
		const [lo, hi] = zRange(crown.positions);
		expect(lo).toBeCloseTo(2.5, 5);
		expect(hi).toBeCloseTo(9, 5);
		// Widest at its middle, crown_radius_m across.
		let widest = 0;
		for (let i = 0; i < crown.positions.length / 3; i++) {
			const [cx, cy] = vertex(crown, i);
			widest = Math.max(widest, Math.hypot(cx - 5, cy + 3));
		}
		expect(widest).toBeCloseTo(3, 5);
		expect(facesOut(crown, [5, -3, 5.75])).toBe(true);
	});

	it('draw as a trunk and a translucent crown in a layer of their own', () => {
		const [scene] = buildLayerScenes([tree]);
		expect(scene.layer).toBe('trees');
		expect(scene.meshes.map((m) => m.colour)).toEqual([TREE_COLOURS.trunk, TREE_COLOURS.crown]);
		expect(scene.meshes[1].opacity).toBeLessThan(1);
		expect(scene.undrawn).toBe(0);
	});

	it('builds a closed ellipsoid, and nothing without a size', () => {
		const m = ellipsoidMesh([0, 0, 0], [2, 2, 1], 16, 8);
		expect(m.positions.length / 3).toBe(9 * 16);
		// Two triangles per quad, one at each polar band.
		expect(m.indices.length / 3).toBe(16 * (2 * 8 - 2));
		expect(facesOut(m, [0, 0, 0])).toBe(true);
		expect(ellipsoidMesh([0, 0, 0], [1, 1, 0]).indices).toHaveLength(0);
	});
});

describe('a site with every W4–W7 layer', () => {
	it('keeps each layer apart, so each can be shown and hidden', () => {
		const scenes = buildLayerScenes([bay, roof, marking({ colour: 'white' }), pole, sign, tree]);
		expect(scenes.map((s) => s.layer)).toEqual([
			'building_parts',
			'roof_planes',
			'lane_markings',
			'street_furniture',
			'trees'
		]);
		expect(scenes.every((s) => s.undrawn === 0)).toBe(true);
		const furniture = scenes.find((s) => s.layer === 'street_furniture');
		expect(furniture?.meshes.map((m) => m.colour)).toEqual([
			FURNITURE_COLOURS.pole,
			FURNITURE_COLOURS.sign
		]);
	});
});
