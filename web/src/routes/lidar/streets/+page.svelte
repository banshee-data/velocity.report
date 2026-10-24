<script lang="ts">
	/**
	 * Streets — mark a site's driveways, parked-car spaces and travel lanes
	 * over Gerardus's layers (Gerardus docs/plans/VECTOR-MAP.md §4.1).
	 *
	 * No public layer says which stretch of kerb is a driveway, where cars park
	 * or where lanes run, so an operator marks them, once per site, over geometry
	 * that is already public or measured: the City's street centrelines and
	 * intersections, kerbs and islands, and the DataSF boxes. The page reads
	 * those from Gerardus as GeoJSON and saves the marks to it in one batch,
	 * with the operator's name and a note. Gerardus snaps, drapes and checks
	 * every edit again, and its problems are shown beside the marks they name.
	 *
	 * The browser talks to Gerardus directly (lib/streets/gerardus), at the
	 * address in Settings. Drawing snaps each vertex, as a preview, to kerbs,
	 * centrelines and other marks within 0.5 m (lib/streets/geometry).
	 *
	 * OpenStreetMap tiles are an optional backdrop behind a consent asked once
	 * per visit, as the site map editor asks; nothing is snapped to or derived
	 * from them, and without them the layers are drawn on a plain dark ground.
	 */
	import { getClipLocations } from '#lib/api.js';
	import type { EN } from '#lib/align/pose.js';
	import {
		addMark,
		canUndo,
		LANE_TYPES,
		marksFromFeatures,
		moveVertex,
		newKey,
		pendingEdits,
		removeMark,
		removeVertex,
		reverseMark,
		setTag,
		SPACE_TYPES,
		startEditing,
		tagsFor,
		undo,
		updateMark,
		type EditState,
		type Mark
	} from '#lib/streets/editor.js';
	import {
		boundsOf,
		distance,
		enOf,
		laneArrows,
		partsOf,
		positionOf,
		rowDividers,
		rowOfSpaces,
		snapPoint,
		streetSide,
		targetsOf,
		type Snap,
		type SnapTarget
	} from '#lib/streets/geometry.js';
	import {
		GerardusError,
		gerardusAddress,
		getSiteGeojson,
		isL16Token,
		mapProblems,
		postStreetUse,
		rememberAuthor,
		rememberedAuthor,
		rememberGerardusAddress,
		type SiteFeature,
		type StreetUseKind
	} from '#lib/streets/gerardus.js';
	import { mdiClose } from '@mdi/js';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import 'leaflet/dist/leaflet.css';
	import type {
		LatLngExpression,
		LayerGroup,
		LeafletMouseEvent,
		Map as LeafletMap,
		Path,
		Renderer,
		TileLayer
	} from 'leaflet';
	import { onDestroy, onMount, tick } from 'svelte';
	import { Button } from 'svelte-ux';

	interface KnownSite {
		token: string;
		label: string;
		lat?: number;
		lon?: number;
		clips: number;
	}

	type Tool = 'select' | 'driveway' | 'space' | 'row' | 'lane';
	const TOOLS: { id: Tool; label: string; hint: string }[] = [
		{ id: 'select', label: 'Select', hint: 'Click a mark to edit it; drag its vertices.' },
		{ id: 'driveway', label: 'Driveway', hint: 'Click the two ends of the kerb cut.' },
		{
			id: 'space',
			label: 'Space',
			hint: 'Click the corners of the space; Enter or double-click finishes.'
		},
		{
			id: 'row',
			label: 'Row of spaces',
			hint: 'Click two points along the kerb: the row runs between them, on the street side.'
		},
		{
			id: 'lane',
			label: 'Travel lane',
			hint: 'Click along the lane in its direction of travel; Enter or double-click finishes.'
		}
	];
	const KIND_LABEL: Record<StreetUseKind, string> = {
		driveway: 'Driveway',
		parking_space: 'Parking space',
		travel_lane: 'Travel lane'
	};
	const LAYER_LABEL: Record<string, string> = {
		city_streets: 'city streets',
		city_intersections: 'intersections',
		kerbs: 'kerbs and islands',
		boxes: 'boxes',
		footprints: 'footprints',
		street_use: 'street use'
	};
	const SPACE_COLOURS: Record<string, string> = {
		metered: '#3b82f6',
		permit: '#a855f7',
		loading: '#eab308',
		disabled: '#06b6d4',
		free: '#10b981'
	};
	const LANE_COLOURS: Record<string, string> = {
		general: '#e5e7eb',
		bus: '#ef4444',
		bike: '#22c55e',
		turn: '#f97316'
	};
	const DRIVEWAY_COLOUR = '#f472b6';
	const SF = { lat: 37.7749, lon: -122.4194 };

	// ---- Gerardus and the site ------------------------------------------------

	let address = gerardusAddress();
	let addressInput = address;
	let sites: KnownSite[] = [];
	let sitesError: string | null = null;
	let siteChoice = '';
	let tokenInput = page.url.searchParams.get('site') ?? '';
	let siteToken = '';
	let loading = false;
	let loadError: GerardusError | Error | null = null;
	let loadAbort: AbortController | null = null;
	let reference: SiteFeature[] = [];
	let layerCounts: Record<string, number> = {};
	let skipped = 0;
	let origin = SF;

	let edit: EditState = startEditing([]);
	let problemsById = new Map<string, string[]>();
	let generalProblems: string[] = [];

	// ---- Tools ------------------------------------------------------------------

	let tool: Tool = 'select';
	let selectedKey: string | null = null;
	let drawing: EN[] = [];
	let cursor: Snap | null = null;
	let spaceType: string = 'metered';
	let count = 1;
	let timeLimit = '';
	let laneType: string = 'general';
	let widthM = 3.3;
	let rowDepth = 2.4;
	let rowFlip = false;

	// ---- Save ---------------------------------------------------------------------

	let author = rememberedAuthor();
	let note = '';
	let saving = false;
	let saveError: string | null = null;
	let savedMessage: string | null = null;

	// ---- The map --------------------------------------------------------------------

	let L: typeof import('leaflet') | null = null;
	let map: LeafletMap | null = null;
	let mapContainer: HTMLDivElement;
	let canvasRenderer: Renderer | null = null;
	let referenceLayer: LayerGroup | null = null;
	let marksLayer: LayerGroup | null = null;
	let vertexLayer: LayerGroup | null = null;
	let draftLayer: LayerGroup | null = null;

	// Consent for OpenStreetMap tiles lasts for this visit, as in the site map editor.
	let osmAllowed = false;
	let askingOsm = false;
	let showTiles = false;
	let tileLayer: TileLayer | null = null;

	$: edits = pendingEdits(edit);
	$: dirty = edits.length > 0;
	$: selected = edit.marks.find((m) => m.key === selectedKey) ?? null;
	$: activeTool = TOOLS.find((t) => t.id === tool)!;
	$: knownSite = sites.find((s) => s.token === siteToken) ?? null;

	// Snap targets in metres about the origin.
	$: refTargets = reference.flatMap((f) => targetsOf(origin, f));
	$: kerbTargets = refTargets.filter((t) => t.layer === 'kerbs');
	$: centrelineTargets = refTargets.filter((t) => t.layer === 'city_streets');
	$: wallTargets = refTargets.filter((t) => t.layer === 'boxes' || t.layer === 'footprints');
	$: markTargets = edit.marks.map((m): SnapTarget => ({
		id: m.key,
		layer: 'street_use',
		points: m.points.map((p) => enOf(origin, p)),
		closed: m.kind === 'parking_space'
	}));
	// Driveways and rows of spaces sit on the kerb, or on a box wall's foot until kerbs exist.
	$: snapTargets =
		tool === 'driveway' || tool === 'row'
			? [...kerbTargets, ...wallTargets, ...centrelineTargets, ...markTargets]
			: [...kerbTargets, ...centrelineTargets, ...markTargets];

	$: if (map && L) drawReference(reference);
	$: if (map && L) drawMarks(edit.marks, selectedKey, problemsById, tool);
	$: if (map && L) drawDraft(drawing, cursor, tool, count, rowDepth, rowFlip);
	$: if (map) {
		if (tool === 'select') map.doubleClickZoom.enable();
		else map.doubleClickZoom.disable();
	}
	$: if (map && L) syncTiles(osmAllowed && showTiles);

	onMount(async () => {
		L = await import('leaflet');
		await tick();
		initMap();
		void loadSites();
		if (tokenInput) void loadSite(tokenInput.trim());
	});

	onDestroy(() => {
		loadAbort?.abort();
		map?.remove();
	});

	function initMap() {
		if (!L || !mapContainer) return;
		map = L.map(mapContainer, {
			center: [origin.lat, origin.lon],
			zoom: 13,
			maxZoom: 23,
			zoomSnap: 0.5
		});
		canvasRenderer = L.canvas({ padding: 0.5 });
		referenceLayer = L.layerGroup().addTo(map);
		marksLayer = L.layerGroup().addTo(map);
		draftLayer = L.layerGroup().addTo(map);
		vertexLayer = L.layerGroup().addTo(map);
		map.on('mousemove', onMove);
		map.on('mouseout', () => (cursor = null));
		map.on('click', onClick);
		map.on('dblclick', () => {
			if (tool === 'space' || tool === 'lane') finishDrawing();
		});
	}

	// ---- Loading ----------------------------------------------------------------------

	async function loadSites() {
		try {
			const r = await getClipLocations();
			sites = r.districts
				.flatMap((d) => d.sites)
				.map((s) => ({
					token: s.s2_l16_token,
					label: s.label || s.clips[0]?.description || '',
					lat: s.canonical_lat ?? s.clips[0]?.origin_lat,
					lon: s.canonical_lon ?? s.clips[0]?.origin_lon,
					clips: s.clips.length
				}))
				.sort((a, b) => (a.label || a.token).localeCompare(b.label || b.token));
			siteChoice = sites.some((s) => s.token === siteToken) ? siteToken : '';
		} catch (e) {
			// The list is a convenience: a token can still be typed.
			sites = [];
			sitesError = e instanceof Error ? e.message : 'Could not list the located sites.';
		}
	}

	function siteLabel(s: KnownSite): string {
		const clips = `${s.clips} clip${s.clips === 1 ? '' : 's'}`;
		return s.label ? `${s.label} · ${s.token} · ${clips}` : `${s.token} · ${clips}`;
	}

	async function loadSite(token: string) {
		if (!token) return;
		if (!isL16Token(token)) {
			loadError = new Error(`“${token}” is not an L16 site token (hex digits, such as 808580f3f).`);
			return;
		}
		if (dirty && token !== siteToken) {
			if (!confirm('Discard the unsaved street-use changes for this site?')) return;
		}
		loadAbort?.abort();
		const abort = new AbortController();
		loadAbort = abort;
		siteToken = token;
		tokenInput = token;
		siteChoice = sites.some((s) => s.token === token) ? token : '';
		loading = true;
		loadError = null;
		savedMessage = null;
		saveError = null;
		try {
			const { features, skipped: bad } = await getSiteGeojson(address, token, undefined, {
				signal: abort.signal
			});
			if (abort.signal.aborted) return;
			const streetUse = features.filter((f) => f.layer === 'street_use');
			const { marks, skipped: unfit } = marksFromFeatures(streetUse);
			reference = features.filter((f) => f.layer !== 'street_use');
			layerCounts = {};
			for (const f of features) layerCounts[f.layer] = (layerCounts[f.layer] ?? 0) + 1;
			skipped = bad + unfit;
			setOrigin(features);
			edit = startEditing(marks);
			problemsById = new Map();
			generalProblems = [];
			selectedKey = null;
			drawing = [];
			fitSite(features);
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') return;
			loadError = e instanceof Error ? e : new Error('Could not load the site.');
			reference = [];
			layerCounts = {};
			edit = startEditing([]);
		} finally {
			if (loadAbort === abort) loading = false;
		}
	}

	function setOrigin(features: SiteFeature[]) {
		const b = boundsOf(features);
		if (b) origin = { lat: (b.south + b.north) / 2, lon: (b.west + b.east) / 2 };
		else if (knownSite?.lat != null && knownSite.lon != null)
			origin = { lat: knownSite.lat, lon: knownSite.lon };
	}

	function fitSite(features: SiteFeature[]) {
		if (!map) return;
		const b = boundsOf(features);
		if (b) {
			map.fitBounds(
				[
					[b.south, b.west],
					[b.north, b.east]
				],
				{ padding: [24, 24], maxZoom: 20 }
			);
		} else {
			map.setView([origin.lat, origin.lon], 19);
		}
	}

	function applyAddress() {
		address = rememberGerardusAddress(addressInput);
		addressInput = address;
		// Unsaved marks stay: the next Save goes to the new address.
		if (siteToken && !dirty) void loadSite(siteToken);
	}

	function reload() {
		if (dirty && !confirm('Discard the unsaved street-use changes and load the site again?'))
			return;
		edit = startEditing(edit.base);
		void loadSite(siteToken);
	}

	// ---- OpenStreetMap backdrop --------------------------------------------------------

	function toggleTiles() {
		if (showTiles) {
			showTiles = false;
			return;
		}
		if (osmAllowed) showTiles = true;
		else askingOsm = true;
	}

	function allowOsm() {
		osmAllowed = true;
		askingOsm = false;
		showTiles = true;
	}

	function syncTiles(on: boolean) {
		// The gate lives here: no path adds tiles without this visit's consent.
		if (!L || !map) return;
		if (on && osmAllowed && !tileLayer) {
			tileLayer = L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
				attribution: '© OpenStreetMap contributors',
				maxZoom: 23,
				maxNativeZoom: 19,
				opacity: 0.55
			}).addTo(map);
			tileLayer.bringToBack();
		} else if (!on && tileLayer) {
			tileLayer.remove();
			tileLayer = null;
		}
	}

	// ---- Drawing --------------------------------------------------------------------

	function latLngOf(p: EN): [number, number] {
		const [lon, lat] = positionOf(origin, p);
		return [lat, lon];
	}

	function snapAt(lat: number, lon: number, excludeKey?: string): Snap {
		return snapPoint(enOf(origin, [lon, lat]), snapTargets, undefined, excludeKey);
	}

	function onMove(e: LeafletMouseEvent) {
		if (tool === 'select' || !siteToken) {
			if (cursor) cursor = null;
			return;
		}
		cursor = snapAt(e.latlng.lat, e.latlng.lng);
	}

	function onClick(e: LeafletMouseEvent) {
		if (!siteToken || loading) return;
		if (tool === 'select') {
			selectedKey = null;
			return;
		}
		const at = snapAt(e.latlng.lat, e.latlng.lng).point;
		const last = drawing[drawing.length - 1];
		// A double-click lands twice on one spot: keep one vertex.
		if (last && distance(last, at) < 0.01) return;
		drawing = [...drawing, at];
		if ((tool === 'driveway' || tool === 'row') && drawing.length === 2) finishDrawing();
	}

	function setTool(next: Tool) {
		tool = next;
		drawing = [];
		cursor = null;
	}

	function newMark(kind: StreetUseKind, points: EN[], tags: Record<string, string>): Mark {
		return {
			key: newKey(),
			id: '',
			kind,
			points: points.map((p) => positionOf(origin, p)),
			tags
		};
	}

	function finishDrawing() {
		const pts = drawing;
		const fields = { spaceType, count, timeLimit, laneType, widthM };
		let mark: Mark | null = null;
		if (tool === 'driveway' && pts.length === 2) {
			mark = newMark('driveway', pts, {});
		} else if (tool === 'row' && pts.length === 2) {
			const side = streetSide(pts[0], pts[1], centrelineTargets) * (rowFlip ? -1 : 1);
			const row = rowOfSpaces(
				pts[0],
				pts[1],
				Math.max(1, Math.round(count)),
				rowDepth,
				side as 1 | -1
			);
			if (row) mark = newMark('parking_space', row.ring, tagsFor('parking_space', fields));
		} else if (tool === 'space' && pts.length >= 3) {
			mark = newMark('parking_space', pts, tagsFor('parking_space', fields));
		} else if (tool === 'lane' && pts.length >= 2) {
			mark = newMark('travel_lane', pts, tagsFor('travel_lane', fields));
		} else {
			// Not enough vertices yet: keep drawing.
			if (pts.length > 0 && (tool === 'space' || tool === 'lane')) return;
		}
		drawing = [];
		if (mark) {
			edit = addMark(edit, mark);
			selectedKey = mark.key;
			savedMessage = null;
		}
	}

	function cancelDrawing() {
		drawing = [];
	}

	function doUndo() {
		if (drawing.length > 0) {
			drawing = drawing.slice(0, -1);
			return;
		}
		edit = undo(edit);
		if (selectedKey && !edit.marks.some((m) => m.key === selectedKey)) selectedKey = null;
	}

	function deleteSelected() {
		if (!selectedKey) return;
		edit = removeMark(edit, selectedKey);
		selectedKey = null;
	}

	function changeSelected(fn: (m: Mark) => Mark) {
		if (!selectedKey) return;
		edit = updateMark(edit, selectedKey, fn);
	}

	function onKey(e: KeyboardEvent) {
		const target = e.target as HTMLElement | null;
		if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return;
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'z') {
			e.preventDefault();
			doUndo();
		} else if (e.key === 'Escape') {
			if (drawing.length > 0) cancelDrawing();
			else selectedKey = null;
		} else if (e.key === 'Enter') {
			if (drawing.length > 0) finishDrawing();
		} else if (e.key === 'Backspace' && drawing.length > 0) {
			e.preventDefault();
			drawing = drawing.slice(0, -1);
		} else if ((e.key === 'Delete' || e.key === 'Backspace') && selectedKey) {
			e.preventDefault();
			deleteSelected();
		}
	}

	function onBeforeUnload(e: BeforeUnloadEvent) {
		if (dirty) e.preventDefault();
	}

	// ---- Rendering -------------------------------------------------------------------

	function ring(points: [number, number][]): LatLngExpression[] {
		return points.map(([lon, lat]) => [lat, lon] as [number, number]);
	}

	function drawReference(features: SiteFeature[]) {
		if (!L || !referenceLayer) return;
		referenceLayer.clearLayers();
		const base = { renderer: canvasRenderer ?? undefined, interactive: false };
		for (const f of features) {
			for (const part of partsOf(f.geometry)) {
				const pts = ring(part.points as [number, number][]);
				if (f.layer === 'city_intersections' || part.points.length === 1) {
					L.circleMarker(pts[0], {
						...base,
						radius: 4,
						color: '#cbd5e1',
						weight: 1.5,
						fillOpacity: 0.2
					}).addTo(referenceLayer);
				} else if (f.layer === 'kerbs') {
					const style = { ...base, color: '#f8fafc', weight: 3, opacity: 0.95 };
					if (part.closed)
						L.polygon(pts, { ...style, fillColor: '#f8fafc', fillOpacity: 0.12 }).addTo(
							referenceLayer
						);
					else L.polyline(pts, style).addTo(referenceLayer);
				} else if (f.layer === 'city_streets') {
					L.polyline(pts, { ...base, color: '#94a3b8', weight: 1.5, opacity: 0.85 }).addTo(
						referenceLayer
					);
				} else if (f.layer === 'boxes') {
					L.polygon(pts, { ...base, color: '#f59e0b', weight: 1.2, fill: false }).addTo(
						referenceLayer
					);
				} else if (f.layer === 'footprints') {
					L.polygon(pts, {
						...base,
						color: '#a78bfa',
						weight: 1,
						dashArray: '4 3',
						fill: false
					}).addTo(referenceLayer);
				} else {
					L.polyline(pts, { ...base, color: '#64748b', weight: 1 }).addTo(referenceLayer);
				}
			}
		}
	}

	function colourOf(m: Mark): string {
		if (m.kind === 'driveway') return DRIVEWAY_COLOUR;
		if (m.kind === 'parking_space') return SPACE_COLOURS[m.tags.space_type] ?? '#94a3b8';
		return LANE_COLOURS[m.tags.lane_type] ?? '#e5e7eb';
	}

	function chevron(at: EN, bearingDeg: number, size = 1.2): [number, number][] {
		const t = (bearingDeg * Math.PI) / 180;
		const back = (angle: number): EN => ({
			east: at.east - size * Math.sin(t + angle),
			north: at.north - size * Math.cos(t + angle)
		});
		return [back(0.5), at, back(-0.5)].map(latLngOf);
	}

	function drawMarks(
		marks: Mark[],
		selKey: string | null,
		problems: Map<string, string[]>,
		current: Tool
	) {
		if (!L || !marksLayer || !vertexLayer) return;
		const Lf = L;
		const layer = marksLayer;
		layer.clearLayers();
		vertexLayer.clearLayers();
		const interactive = current === 'select';
		for (const m of marks) {
			const colour = colourOf(m);
			const isSelected = m.key === selKey;
			const flagged = problems.get(m.id) ?? [];
			const pts = ring(m.points as [number, number][]);
			const en = m.points.map((p) => enOf(origin, p));
			const style = {
				color: colour,
				weight: isSelected ? 5 : 3,
				opacity: 0.95,
				interactive,
				bubblingMouseEvents: false
			};
			const shapes: Path[] = [];
			if (flagged.length > 0) {
				const halo = { color: '#ef4444', weight: isSelected ? 11 : 9, opacity: 0.55, interactive };
				shapes.push(
					m.kind === 'parking_space'
						? Lf.polygon(pts, { ...halo, fill: false })
						: Lf.polyline(pts, halo)
				);
			}
			if (m.kind === 'parking_space') {
				shapes.push(Lf.polygon(pts, { ...style, fillColor: colour, fillOpacity: 0.25 }));
				for (const [a, b] of rowDividers(en, Number(m.tags.count))) {
					shapes.push(
						Lf.polyline([latLngOf(a), latLngOf(b)], {
							color: colour,
							weight: 1.5,
							interactive: false
						})
					);
				}
			} else if (m.kind === 'driveway') {
				shapes.push(Lf.polyline(pts, { ...style, weight: isSelected ? 9 : 7, lineCap: 'butt' }));
			} else {
				shapes.push(Lf.polyline(pts, style));
				for (const a of laneArrows(en, 8)) {
					shapes.push(
						Lf.polyline(chevron(a.at, a.bearing), { color: colour, weight: 2, interactive: false })
					);
				}
			}
			for (const s of shapes) {
				if (interactive) {
					s.on('click', (e) => {
						Lf.DomEvent.stopPropagation(e);
						selectedKey = m.key;
					});
				}
				if (flagged.length > 0) s.bindTooltip(flagged.join('\n'), { sticky: true });
				s.addTo(layer);
			}
			if (isSelected && interactive) drawVertices(m);
		}
	}

	function drawVertices(m: Mark) {
		if (!L || !vertexLayer) return;
		const Lf = L;
		const layer = vertexLayer;
		const preview = Lf.circleMarker([0, 0], {
			radius: 7,
			color: '#22c55e',
			weight: 2,
			fill: false,
			interactive: false
		});
		m.points.forEach((p, index) => {
			const marker = Lf.marker([p[1], p[0]], {
				draggable: true,
				icon: Lf.divIcon({
					className: '',
					html: '<div style="width:12px;height:12px;border-radius:50%;background:#0b0f14;border:2px solid #fff;box-shadow:0 0 2px #000"></div>',
					iconSize: [12, 12],
					iconAnchor: [6, 6]
				}),
				title: 'Drag to move; right-click to remove this vertex'
			});
			marker.on('drag', () => {
				const at = marker.getLatLng();
				const s = snapAt(at.lat, at.lng, m.key);
				if (s.on === 'none') preview.remove();
				else preview.setLatLng(latLngOf(s.point)).addTo(layer);
			});
			marker.on('dragend', () => {
				preview.remove();
				const at = marker.getLatLng();
				const s = snapAt(at.lat, at.lng, m.key);
				edit = updateMark(edit, m.key, (mk) => moveVertex(mk, index, positionOf(origin, s.point)));
			});
			marker.on('contextmenu', (e) => {
				Lf.DomEvent.preventDefault(e.originalEvent);
				edit = updateMark(edit, m.key, (mk) => removeVertex(mk, index));
			});
			marker.addTo(layer);
		});
	}

	function drawDraft(
		placed: EN[],
		at: Snap | null,
		current: Tool,
		rowCount: number,
		depth: number,
		flip: boolean
	) {
		if (!L || !draftLayer) return;
		draftLayer.clearLayers();
		if (current === 'select') return;
		const pts = at ? [...placed, at.point] : placed;
		const line = { color: '#ffffff', weight: 2, dashArray: '5 4', interactive: false };
		if (current === 'row' && pts.length === 2) {
			const side = streetSide(pts[0], pts[1], centrelineTargets) * (flip ? -1 : 1);
			const row = rowOfSpaces(
				pts[0],
				pts[1],
				Math.max(1, Math.round(rowCount)),
				depth,
				side as 1 | -1
			);
			if (row) {
				L.polygon(row.ring.map(latLngOf), { ...line, fillOpacity: 0.1 }).addTo(draftLayer);
				for (const [a, b] of row.dividers) {
					L.polyline([latLngOf(a), latLngOf(b)], { ...line, weight: 1 }).addTo(draftLayer);
				}
			}
		} else if (pts.length >= 2) {
			const closing = current === 'space' && pts.length >= 3 ? [pts[0]] : [];
			L.polyline([...pts, ...closing].map(latLngOf), line).addTo(draftLayer);
		}
		for (const p of placed) {
			L.circleMarker(latLngOf(p), {
				radius: 3,
				color: '#ffffff',
				weight: 1,
				fillOpacity: 1,
				interactive: false
			}).addTo(draftLayer);
		}
		if (at) {
			const snapped = at.on !== 'none';
			L.circleMarker(latLngOf(at.point), {
				radius: snapped ? 7 : 4,
				color: snapped ? '#22c55e' : '#ffffff',
				weight: 2,
				fill: !snapped,
				fillOpacity: 0.6,
				interactive: false
			}).addTo(draftLayer);
		}
	}

	// ---- Save -----------------------------------------------------------------------

	async function save() {
		if (!siteToken || !dirty || saving) return;
		saving = true;
		saveError = null;
		savedMessage = null;
		const sent = edits.length;
		try {
			const result = await postStreetUse(address, siteToken, { author, note, edits });
			rememberAuthor(author);
			const { marks } = marksFromFeatures(result.features);
			edit = startEditing(marks);
			layerCounts = { ...layerCounts, street_use: marks.length };
			const mapped = mapProblems(
				result.problems,
				marks.map((m) => m.id)
			);
			problemsById = mapped.byId;
			generalProblems = mapped.general;
			selectedKey = null;
			note = '';
			const flagged = result.problems.length;
			savedMessage =
				`Saved ${sent} change${sent === 1 ? '' : 's'}; Gerardus holds ${marks.length} mark${marks.length === 1 ? '' : 's'} for this site` +
				(flagged ? `, with ${flagged} problem${flagged === 1 ? '' : 's'} to look at.` : '.');
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Could not save the street use.';
		} finally {
			saving = false;
		}
	}

	function tagSummary(m: Mark): string {
		if (m.kind === 'parking_space') {
			const n = Number(m.tags.count) || 1;
			return [m.tags.space_type, n > 1 ? `${n} spaces` : '1 space', m.tags.time_limit]
				.filter(Boolean)
				.join(' · ');
		}
		if (m.kind === 'travel_lane') {
			return [m.tags.lane_type, m.tags.width_m ? `${m.tags.width_m} m` : '']
				.filter(Boolean)
				.join(' · ');
		}
		return '';
	}
