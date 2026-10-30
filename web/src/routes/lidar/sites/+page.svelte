<script lang="ts">
	/**
	 * Site viewer — what Gerardus serves for a site, shown and not edited
	 * (Gerardus docs/plans/VECTOR-MAP.md §4.1). Street use is marked in the
	 * macOS app; this page only reads.
	 *
	 * The 2D map draws the site's GeoJSON layers in WGS 84 over a plain dark
	 * ground: street use by kind and surfaces by what they are, the City's
	 * streets and intersections, kerbs, boxes, footprints, wall planes and what
	 * the scans observed, and, where the site lists them, Gerardus's W4–W7
	 * layers: building parts by roof:shape, roof planes by slope, lane markings,
	 * poles and signs, and trees by their crowns. A click shows a feature's
	 * properties. The 3D view asks for the same layers in the site frame and
	 * draws them at their heights (box walls and building walls up to the
	 * roofline as Gerardus sends them, where it does; an older Gerardus's boxes
	 * and parts raised by their heights; poles as cylinders, trees as a trunk
	 * and a crown), with the ground grid and, where Gerardus serves them to this
	 * browser, the scan points.
	 *
	 * The browser talks to Gerardus directly (lib/gerardus), at the address in
	 * Settings. Sites come from Gerardus's own list, or, from a Gerardus older
	 * than that list, from velocity.report's located sites.
	 *
	 * OpenStreetMap is behind a consent asked once per visit, as the site map
	 * editor asks: no tile and no OpenStreetMap layer is requested before it.
	 */
	import { getClipLocations } from '#lib/api.js';
	import SiteScene3D from '#lib/components/lidar/SiteScene3D.svelte';
	import {
		GerardusError,
		gerardusAddress,
		getSiteFrameGeojson,
		getSiteGeojson,
		getSiteGround,
		getSiteScans,
		groupByLayer,
		isL16Token,
		listSites,
		OBSERVED_KINDS,
		rememberGerardusAddress,
		SCAN_VOXELS_M,
		servedLayers,
		SURFACE_TYPES,
		viewLayers,
		WALL_ROLES,
		type GroundGrid,
		type ScanCloud,
		type SiteFeature,
		type SiteFrameFeature
	} from '#lib/gerardus/client.js';
	import {
		boundsOf,
		enOf,
		isUpright,
		laneArrows,
		partsOf,
		polygonsOf,
		positionOf
	} from '#lib/gerardus/geometry.js';
	import {
		crownRadius,
		FURNITURE_COLOURS,
		LANE_COLOURS,
		LAYER_COLOURS,
		layerLabel,
		mapStyle,
		MARKING_COLOURS,
		OBSERVED_COLOURS,
		propertyRows,
		roofSlopeColour,
		ROOF_SHAPE_COLOURS,
		ROOF_SHAPE_UNKNOWN,
		SPACE_COLOURS,
		STREET_USE_COLOURS,
		SURFACE_COLOURS,
		TREE_COLOURS,
		WALL_ROLE_COLOURS
	} from '#lib/gerardus/style.js';
	import { mdiClose } from '@mdi/js';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import 'leaflet/dist/leaflet.css';
	import type {
		LatLngExpression,
		LayerGroup,
		Map as LeafletMap,
		Path,
		Renderer,
		TileLayer
	} from 'leaflet';
	import { onDestroy, onMount, tick } from 'svelte';
	import { Button } from 'svelte-ux';

	interface ViewerSite {
		token: string;
		label: string;
		lat?: number;
		lon?: number;
		/** The layers Gerardus says it serves; empty when the list came from velocity.report. */
		layers: string[];
		/** Where the list came from: Gerardus, or velocity.report's located sites. */
		source: 'gerardus' | 'located';
		clips?: number;
	}

	type Tab = '2d' | '3d';
	type OsmRequest = { kind: 'tiles' };

	const SF = { lat: 37.7749, lon: -122.4194 };

	// ---- Gerardus and the site ------------------------------------------------

	let address = gerardusAddress();
	let addressInput = address;
	let sites: ViewerSite[] = [];
	let sitesNote: string | null = null;
	// The epoch Gerardus gives its sites' origins at, when it says.
	let sitesEpoch: number | null = null;
	let siteChoice = '';
	let tokenInput = page.url.searchParams.get('site') ?? '';
	let siteToken = '';
	let tab: Tab = page.url.searchParams.get('view') === '3d' ? '3d' : '2d';
	let shown3d = tab === '3d';

	// The 2D map's features, WGS 84.
	let loading = false;
	let loadError: Error | null = null;
	let loadAbort: AbortController | null = null;
	let features: SiteFeature[] = [];
	let skipped = 0;
	let origin = SF;
	let selected: SiteFeature | null = null;

	// The 3D view's, in the site frame.
	let loading3d = false;
	let error3d: Error | null = null;
	let abort3d: AbortController | null = null;
	let loaded3dFor: string | null = null;
	let frameFeatures: SiteFrameFeature[] = [];
	let skipped3d = 0;
	let ground: GroundGrid | null = null;
	let groundError: string | null = null;
	let voxelM: number = 0.2;
	let scans: ScanCloud | null = null;
	let scansLoading = false;
	let scansNote: string | null = null;
	let scansAbort: AbortController | null = null;

	// Which layers are shown, by name, with `ground`, `scans` and `tiles`. Absent means shown.
	let visible: Record<string, boolean> = {};

	// ---- OpenStreetMap consent, for this visit only, as in the site map editor --

	let osmAllowed = false;
	let askingOsm: OsmRequest | null = null;

	// ---- The map ----------------------------------------------------------------

	let L: typeof import('leaflet') | null = null;
	let map: LeafletMap | null = null;
	let mapContainer: HTMLDivElement;
	let canvasRenderer: Renderer | null = null;
	let featureLayer: LayerGroup | null = null;
	let tileLayer: TileLayer | null = null;
	let scene3d: SiteScene3D | null = null;

	$: knownSite = siteOf(siteToken, sites);
	$: gerardusLayers = viewLayers(siteLayers(knownSite), tab);
	$: counts2d = countsOf(features);
	$: counts3d = countsOf(frameFeatures);
	$: counts = tab === '3d' ? counts3d : counts2d;

	$: if (map && L) drawFeatures(features, visible, selected);
	$: if (map && L) syncTiles(osmAllowed && visible.tiles === true);
	$: if (tab === '3d') shown3d = true;
	$: if (tab === '3d' && siteToken && !loading && !loadError && loaded3dFor !== siteToken)
		void load3d();
	$: if (tab === '2d' && map) void tick().then(() => map?.invalidateSize());

	onMount(async () => {
		L = await import('leaflet');
		await tick();
		initMap();
		await loadSites();
		if (tokenInput) void loadSite(tokenInput.trim());
	});

	onDestroy(() => {
		loadAbort?.abort();
		abort3d?.abort();
		scansAbort?.abort();
		map?.remove();
	});

	function siteOf(token: string, list: ViewerSite[]): ViewerSite | null {
		return list.find((s) => s.token === token) ?? null;
	}

	/**
	 * The layers a site serves: Gerardus's word for it, else every layer the
	 * viewer knows but the W4–W7 ones, which are asked for only when listed.
	 */
	function siteLayers(site: ViewerSite | null): string[] {
		return servedLayers(site?.source === 'gerardus' ? site.layers : []);
	}

	/** Whether an OpenStreetMap layer is wanted: only ever after this visit's consent. */

	function countsOf(fs: { layer: string }[]): Record<string, number> {
		const out: Record<string, number> = {};
		for (const [layer, list] of groupByLayer(fs)) out[layer] = list.length;
		return out;
	}

	function initMap() {
		if (!L || !mapContainer) return;
		map = L.map(mapContainer, {
			center: [origin.lat, origin.lon],
			zoom: 13,
			maxZoom: 23,
			zoomSnap: 0.5
		});
		// A few pixels of slack so thin lines can be clicked.
		canvasRenderer = L.canvas({ padding: 0.5, tolerance: 5 });
		featureLayer = L.layerGroup().addTo(map);
		map.on('click', () => (selected = null));
	}

	// ---- Loading ----------------------------------------------------------------------

	async function loadSites() {
		sitesNote = null;
		sitesEpoch = null;
		try {
			const r = await listSites(address);
			sitesEpoch = r.epoch ?? null;
			sites = r.sites
				.map((s): ViewerSite => ({
					token: s.l16,
					label: s.label,
					lat: s.lat,
					lon: s.lon,
					layers: s.layers,
					source: 'gerardus'
				}))
				.sort((a, b) => (a.label || a.token).localeCompare(b.label || b.token));
			if (r.skipped > 0) {
				sitesNote = `${r.skipped} site${r.skipped === 1 ? '' : 's'} in Gerardus's list could not be read.`;
			}
		} catch (e) {
			// A Gerardus older than the site list: offer the sites velocity.report has located.
			const why = e instanceof Error ? e.message : 'Gerardus did not list its sites.';
			try {
				const r = await getClipLocations();
				sites = r.districts
					.flatMap((d) => d.sites)
					.map((s): ViewerSite => ({
						token: s.s2_l16_token,
						label: s.label || s.clips[0]?.description || '',
						lat: s.canonical_lat ?? s.clips[0]?.origin_lat,
						lon: s.canonical_lon ?? s.clips[0]?.origin_lon,
						layers: [],
						source: 'located',
						clips: s.clips.length
					}))
					.sort((a, b) => (a.label || a.token).localeCompare(b.label || b.token));
				sitesNote = `${why} Listing the sites velocity.report has located instead.`;
			} catch {
				// The list is a convenience: a token can still be typed.
				sites = [];
				sitesNote = `${why} velocity.report's located sites could not be listed either; type an L16 token.`;
			}
		}
		siteChoice = sites.some((s) => s.token === siteToken) ? siteToken : '';
	}

	function siteLabel(s: ViewerSite): string {
		const extra =
			s.source === 'located' && s.clips !== undefined
				? ` · ${s.clips} clip${s.clips === 1 ? '' : 's'}`
				: '';
		return s.label ? `${s.label} · ${s.token}${extra}` : `${s.token}${extra}`;
	}

	async function loadSite(token: string) {
		if (!token) return;
		if (!isL16Token(token)) {
			loadError = new Error(`“${token}” is not an L16 site token (hex digits, such as 808580f3f).`);
			return;
		}
		loadAbort?.abort();
		abort3d?.abort();
		scansAbort?.abort();
		const abort = new AbortController();
		loadAbort = abort;
		siteToken = token;
		tokenInput = token;
		siteChoice = sites.some((s) => s.token === token) ? token : '';
		loading = true;
		loadError = null;
		selected = null;
		// The 3D view's data belongs to the old site: drop it, so it is asked for again.
		loaded3dFor = null;
		frameFeatures = [];
		ground = null;
		scans = null;
		scansNote = null;
		error3d = null;
		groundError = null;
		const site = siteOf(token, sites);
		const served = siteLayers(site);
		try {
			const { features: fs, skipped: bad } = await getGeojsonOrOlder(
				token,
				viewLayers(served, '2d'),
				site?.source === 'gerardus',
				abort.signal
			);
			if (abort.signal.aborted) return;
			features = fs;
			skipped = bad;
			setOrigin(token, fs);
			fitSite(fs);
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') return;
			loadError = e instanceof Error ? e : new Error('Could not load the site.');
			features = [];
			skipped = 0;
		} finally {
			if (loadAbort === abort) loading = false;
		}
	}

	/**
	 * The 2D layers. Gerardus refuses a layer name it does not know (400), so a
	 * Gerardus that did not list its own layers, and is older than the observed
	 * and wall-plane layers, is asked again without them.
	 */
	async function getGeojsonOrOlder(
		token: string,
		layers: string[],
		listed: boolean,
		signal: AbortSignal
	) {
		try {
			return await getSiteGeojson(address, token, layers, { signal });
		} catch (e) {
			const newer = ['observed', 'wall_planes'];
			if (listed || !(e instanceof GerardusError) || e.kind !== 'rejected') throw e;
			if (!layers.some((l) => newer.includes(l))) throw e;
			return getSiteGeojson(
				address,
				token,
				layers.filter((l) => !newer.includes(l)),
				{ signal }
			);
		}
	}

	/** The site frame's origin when Gerardus or velocity.report knows it, else the middle of the features. */
	function setOrigin(token: string, fs: SiteFeature[]) {
		const site = siteOf(token, sites);
		if (site?.lat != null && site.lon != null) {
			origin = { lat: site.lat, lon: site.lon };
			return;
		}
		const b = boundsOf(fs);
		if (b) origin = { lat: (b.south + b.north) / 2, lon: (b.west + b.east) / 2 };
	}

	function fitSite(fs: SiteFeature[]) {
		if (!map) return;
		const b = boundsOf(fs);
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

	async function load3d() {
		const token = siteToken;
		abort3d?.abort();
		const abort = new AbortController();
		abort3d = abort;
		loaded3dFor = token;
		loading3d = true;
		error3d = null;
		groundError = null;
		const layers = viewLayers(siteLayers(siteOf(token, sites)), '3d');
		const [geo, grid] = await Promise.allSettled([
			getSiteFrameGeojson(address, token, layers, {
				signal: abort.signal,
				origin: { lon: origin.lon, lat: origin.lat }
			}),
			getSiteGround(address, token, { signal: abort.signal })
		]);
		if (abort.signal.aborted || token !== siteToken) return;
		if (geo.status === 'fulfilled') {
			frameFeatures = geo.value.features;
			skipped3d = geo.value.skipped;
		} else {
			frameFeatures = [];
			skipped3d = 0;
			error3d = geo.reason instanceof Error ? geo.reason : new Error('Could not load the site.');
		}
		if (grid.status === 'fulfilled') {
			ground = grid.value;
		} else {
			ground = null;
			groundError =
				grid.reason instanceof Error ? grid.reason.message : 'Could not load the ground.';
		}
		loading3d = false;
		if (!error3d) void loadScans();
	}

	async function loadScans() {
		const token = siteToken;
		if (!token) return;
		scansAbort?.abort();
		const abort = new AbortController();
		scansAbort = abort;
		scansLoading = true;
		scansNote = null;
		try {
			const cloud = await getSiteScans(address, token, voxelM, { signal: abort.signal });
			if (abort.signal.aborted || token !== siteToken) return;
			scans = cloud;
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') return;
			scans = null;
			scansNote =
				e instanceof GerardusError && e.kind === 'forbidden'
					? 'Gerardus does not serve scan points to this browser, so none are drawn. It serves them only to the peers it is set to.'
					: e instanceof Error
						? e.message
						: 'Could not load the scan points.';
		} finally {
			if (scansAbort === abort) scansLoading = false;
		}
	}

	async function applyAddress() {
		address = rememberGerardusAddress(addressInput);
		addressInput = address;
		await loadSites();
		if (siteToken) void loadSite(siteToken);
	}

	function reload() {
		if (siteToken) void loadSite(siteToken);
	}

	// ---- Layers and OpenStreetMap -------------------------------------------------------

	function isShown(layer: string, shownMap: Record<string, boolean>): boolean {
		if (layer === 'tiles') return osmAllowed && shownMap[layer] === true;
		return shownMap[layer] !== false;
	}

	function toggle(layer: string) {
		const on = !isShown(layer, visible);
		if (on && layer === 'tiles' && !osmAllowed) {
			askingOsm = { kind: 'tiles' };
			return;
		}
		visible = { ...visible, [layer]: on };
	}

	function allowOsm() {
		const request = askingOsm;
		osmAllowed = true;
		askingOsm = null;
		if (!request) return;
		const layer = 'tiles';
		visible = { ...visible, [layer]: true };
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

	// ---- Drawing the 2D map ---------------------------------------------------------------

	function ring(points: readonly (readonly number[])[]): LatLngExpression[] {
		return points.map((p) => [p[1], p[0]] as [number, number]);
	}

	function chevron(at: { east: number; north: number }, bearingDeg: number, size = 1.2) {
		const t = (bearingDeg * Math.PI) / 180;
		const back = (angle: number) => ({
			east: at.east - size * Math.sin(t + angle),
			north: at.north - size * Math.cos(t + angle)
		});
		return [back(0.5), at, back(-0.5)].map((p) => {
			const [lon, lat] = positionOf(origin, p);
			return [lat, lon] as [number, number];
		});
	}

	function drawFeatures(
		fs: SiteFeature[],
		shownMap: Record<string, boolean>,
		sel: SiteFeature | null
	) {
		if (!L || !featureLayer) return;
		const Lf = L;
		const layer = featureLayer;
		layer.clearLayers();
		const drawn = fs
			.filter((f) => isShown(f.layer, shownMap))
			.map((f) => ({ f, style: mapStyle(f) }))
			.sort((a, b) => a.style.rank - b.style.rank);
		for (const { f, style } of drawn) {
			const base = {
				renderer: canvasRenderer ?? undefined,
				color: style.colour,
				weight: style.weight,
				opacity: style.opacity,
				dashArray: style.dashArray,
				bubblingMouseEvents: false
			};
			const shapes: Path[] = [];
			const polygons = polygonsOf(f.geometry);
			if (polygons.length > 0) {
				for (const { outer, holes } of polygons) {
					shapes.push(
						Lf.polygon([ring(outer), ...holes.map(ring)] as LatLngExpression[][], {
							...base,
							fill: style.fillOpacity > 0,
							fillColor: style.colour,
							fillOpacity: style.fillOpacity
						})
					);
				}
			} else {
				for (const part of partsOf(f.geometry)) {
					const pts = ring(part.points);
					// A tree is its crown, seen from above, as wide as it is.
					if (f.kind === 'tree') {
						shapes.push(
							Lf.circle(pts[0], {
								...base,
								radius: crownRadius(f.tags),
								fillColor: style.colour,
								fillOpacity: style.fillOpacity
							})
						);
						continue;
					}
					// A pole or a sign stands up: seen from above, a point.
					if (part.points.length === 1 || isUpright(part.points)) {
						shapes.push(
							Lf.circleMarker(pts[0], {
								...base,
								radius: style.radius,
								fillColor: style.colour,
								fillOpacity: 0.3
							})
						);
					} else {
						shapes.push(Lf.polyline(pts, base));
						if (style.arrows) {
							const en = part.points.map((p) => enOf(origin, p));
							for (const a of laneArrows(en, 8)) {
								Lf.polyline(chevron(a.at, a.bearing), {
									renderer: canvasRenderer ?? undefined,
									color: style.colour,
									weight: 2,
									interactive: false
								}).addTo(layer);
							}
						}
					}
				}
			}
			for (const s of shapes) {
				s.on('click', (e) => {
					Lf.DomEvent.stopPropagation(e);
					selected = f;
				});
				s.addTo(layer);
			}
		}
		if (sel && isShown(sel.layer, shownMap)) {
			const halo = {
				renderer: canvasRenderer ?? undefined,
				color: '#fde047',
				weight: 4,
				opacity: 0.9,
				fill: false,
				interactive: false
			};
			for (const part of partsOf(sel.geometry)) {
				const pts = ring(part.points);
				if (part.points.length === 1 || isUpright(part.points))
					Lf.circleMarker(pts[0], { ...halo, radius: 9 }).addTo(layer);
				else if (part.closed) Lf.polygon(pts, halo).addTo(layer);
				else Lf.polyline(pts, halo).addTo(layer);
			}
		}
	}

	function errorTitle(e: Error): string {
		if (!(e instanceof GerardusError)) return 'Could not load the site';
		switch (e.kind) {
			case 'unreachable':
				return 'Gerardus is unreachable';
			case 'not_prepared':
				return 'This site is not prepared';
			case 'unsupported':
				return 'This Gerardus does not serve this view';
			case 'forbidden':
				return 'Gerardus does not serve this to this browser';
			case 'server':
				return 'Gerardus failed';
			case 'malformed':
				return 'Gerardus answered with something unexpected';
			default:
				return 'Could not load the site';
		}
	}

	function setTab(next: Tab) {
		tab = next;
	}
</script>

<svelte:head>
	<title>Site viewer — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<header class="flex items-start justify-between gap-4">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Site viewer</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					What Gerardus serves for a site: its vector map on a 2D map, and the same layers with the
					ground and scans in 3D. Street use is marked in the macOS app, not here.
				</p>
			</div>
			<a
				href={resolve('/lidar/clip-locations')}
				class="text-primary text-sm whitespace-nowrap hover:underline">Clip locations →</a
			>
		</header>
		<div class="mt-3 flex gap-2" role="tablist" aria-label="View">
			{#each [{ id: '2d', label: '2D map' }, { id: '3d', label: '3D view' }] as t (t.id)}
				<button
					type="button"
					role="tab"
					aria-selected={tab === t.id}
					aria-controls="site-view-{t.id}"
					class="rounded px-3 py-1 text-sm {tab === t.id
						? 'bg-primary text-primary-content'
						: 'bg-surface-200 text-surface-content/70'}"
					on:click={() => setTab(t.id as Tab)}>{t.label}</button
				>
			{/each}
		</div>
	</div>

	<div class="flex min-h-0 flex-1 gap-4 p-4">
		<aside class="w-80 flex-none space-y-4 overflow-y-auto text-sm">
			<section class="space-y-2">
				<div class="text-surface-content/70 font-medium">Site</div>
				{#if sites.length > 0}
					<select
						class="border-surface-content/20 bg-surface-50 w-full rounded border px-2 py-1 text-xs"
						aria-label="Site"
						bind:value={siteChoice}
						on:change={() => siteChoice && loadSite(siteChoice)}
					>
						<option value=""
							>{sites[0].source === 'gerardus'
								? '— a prepared site —'
								: '— a located site —'}</option
						>
						{#each sites as s (s.token)}
							<option value={s.token}>{siteLabel(s)}</option>
						{/each}
					</select>
				{/if}
				{#if sitesNote}
					<p class="text-surface-content/50 text-xs">{sitesNote}</p>
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
						{#if knownSite?.source === 'gerardus'}
							<div>
								Site frame origin {knownSite.lat?.toFixed(7)}, {knownSite.lon?.toFixed(
									7
								)}{sitesEpoch !== null ? ` (WGS 84 at epoch ${sitesEpoch})` : ''}
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
					<div class="font-medium">{errorTitle(loadError)}</div>
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
				<section class="space-y-1">
					<div class="text-surface-content/70 font-medium">Layers</div>
					<ul class="space-y-0.5 text-xs">
						{#if tab === '3d'}
							<li>
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										checked={isShown('ground', visible)}
										on:change={() => toggle('ground')}
									/>
									<span class="inline-block h-2 w-2 rounded-sm" style="background:#3b4656"></span>
									ground grid
									<span class="text-surface-content/40 ml-auto"
										>{ground ? `${ground.nx} × ${ground.ny} at ${ground.cellM} m` : 'none'}</span
									>
								</label>
							</li>
						{/if}
						{#each gerardusLayers as layer (layer)}
							<li>
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										checked={isShown(layer, visible)}
										on:change={() => toggle(layer)}
									/>
									<span
										class="inline-block h-2 w-2 rounded-sm"
										style="background:{LAYER_COLOURS[layer] ?? '#64748b'}"
									></span>
									{layerLabel(layer)}
									<span class="text-surface-content/40 ml-auto">{counts[layer] ?? '—'}</span>
								</label>
							</li>
						{/each}
						{#if tab === '3d'}
							<li>
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										checked={isShown('scans', visible)}
										on:change={() => toggle('scans')}
									/>
									<span
										class="inline-block h-2 w-2 rounded-sm"
										style="background:linear-gradient(90deg,#3b82f6,#f97316)"
									></span>
									scan points
									<span class="text-surface-content/40 ml-auto"
										>{scansLoading ? '…' : scans ? scans.count.toLocaleString() : '—'}</span
									>
								</label>
							</li>
						{/if}
					</ul>
					{#if tab === '2d'}
						<div class="text-surface-content/60 pt-2 text-xs font-medium">OpenStreetMap</div>
					{/if}
					<ul class="space-y-0.5 text-xs">
						{#if tab === '2d'}
							<li>
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										checked={isShown('tiles', visible)}
										on:change={() => toggle('tiles')}
									/>
									map tiles (backdrop)
								</label>
							</li>
						{/if}
					</ul>
					{#if !osmAllowed && tab === '2d'}
						<p class="text-surface-content/40 text-xs">
							Nothing from OpenStreetMap is asked for until you allow it, once per visit.
						</p>
					{/if}
				</section>
			{/if}

			{#if tab === '3d' && siteToken && !loadError}
				<section class="space-y-2">
					<div class="text-surface-content/70 font-medium">3D view</div>
					{#if loading3d}
						<p class="text-surface-content/50 text-xs">Loading the site frame…</p>
					{/if}
					{#if error3d}
						<div class="space-y-1 rounded bg-red-50 px-3 py-2 text-xs text-red-700" role="alert">
							<div class="font-medium">{errorTitle(error3d)}</div>
							<p>{error3d.message}</p>
						</div>
					{/if}
					{#if skipped3d > 0}
						<p class="text-xs text-amber-600">
							{skipped3d} feature{skipped3d === 1 ? '' : 's'} had no height or could not be read, and
							are not drawn.
						</p>
					{/if}
					{#if !loading3d && !ground && !groundError && !error3d}
						<p class="text-surface-content/50 text-xs">This site has no ground grid.</p>
					{/if}
					{#if groundError}
						<p class="text-xs text-amber-600">Ground: {groundError}</p>
					{/if}
					<label class="text-surface-content/60 flex items-center gap-2 text-xs">
						Scan voxel
						<select
							class="border-surface-content/20 bg-surface-50 rounded border px-1 py-0.5"
							bind:value={voxelM}
							on:change={loadScans}
						>
							{#each SCAN_VOXELS_M as v (v)}<option value={v}>{v} m</option>{/each}
						</select>
					</label>
					{#if scans}
						<p class="text-surface-content/50 text-xs">
							{scans.count.toLocaleString()} points at {scans.voxelM} m{scans.voxelM !== voxelM
								? ` (Gerardus's limit; ${voxelM} m was asked for)`
								: ''}{scans.modelId ? ` · model ${scans.modelId}` : ''}
						</p>
					{/if}
					{#if scansNote}
						<p class="rounded bg-amber-50 px-2 py-1 text-xs text-amber-700">{scansNote}</p>
					{/if}
					<Button size="sm" variant="outline" on:click={() => scene3d?.resetView()}
						>Reset view</Button
					>
				</section>
			{/if}

			{#if tab === '2d' && selected}
				<section class="border-surface-content/10 space-y-1 rounded border p-2">
					<div class="flex items-center justify-between">
						<div class="font-medium">{layerLabel(selected.layer)}</div>
						<button
							class="text-surface-content/50 p-0.5"
							aria-label="Close the feature's properties"
							on:click={() => (selected = null)}
						>
							<svg class="h-4 w-4" viewBox="0 0 24 24"
								><path fill="currentColor" d={mdiClose} /></svg
							>
						</button>
					</div>
					<table class="w-full text-xs">
						<tbody>
							{#each propertyRows(selected) as [k, v] (k)}
								<tr class="align-top">
									<td class="text-surface-content/50 pr-2 font-mono whitespace-nowrap">{k}</td>
									<td class="font-mono break-all">{v}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</section>
			{:else if tab === '2d' && siteToken && !loadError}
				<p class="text-surface-content/40 text-xs">Click a feature to see its properties.</p>
			{/if}

			<section class="text-surface-content/60 space-y-0.5 text-xs">
				<div class="text-surface-content/70 font-medium">Key</div>
				<div>
					surfaces:
					{#each SURFACE_TYPES as s, i (s)}{i > 0 ? ' · ' : ''}<span
							style="color:{SURFACE_COLOURS[s]}">{s}</span
						>{/each}
				</div>
				<div>
					<span style="color:{STREET_USE_COLOURS.driveway}">driveway</span> · spaces:
					{#each Object.keys(SPACE_COLOURS) as s, i (s)}{i > 0 ? ' · ' : ''}<span
							style="color:{SPACE_COLOURS[s]}">{s}</span
						>{/each}
				</div>
				<div>
					lanes:
					{#each Object.keys(LANE_COLOURS) as s, i (s)}{i > 0 ? ' · ' : ''}<span
							style="color:{LANE_COLOURS[s]}">{s}</span
						>{/each}, arrows in the direction of travel
				</div>
				<div>
					wall planes:
					{#each WALL_ROLES as s, i (s)}{i > 0 ? ' · ' : ''}<span
							style="color:{WALL_ROLE_COLOURS[s]}">{s}</span
						>{/each}
				</div>
				<div>
					observed:
					{#each OBSERVED_KINDS as s, i (s)}{i > 0 ? ' · ' : ''}<span
							style="color:{OBSERVED_COLOURS[s]}">{s}</span
						>{/each}
				</div>
				{#if gerardusLayers.some((l) => l === 'building_parts' || l === 'roof_planes')}
					<div>
						building parts by roof:shape:
						{#each Object.keys(ROOF_SHAPE_COLOURS).slice(0, 6) as s, i (s)}{i > 0 ? ' · ' : ''}<span
								style="color:{ROOF_SHAPE_COLOURS[s]}">{s}</span
							>{/each} · <span style="color:{ROOF_SHAPE_UNKNOWN}">other or unknown</span>
					</div>
					{#if gerardusLayers.includes('wall_surfaces')}
						<div>
							<span style="color:{LAYER_COLOURS.wall_surfaces}">▮</span> building wall, up to the roofline
						</div>
					{/if}
					<div>
						roof planes by slope:
						{#each [0, 15, 30, 45] as d, i (d)}{i > 0 ? ' · ' : ''}<span
								style="color:{roofSlopeColour(d)}">{d}°{d === 45 ? '+' : ''}</span
							>{/each}
					</div>
				{/if}
				{#if gerardusLayers.some((l) => l === 'lane_markings' || l === 'street_furniture' || l === 'trees')}
					<div>
						<span style="color:{MARKING_COLOURS.white}">━</span> white and
						<span style="color:{MARKING_COLOURS.yellow}">━</span> yellow markings ·
						<span style="color:{FURNITURE_COLOURS.pole}">●</span> pole ·
						<span style="color:{FURNITURE_COLOURS.sign}">●</span> sign ·
						<span style="color:{TREE_COLOURS.crown}">◯</span> tree crown
					</div>
				{/if}
				<div>
					<span style="color:{LAYER_COLOURS.boxes}">▭</span> box ·
					<span style="color:{LAYER_COLOURS.footprints}">▭</span> footprint ·
					<span style="color:{LAYER_COLOURS.kerbs}">━━</span> kerb ·
					<span style="color:{LAYER_COLOURS.city_streets}">━</span> centreline
				</div>
				<div class="text-surface-content/50 pt-1">
					Measured or derived geometry at each source's acquisition time; not a plan or survey.
				</div>
			</section>
		</aside>

		<div class="relative min-h-[480px] flex-1 overflow-hidden rounded border">
			<div
				id="site-view-2d"
				role="tabpanel"
				class="absolute inset-0 {tab === '2d' ? '' : 'invisible'}"
				aria-hidden={tab !== '2d'}
			>
				<div bind:this={mapContainer} class="site-map absolute inset-0"></div>
			</div>
			{#if shown3d}
				<div
					id="site-view-3d"
					role="tabpanel"
					class="absolute inset-0 bg-[#0b0f14] {tab === '3d' ? '' : 'invisible'}"
					aria-hidden={tab !== '3d'}
				>
					<SiteScene3D
						bind:this={scene3d}
						features={frameFeatures}
						{ground}
						{scans}
						{visible}
						siteKey={loaded3dFor ?? ''}
					/>
				</div>
			{/if}
			{#if !siteToken && !loading}
				<div
					class="pointer-events-none absolute inset-0 z-[450] flex items-center justify-center p-6 text-center text-sm text-neutral-300"
				>
					Choose a site, or type its L16 token, to see what Gerardus serves for it.
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
			aria-labelledby="osm-consent-title"
		>
			<div class="mb-4 flex items-center justify-between gap-3">
				<h3 id="osm-consent-title" class="text-lg font-semibold">Allow OpenStreetMap Requests?</h3>
				<button
					class="-mt-1 -mr-2 p-1 text-neutral-300 hover:text-white"
					on:click={() => (askingOsm = null)}
					aria-label="Close"
				>
					<svg class="h-5 w-5" viewBox="0 0 24 24"><path fill="currentColor" d={mdiClose} /></svg>
				</button>
			</div>
			<div class="space-y-3 text-sm text-neutral-100">
				<p>
					This page will request OpenStreetMap raster tiles to draw a backdrop behind the site's map
					tiles. Gerardus reference geometry is loaded independently of this browser map.
				</p>
				<p>The map area may be sent to the tile servers.</p>
				<p>
					LiDAR points, street marks, vehicle data, reports and raw sensor data are not sent.
					Nothing is taken from the tiles.
				</p>
				<p>The choice lasts for this visit to the page.</p>
				<p class="text-neutral-300">
					To work without the network, cancel: the layers are drawn on a plain dark ground.
				</p>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<Button on:click={() => (askingOsm = null)} variant="outline">Cancel</Button>
				<Button on:click={allowOsm} variant="fill" color="primary"
					>Allow OpenStreetMap This Visit</Button
				>
			</div>
		</div>
	</div>
{/if}

<style>
	:global(.site-map.leaflet-container) {
		background: #0b0f14;
		font-family: inherit;
	}
</style>