</script>

<svelte:head>
	<title>Streets — velocity.report</title>
</svelte:head>

<svelte:window on:keydown={onKey} on:beforeunload={onBeforeUnload} />

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<header class="flex items-start justify-between gap-4">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Street use</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Mark a site's driveways, parked-car spaces and travel lanes over the City's streets, the
					kerbs and the boxes Gerardus prepared. Gerardus keeps the marks and checks them.
				</p>
			</div>
			<a
				href={resolve('/lidar/clip-locations')}
				class="text-primary text-sm whitespace-nowrap hover:underline">Clip locations →</a
			>
		</header>
	</div>

	<div class="flex min-h-0 flex-1 gap-4 p-4">
		<aside class="w-80 flex-none space-y-4 overflow-y-auto text-sm">
			<section class="space-y-2">
				<div class="text-surface-content/70 font-medium">Site</div>
				{#if sites.length > 0}
					<select
						class="border-surface-content/20 bg-surface-50 w-full rounded border px-2 py-1 text-xs"
						bind:value={siteChoice}
						on:change={() => siteChoice && loadSite(siteChoice)}
					>
						<option value="">— a located site —</option>
						{#each sites as s (s.token)}
							<option value={s.token}>{siteLabel(s)}</option>
						{/each}
					</select>
				{:else if sitesError}
					<p class="text-surface-content/50 text-xs">{sitesError}</p>
				{/if}
				<form class="flex gap-2" on:submit|preventDefault={() => loadSite(tokenInput.trim())}>
					<input
						class="border-surface-content/20 bg-surface-50 min-w-0 flex-1 rounded border px-2 py-1 font-mono text-xs"
						placeholder="L16 token, such as 808580f3f"
						aria-label="L16 site token"
						bind:value={tokenInput}
					/>
					<Button size="sm" variant="outline" type="submit" disabled={loading}
						>{loading ? '…' : 'Load'}</Button
					>
				</form>
				{#if siteToken && !loadError && !loading}
					<div class="text-surface-content/60 space-y-0.5 text-xs">
						<div class="font-mono">
							{siteToken}{knownSite?.label ? ` · ${knownSite.label}` : ''}
						</div>
						<div>
							{#each Object.entries(LAYER_LABEL) as [layer, label], i (layer)}{i > 0
									? ' · '
									: ''}{label}
								{layerCounts[layer] ?? '—'}{/each}
						</div>
						{#if !layerCounts.kerbs}
							<div class="text-surface-content/40">
								No kerbs in this bundle yet: driveways snap to the box walls' feet.
							</div>
						{/if}
						{#if skipped > 0}
							<div class="text-amber-600">
								{skipped} feature{skipped === 1 ? '' : 's'} could not be read and are not drawn.
							</div>
						{/if}
						<button class="text-surface-content/50 underline" on:click={reload}>Load again</button>
					</div>
				{/if}
			</section>

			{#if loadError}
				<section class="space-y-1 rounded bg-red-50 px-3 py-2 text-xs text-red-700" role="alert">
					<div class="font-medium">
						{#if loadError instanceof GerardusError && loadError.kind === 'unreachable'}
							Gerardus is unreachable
						{:else if loadError instanceof GerardusError && loadError.kind === 'not_prepared'}
							This site is not prepared
						{:else if loadError instanceof GerardusError && loadError.kind === 'unsupported'}
							This Gerardus has no street editor
						{:else}
							Could not load the site
						{/if}
					</div>
					<p>{loadError.message}</p>
				</section>
			{/if}

			<section class="space-y-1">
				<label class="text-surface-content/60 block text-xs">
					Gerardus address
					<input
						class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-2 py-1 font-mono text-xs"
						bind:value={addressInput}
						on:change={applyAddress}
					/>
				</label>
				<p class="text-surface-content/40 text-xs">
					This browser talks to Gerardus directly. The address is saved in this browser, as on the
					<a href={resolve('/settings')} class="underline">Settings</a> page.
				</p>
			</section>

			{#if siteToken && !loadError}
				<section class="space-y-2">
					<div class="text-surface-content/70 font-medium">Tools</div>
					<div class="grid grid-cols-2 gap-1">
						{#each TOOLS as t (t.id)}
							<Button
								size="sm"
								variant={tool === t.id ? 'fill' : 'outline'}
								color={tool === t.id ? 'primary' : 'default'}
								on:click={() => setTool(t.id)}>{t.label}</Button
							>
						{/each}
					</div>
					<p class="text-surface-content/50 text-xs">{activeTool.hint}</p>

					{#if tool === 'space' || tool === 'row'}
						<div class="grid grid-cols-2 gap-2 text-xs">
							<label class="text-surface-content/60">
								space_type
								<select
									class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
									bind:value={spaceType}
								>
									{#each SPACE_TYPES as s (s)}<option value={s}>{s}</option>{/each}
								</select>
							</label>
							<label class="text-surface-content/60">
								count
								<input
									type="number"
									min="1"
									step="1"
									class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
									bind:value={count}
								/>
							</label>
							<label class="text-surface-content/60">
								time_limit
								<input
									class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
									placeholder="e.g. 2h"
									bind:value={timeLimit}
								/>
							</label>
							{#if tool === 'row'}
								<label class="text-surface-content/60">
									depth (m)
									<input
										type="number"
										min="0.5"
										step="0.1"
										class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
										bind:value={rowDepth}
									/>
								</label>
							{/if}
						</div>
						{#if tool === 'row'}
							<label class="text-surface-content/60 flex items-center gap-2 text-xs">
								<input type="checkbox" bind:checked={rowFlip} />
								Put the row on the other side of the kerb
							</label>
						{/if}
					{:else if tool === 'lane'}
						<div class="grid grid-cols-2 gap-2 text-xs">
							<label class="text-surface-content/60">
								lane_type
								<select
									class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
									bind:value={laneType}
								>
									{#each LANE_TYPES as s (s)}<option value={s}>{s}</option>{/each}
								</select>
							</label>
							<label class="text-surface-content/60">
								width_m
								<input
									type="number"
									min="0.5"
									step="0.1"
									class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
									bind:value={widthM}
								/>
							</label>
						</div>
					{/if}

					{#if drawing.length > 0}
						<div class="flex flex-wrap gap-2">
							{#if tool === 'space' || tool === 'lane'}
								<Button size="sm" variant="outline" on:click={finishDrawing}>Finish</Button>
							{/if}
							<Button size="sm" variant="outline" on:click={cancelDrawing}>Cancel</Button>
						</div>
					{/if}
					{#if cursor}
						<p class="text-surface-content/50 text-xs">
							{cursor.on === 'none'
								? 'Not snapped: nothing within 0.5 m.'
								: `Snapped to a ${cursor.on === 'vertex' ? 'vertex' : 'line'} of ${LAYER_LABEL[cursor.target?.layer ?? ''] ?? 'a mark'}.`}
						</p>
					{/if}
					<div class="flex flex-wrap gap-2">
						<Button
							size="sm"
							variant="outline"
							disabled={!canUndo(edit) && drawing.length === 0}
							on:click={doUndo}>Undo</Button
						>
					</div>
					<p class="text-surface-content/40 text-xs">
						Esc cancels, Enter finishes, Backspace drops the last vertex, Delete removes the
						selected mark, Ctrl or ⌘ Z undoes. Gerardus snaps and drapes every vertex again on Save.
					</p>
				</section>

				{#if selected}
					<section class="border-surface-content/10 space-y-2 rounded border p-2">
						<div class="flex items-center justify-between">
							<div class="font-medium">{KIND_LABEL[selected.kind]}</div>
							<div class="text-surface-content/50 font-mono text-xs">
								{selected.id || 'new'}
							</div>
						</div>
						{#if selected.author}
							<div class="text-surface-content/50 text-xs">
								{selected.author}{selected.editedAt ? `, ${selected.editedAt}` : ''}
							</div>
						{/if}
						{#if selected.kind === 'parking_space'}
							<div class="grid grid-cols-2 gap-2 text-xs">
								<label class="text-surface-content/60">
									space_type
									<select
										class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
										value={selected.tags.space_type ?? ''}
										on:change={(e) =>
											changeSelected((m) => setTag(m, 'space_type', e.currentTarget.value))}
									>
										<option value="">—</option>
										{#each SPACE_TYPES as s (s)}<option value={s}>{s}</option>{/each}
									</select>
								</label>
								<label class="text-surface-content/60">
									count
									<input
										type="number"
										min="1"
										step="1"
										class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
										value={selected.tags.count ?? ''}
										on:change={(e) =>
											changeSelected((m) => setTag(m, 'count', e.currentTarget.value))}
									/>
								</label>
								<label class="text-surface-content/60 col-span-2">
									time_limit
									<input
										class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
										value={selected.tags.time_limit ?? ''}
										on:change={(e) =>
											changeSelected((m) => setTag(m, 'time_limit', e.currentTarget.value))}
									/>
								</label>
							</div>
						{:else if selected.kind === 'travel_lane'}
							<div class="grid grid-cols-2 gap-2 text-xs">
								<label class="text-surface-content/60">
									lane_type
									<select
										class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
										value={selected.tags.lane_type ?? ''}
										on:change={(e) =>
											changeSelected((m) => setTag(m, 'lane_type', e.currentTarget.value))}
									>
										<option value="">—</option>
										{#each LANE_TYPES as s (s)}<option value={s}>{s}</option>{/each}
									</select>
								</label>
								<label class="text-surface-content/60">
									width_m
									<input
										type="number"
										min="0.5"
										step="0.1"
										class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-1 py-1"
										value={selected.tags.width_m ?? ''}
										on:change={(e) =>
											changeSelected((m) => setTag(m, 'width_m', e.currentTarget.value))}
									/>
								</label>
							</div>
						{/if}
						{#if (problemsById.get(selected.id) ?? []).length > 0}
							<ul class="list-disc pl-4 text-xs text-red-600">
								{#each problemsById.get(selected.id) ?? [] as p, i (i)}<li>{p}</li>{/each}
							</ul>
						{/if}
						<div class="flex flex-wrap gap-2">
							{#if selected.kind === 'travel_lane'}
								<Button size="sm" variant="outline" on:click={() => changeSelected(reverseMark)}
									>Reverse direction</Button
								>
							{/if}
							<Button size="sm" variant="outline" color="danger" on:click={deleteSelected}
								>Delete</Button
							>
						</div>
						{#if tool !== 'select'}
							<p class="text-surface-content/40 text-xs">Choose Select to move its vertices.</p>
						{:else}
							<p class="text-surface-content/40 text-xs">
								Drag a vertex to move it; right-click one to remove it.
							</p>
						{/if}
					</section>
				{/if}

				<section class="space-y-1">
					<div class="text-surface-content/70 font-medium">
						Marks ({edit.marks.length})
					</div>
					{#if edit.marks.length === 0}
						<p class="text-surface-content/50 text-xs">None yet for this site.</p>
					{:else}
						<ul class="border-surface-content/10 divide-y rounded border text-xs">
							{#each edit.marks as m (m.key)}
								<li>
									<button
										class="hover:bg-surface-200 w-full px-2 py-1 text-left {m.key === selectedKey
											? 'bg-surface-200'
											: ''}"
										on:click={() => {
											selectedKey = m.key;
											if (tool !== 'select') setTool('select');
										}}
									>
										<span
											class="mr-1 inline-block h-2 w-2 rounded-full"
											style="background:{colourOf(m)}"
										></span>
										{KIND_LABEL[m.kind]}
										<span class="text-surface-content/50">{tagSummary(m)}</span>
										{#if !m.id}<span class="text-surface-content/40"> · new</span>{/if}
										{#each problemsById.get(m.id) ?? [] as p, i (i)}
											<div class="text-red-600">{p}</div>
										{/each}
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				</section>

				<section class="space-y-2">
					<div class="text-surface-content/70 font-medium">Save</div>
					<label class="text-surface-content/60 block text-xs">
						Author (required)
						<input
							class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-2 py-1 text-xs"
							placeholder="your name"
							bind:value={author}
						/>
					</label>
					<label class="text-surface-content/60 block text-xs">
						Note
						<input
							class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-2 py-1 text-xs"
							placeholder="what was marked, and from what"
							bind:value={note}
						/>
					</label>
					<Button
						variant="fill"
						color="primary"
						disabled={saving || !dirty || author.trim() === ''}
						on:click={save}
						>{saving
							? 'Saving…'
							: dirty
								? `Save ${edits.length} change${edits.length === 1 ? '' : 's'}`
								: 'No changes to save'}</Button
					>
					{#if dirty && author.trim() === ''}
						<p class="text-xs text-amber-600">Say who is making the edits to save them.</p>
					{/if}
					{#if saveError}
						<p class="rounded bg-red-50 px-3 py-2 text-xs text-red-600" role="alert">{saveError}</p>
					{/if}
					{#if savedMessage}
						<p class="rounded bg-emerald-50 px-3 py-2 text-xs text-emerald-700">{savedMessage}</p>
					{/if}
					{#if generalProblems.length > 0}
						<ul class="list-disc rounded bg-amber-50 py-2 pr-3 pl-6 text-xs text-amber-700">
							{#each generalProblems as p, i (i)}<li>{p}</li>{/each}
						</ul>
					{/if}
				</section>
			{/if}

			<section class="space-y-1">
				<div class="text-surface-content/70 font-medium">Backdrop</div>
				<Button size="sm" variant="outline" on:click={toggleTiles}
					>{showTiles ? 'Hide the OpenStreetMap map' : 'Show the OpenStreetMap map'}</Button
				>
				<p class="text-surface-content/40 text-xs">
					Optional, and only for finding your way: nothing is snapped to or taken from it.
				</p>
			</section>

			<section class="text-surface-content/60 space-y-0.5 text-xs">
				<div class="text-surface-content/70 font-medium">Key</div>
				<div><span style="color:#94a3b8">━</span> city street centreline</div>
				<div><span style="color:#f8fafc">━━</span> kerb or island</div>
				<div>
					<span style="color:#f59e0b">▭</span> box · <span style="color:#a78bfa">▭</span> footprint
				</div>
				<div><span style="color:{DRIVEWAY_COLOUR}">━━</span> driveway</div>
				<div>
					spaces:
					{#each SPACE_TYPES as s, i (s)}{i > 0 ? ' · ' : ''}<span style="color:{SPACE_COLOURS[s]}"
							>{s}</span
						>{/each}
				</div>
				<div>
					lanes:
					{#each LANE_TYPES as s, i (s)}{i > 0 ? ' · ' : ''}<span style="color:{LANE_COLOURS[s]}"
							>{s}</span
						>{/each}
				</div>
				<div><span class="text-red-500">▬</span> a problem Gerardus reported</div>
			</section>
		</aside>

		<div class="relative min-h-[480px] flex-1 overflow-hidden rounded border">
			<div bind:this={mapContainer} class="street-map absolute inset-0"></div>
			{#if !siteToken && !loading}
				<div
					class="pointer-events-none absolute inset-0 z-[450] flex items-center justify-center p-6 text-center text-sm text-neutral-300"
				>
					Choose a located site, or type its L16 token, to load Gerardus's layers for it.
				</div>
			{/if}
		</div>
	</div>
</main>

{#if askingOsm}
	<div
		class="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70 p-4"
		role="presentation"
	>
		<div
			class="w-full max-w-md rounded border border-neutral-700 bg-neutral-950 p-6 text-white shadow-2xl"
			role="alertdialog"
			aria-modal="true"
			aria-labelledby="osm-tiles-title"
		>
			<div class="mb-4 flex items-center justify-between gap-3">
				<h3 id="osm-tiles-title" class="text-lg font-semibold">Allow OpenStreetMap Requests?</h3>
				<button
					class="-mt-1 -mr-2 p-1 text-neutral-300 hover:text-white"
					on:click={() => (askingOsm = false)}
					aria-label="Close"
				>
					<svg class="h-5 w-5" viewBox="0 0 24 24"><path fill="currentColor" d={mdiClose} /></svg>
				</button>
			</div>
			<div class="space-y-3 text-sm text-neutral-100">
				<p>
					This page will request OpenStreetMap raster tiles to draw a backdrop behind the site's
					layers.
				</p>
				<p>The map area may be sent to the tile servers.</p>
				<p>
					LiDAR points, street marks, vehicle data, reports and raw sensor data are not sent.
					Nothing is snapped to the tiles or taken from them.
				</p>
				<p>The choice lasts for this visit to the page.</p>
				<p class="text-neutral-300">
					To work without the network, cancel: the layers are drawn on a plain dark ground.
				</p>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<Button on:click={() => (askingOsm = false)} variant="outline">Cancel</Button>
				<Button on:click={allowOsm} variant="fill" color="primary">Allow Tiles This Session</Button>
			</div>
		</div>
	</div>
{/if}

<style>
	:global(.street-map.leaflet-container) {
		background: #0b0f14;
		font-family: inherit;
	}
</style>
