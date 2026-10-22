<script lang="ts">
	/**
	 * Locate — put a static period on the map.
	 *
	 * The period's settled background (what the sensor saw that does not move)
	 * is drawn from above over OpenStreetMap. The operator finds the place by
	 * address, drops the sensor there, drags it and turns its heading until the
	 * walls line up, and lets Snap refine the fit against OSM's building and
	 * wall outlines with ICP in the browser. Save records the position and the
	 * heading on the period's clip, creating the clip if the period has none.
	 *
	 * Every OpenStreetMap request — tiles, the address search, the outlines —
	 * waits for the operator to allow them for the session, as the site map
	 * editor's tiles do. What leaves is a typed address or the map area, never
	 * sensor data.
	 */
	import {
		ensurePeriodClip,
		getCapturePeriods,
		getPeriodBackground,
		setClipLocation,
		setClipPublishing
	} from '#lib/api.js';
	import { icp2d, thin, type IcpResult } from '#lib/align/icp.js';
	import {
		fetchOutlines,
		outlineSegments,
		searchPlaces,
		type Outline,
		type Place
	} from '#lib/align/osm.js';
	import {
		fromLocal,
		headingFromNorthAzimuth,
		movePose,
		northAzimuthFromHeading,
		placeScan,
		sensorToEN,
		toLocal,
		wrapDegrees,
		type Pose
	} from '#lib/align/pose.js';
	import { formatClock, formatDuration } from '#lib/captures/timeline.js';
	import { clipQuery } from '#lib/lidarLinks.js';
	import type { MotionPeriod, PeriodClip } from '#lib/types/captures.js';
	import { mdiClose } from '@mdi/js';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import 'leaflet/dist/leaflet.css';
	import type { LayerGroup, Map as LeafletMap, Marker } from 'leaflet';
	import { onDestroy, onMount, tick } from 'svelte';
	import { Button } from 'svelte-ux';

	const sessionId = page.url.searchParams.get('session') ?? '';
	const periodId = page.url.searchParams.get('period') ?? '';

	// Height bands, in metres relative to the sensor, which sits about 2.3 m
	// above the road. Walls are what OSM outlines; the ground shows kerbs.
	const BANDS = {
		walls: { label: 'walls (−1.5 to +4 m)', zMin: -1.5, zMax: 4 },
		ground: { label: 'ground (below −1.5 m)', zMin: -Infinity, zMax: -1.5 },
		all: { label: 'everything', zMin: -Infinity, zMax: Infinity }
	} as const;
	type Band = keyof typeof BANDS;
	const HANDLE_METRES = 20;
	const OUTLINE_RADIUS = 150;

	let period: MotionPeriod | null = null;
	let periodClip: PeriodClip | null = null;
	let xyz: Float32Array | null = null;
	let frames = 0;
	let loadingPeriod = true;
	let loadingBackground = false;
	let error: string | null = null;

	let pose: Pose | null = null;
	let band: Band = 'walls';

	let L: typeof import('leaflet') | null = null;
	let map: LeafletMap | null = null;
	let mapContainer: HTMLDivElement;
	let canvas: HTMLCanvasElement;
	let sensorMarker: Marker | null = null;
	let headingMarker: Marker | null = null;
	// While the operator drags the heading handle it follows the pointer, not the pose.
	let turning = false;
	let outlineLayer: LayerGroup | null = null;

	// Consent for OpenStreetMap requests, for this page's session.
	type OsmAction = 'tiles' | 'search' | 'snap';
	let osmAllowed = false;
	let pendingAction: OsmAction | null = null;
	let tilesAdded = false;

	let query = '';
	let places: Place[] = [];
	let searching = false;

	let outlines: Outline[] = [];
	let outlinesCentre: { lat: number; lon: number } | null = null;
	let snapping = false;
	let lastSnap: IcpResult | null = null;
	let beforeSnap: Pose | null = null;

	let saving = false;
	let savedClipId: string | null = null;
	let savedMessage: string | null = null;

	$: headingText = pose ? pose.headingDeg.toFixed(1) : '';
	$: if (map && pose) syncMarkers(pose);
	$: if (map && (pose || xyz || band)) redraw();

	onMount(async () => {
		L = await import('leaflet');
		await loadPeriod();
		await tick();
		initMap();
		void loadBackground();
	});

	onDestroy(() => {
		map?.remove();
	});

	async function loadPeriod() {
		if (!sessionId || !periodId) {
			error = 'Open this page from a static period on the Captures page.';
			loadingPeriod = false;
			return;
		}
		try {
			const r = await getCapturePeriods(sessionId);
			period = r.periods.find((p) => p.period_id === periodId) ?? null;
			periodClip = r.clips?.[periodId] ?? null;
			if (!period) error = `No period ${periodId} in session ${sessionId}.`;
			if (periodClip?.located && periodClip.origin_lat != null && periodClip.origin_lon != null) {
				pose = {
					lat: periodClip.origin_lat,
					lon: periodClip.origin_lon,
					headingDeg:
						periodClip.north_azimuth_deg != null
							? headingFromNorthAzimuth(periodClip.north_azimuth_deg)
							: 0
				};
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the period.';
		} finally {
			loadingPeriod = false;
		}
	}

	async function loadBackground() {
		if (!period || period.type !== 'static') return;
		loadingBackground = true;
		try {
			const bg = await getPeriodBackground(periodId);
			xyz = Float32Array.from(bg.xyz);
			frames = bg.frames;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not build the settled background.';
		} finally {
			loadingBackground = false;
		}
	}

	function initMap() {
		if (!L || !mapContainer) return;
		const centre = pose ?? { lat: 37.7749, lon: -122.4194 };
		map = L.map(mapContainer, {
			center: [centre.lat, centre.lon],
			zoom: pose ? 19 : 13,
			maxZoom: 22
		});
		map.on('move zoom resize', redraw);
		map.on('click', (e) => {
			if (!pose) placeSensor(e.latlng.lat, e.latlng.lng);
		});
		outlineLayer = L.layerGroup().addTo(map);
		if (pose) syncMarkers(pose);
	}

	// ---- OpenStreetMap consent ------------------------------------------------

	function requestOsm(action: OsmAction) {
		if (osmAllowed) {
			void runOsm(action);
			return;
		}
		pendingAction = action;
	}

	function allowOsm() {
		const action = pendingAction;
		osmAllowed = true;
		pendingAction = null;
		addTiles();
		if (action && action !== 'tiles') void runOsm(action);
	}

	async function runOsm(action: OsmAction) {
		if (action === 'tiles') addTiles();
		else if (action === 'search') await search();
		else await snap();
	}

	function addTiles() {
		// The gate lives here: no path adds tiles without consent.
		if (!L || !map || tilesAdded || !osmAllowed) return;
		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			attribution: '© OpenStreetMap contributors',
			maxZoom: 22,
			maxNativeZoom: 19
		}).addTo(map);
		tilesAdded = true;
	}

	// ---- Placing the sensor ---------------------------------------------------

	async function search() {
		if (!osmAllowed || query.trim() === '') return;
		searching = true;
		error = null;
		try {
			places = await searchPlaces(query);
			if (places.length === 0) error = `Nothing found for “${query}”.`;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Address search failed.';
		} finally {
			searching = false;
		}
	}

	function goTo(place: Place) {
		places = [];
		map?.setView([place.lat, place.lon], 19);
		placeSensor(place.lat, place.lon);
	}

	function placeSensor(lat: number, lon: number) {
		pose = { lat, lon, headingDeg: pose?.headingDeg ?? 0 };
		// The scan spans a street: below street zoom it is a dot.
		if (map && map.getZoom() < 18) map.setView([lat, lon], 19);
		lastSnap = null;
		savedMessage = null;
	}

	function handlePosition(p: Pose) {
		const tip = fromLocal(p, sensorToEN(0, HANDLE_METRES, p.headingDeg));
		return [tip.lat, tip.lon] as [number, number];
	}

	function syncMarkers(p: Pose) {
		if (!L || !map) return;
		if (!sensorMarker) {
			sensorMarker = L.marker([p.lat, p.lon], {
				draggable: true,
				icon: L.divIcon({
					className: '',
					html: '<div style="width:16px;height:16px;border-radius:50%;background:#3b82f6;border:2px solid white;box-shadow:0 0 2px #000"></div>',
					iconSize: [16, 16],
					iconAnchor: [8, 8]
				}),
				title: 'Sensor: drag to move'
			}).addTo(map);
			sensorMarker.on('drag', () => {
				const at = sensorMarker!.getLatLng();
				pose = { ...pose!, lat: at.lat, lon: at.lng };
				lastSnap = null;
			});
		} else {
			sensorMarker.setLatLng([p.lat, p.lon]);
		}
		if (!headingMarker) {
			headingMarker = L.marker(handlePosition(p), {
				draggable: true,
				icon: L.divIcon({
					className: '',
					html: '<div style="width:12px;height:12px;border-radius:50%;background:#f97316;border:2px solid white;box-shadow:0 0 2px #000"></div>',
					iconSize: [12, 12],
					iconAnchor: [6, 6]
				}),
				title: 'Heading (+Y): drag to turn'
			}).addTo(map);
			headingMarker.on('dragstart', () => (turning = true));
			headingMarker.on('drag', () => {
				const at = headingMarker!.getLatLng();
				const v = toLocal(pose!, at.lat, at.lng);
				pose = {
					...pose!,
					headingDeg: wrapDegrees((Math.atan2(v.east, v.north) * 180) / Math.PI)
				};
				lastSnap = null;
			});
			headingMarker.on('dragend', () => {
				turning = false;
				headingMarker?.setLatLng(handlePosition(pose!));
			});
		} else if (!turning) {
			headingMarker.setLatLng(handlePosition(p));
		}
	}

	function nudge(dEast: number, dNorth: number, dHeading: number) {
		if (!pose) return;
		pose = movePose(pose, dEast, dNorth, dHeading);
		lastSnap = null;
	}

	function setHeading(text: string) {
		const v = Number(text);
		if (pose && Number.isFinite(v)) pose = { ...pose, headingDeg: wrapDegrees(v) };
	}

	// ---- Drawing the background -------------------------------------------------

	function colourFor(z: number): string {
		// Below the road is blue, walls green to yellow, high structure red.
		const t = Math.max(0, Math.min(1, (z + 3) / 9));
		const hue = 220 - 220 * t;
		return `hsl(${hue}, 90%, 50%)`;
	}

	function redraw() {
		if (!canvas || !map || !mapContainer) return;
		const w = mapContainer.clientWidth;
		const h = mapContainer.clientHeight;
		const dpr = window.devicePixelRatio || 1;
		if (canvas.width !== w * dpr || canvas.height !== h * dpr) {
			canvas.width = w * dpr;
			canvas.height = h * dpr;
			canvas.style.width = `${w}px`;
			canvas.style.height = `${h}px`;
		}
		const ctx = canvas.getContext('2d');
		if (!ctx) return;
		ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
		ctx.clearRect(0, 0, w, h);
		if (!pose || !xyz) return;

		// Over a site the map is linear: three projected points give the affine
		// map from east/north metres to pixels.
		const p0 = map.latLngToContainerPoint([pose.lat, pose.lon]);
		const e1 = fromLocal(pose, { east: 1, north: 0 });
		const n1 = fromLocal(pose, { east: 0, north: 1 });
		const pe = map.latLngToContainerPoint([e1.lat, e1.lon]);
		const pn = map.latLngToContainerPoint([n1.lat, n1.lon]);
		const ex = pe.x - p0.x;
		const ey = pe.y - p0.y;
		const nx = pn.x - p0.x;
		const ny = pn.y - p0.y;
		const size = Math.max(1.5, Math.min(4, Math.hypot(ex, ey) * 0.15));
		const { zMin, zMax } = BANDS[band];
		const t = (pose.headingDeg * Math.PI) / 180;
		const c = Math.cos(t);
		const s = Math.sin(t);
		let lastColour = '';
		for (let i = 0; i + 2 < xyz.length; i += 3) {
			const z = xyz[i + 2];
			if (z < zMin || z > zMax) continue;
			const x = xyz[i];
			const y = xyz[i + 1];
			const east = x * c + y * s;
			const north = -x * s + y * c;
			const px = p0.x + east * ex + north * nx;
			const py = p0.y + east * ey + north * ny;
			if (px < -4 || py < -4 || px > w + 4 || py > h + 4) continue;
			const colour = colourFor(z);
			if (colour !== lastColour) {
				ctx.fillStyle = colour;
				lastColour = colour;
			}
			ctx.fillRect(px - size / 2, py - size / 2, size, size);
		}
	}

	// ---- Snap -------------------------------------------------------------------

	async function snap() {
		if (!pose || !xyz || !osmAllowed) return;
		snapping = true;
		error = null;
		try {
			if (
				!outlinesCentre ||
				Math.hypot(...Object.values(toLocal(outlinesCentre, pose.lat, pose.lon))) > 50
			) {
				outlines = await fetchOutlines(pose.lat, pose.lon, OUTLINE_RADIUS);
				outlinesCentre = { lat: pose.lat, lon: pose.lon };
				drawOutlines();
			}
			const segments = outlineSegments(outlines, pose);
			if (segments.length === 0) {
				error = 'OpenStreetMap has no building or wall outlines here to snap to.';
				return;
			}
			const scan = thin(placeScan(xyz, pose, pose, BANDS.walls.zMin, BANDS.walls.zMax), 0.25);
			const result = icp2d(scan, segments, { east: 0, north: 0 }, { maxDistance: 4 });
			if (result.inliers < 50) {
				error = `Snap found only ${result.inliers} wall points near an outline: move the sensor closer by hand first.`;
				lastSnap = result;
				return;
			}
			beforeSnap = pose;
			pose = movePose(pose, result.dEast, result.dNorth, result.dHeadingDeg);
			lastSnap = result;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Snap failed.';
		} finally {
			snapping = false;
		}
	}

	function undoSnap() {
		if (beforeSnap) pose = beforeSnap;
		beforeSnap = null;
		lastSnap = null;
	}

	function drawOutlines() {
		if (!L || !outlineLayer) return;
		outlineLayer.clearLayers();
		for (const o of outlines) {
			L.polyline(
				o.points.map((p) => [p.lat, p.lon] as [number, number]),
				{ color: o.kind === 'building' ? '#f97316' : '#a855f7', weight: 1.5, opacity: 0.9 }
			).addTo(outlineLayer);
		}
	}

	// ---- Save -------------------------------------------------------------------

	async function save() {
		if (!pose) return;
		saving = true;
		error = null;
		savedMessage = null;
		try {
			const clip = await ensurePeriodClip(periodId);
			await setClipLocation(clip.clip_id, {
				origin_lat: pose.lat,
				origin_lon: pose.lon,
				geographic_source: 'operator'
			});
			await setClipPublishing(clip.clip_id, {
				dataset_id: clip.dataset_id ?? '',
				north_azimuth_deg: Math.round(northAzimuthFromHeading(pose.headingDeg) * 10) / 10,
				storage: clip.storage ?? 'sources'
			});
			savedClipId = clip.clip_id;
			periodClip = {
				clip_id: clip.clip_id,
				dataset_id: clip.dataset_id,
				located: true,
				origin_lat: pose.lat,
				origin_lon: pose.lon,
				north_azimuth_deg: northAzimuthFromHeading(pose.headingDeg)
			};
			savedMessage = `Saved on clip ${clip.clip_id.slice(0, 8)}: ${pose.lat.toFixed(6)}, ${pose.lon.toFixed(6)}, heading ${pose.headingDeg.toFixed(1)}°.`;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save the location.';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Locate — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<header class="flex items-start justify-between gap-4">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Locate a static period</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					The settled background is what the sensor saw that does not move, drawn from above. Place
					the sensor, turn it until the walls sit on the buildings, then Snap and Save.
				</p>
			</div>
			<a
				href={resolve('/lidar/captures')}
				class="text-primary text-sm whitespace-nowrap hover:underline">← Captures</a
			>
		</header>
	</div>

	<div class="flex min-h-0 flex-1 gap-4 p-4">
		<aside class="w-80 flex-none space-y-4 overflow-y-auto text-sm">
			{#if loadingPeriod}
				<p class="text-surface-content/60">Loading the period…</p>
			{:else if period}
				<section class="space-y-1">
					<div class="font-mono text-xs">{period.label} · {sessionId}</div>
					<div class="text-surface-content/60 text-xs">
						{formatClock(period.start_ns, true)} – {formatClock(period.end_ns, true)} · {formatDuration(
							period.duration_ns / 1e9
						)}
					</div>
					<div class="text-xs">
						{#if periodClip?.located}
							<span class="text-emerald-600">Located</span> on clip {periodClip.clip_id.slice(0, 8)}
						{:else if periodClip}
							Clip {periodClip.clip_id.slice(0, 8)}, not located yet
						{:else}
							No clip yet; Save makes one
						{/if}
					</div>
				</section>
			{/if}

			<section class="space-y-1">
				<div class="text-surface-content/70 font-medium">Background</div>
				{#if loadingBackground}
					<p class="text-surface-content/60 text-xs">
						Building the settled background from the period's first 90 s. This takes seconds to a
						minute the first time.
					</p>
				{:else if xyz}
					<p class="text-surface-content/60 text-xs">
						{(xyz.length / 3).toLocaleString()} settled points from {frames} rotations.
					</p>
				{/if}
				<select
					class="border-surface-content/20 bg-surface-50 w-full rounded border px-2 py-1 text-xs"
					bind:value={band}
				>
					{#each Object.entries(BANDS) as [key, b] (key)}
						<option value={key}>{b.label}</option>
					{/each}
				</select>
			</section>

			<section class="space-y-2">
				<div class="text-surface-content/70 font-medium">Find the place</div>
				{#if !osmAllowed}
					<Button size="sm" variant="outline" on:click={() => requestOsm('tiles')}
						>Show the OpenStreetMap map</Button
					>
				{/if}
				<form class="flex gap-2" on:submit|preventDefault={() => requestOsm('search')}>
					<input
						class="border-surface-content/20 bg-surface-50 min-w-0 flex-1 rounded border px-2 py-1 text-xs"
						placeholder="address or junction"
						bind:value={query}
					/>
					<Button size="sm" variant="outline" type="submit" disabled={searching}
						>{searching ? '…' : 'Search'}</Button
					>
				</form>
				{#if places.length > 0}
					<ul class="border-surface-content/10 divide-y rounded border text-xs">
						{#each places as place (place.label)}
							<li>
								<button
									class="hover:bg-surface-200 w-full px-2 py-1 text-left"
									on:click={() => goTo(place)}>{place.label}</button
								>
							</li>
						{/each}
					</ul>
				{/if}
				{#if !pose}
					<p class="text-surface-content/50 text-xs">Or click the map to drop the sensor.</p>
				{/if}
			</section>

			{#if pose}
				<section class="space-y-2">
					<div class="text-surface-content/70 font-medium">Align</div>
					<div class="text-surface-content/60 font-mono text-xs">
						{pose.lat.toFixed(6)}, {pose.lon.toFixed(6)}
					</div>
					<label class="text-surface-content/60 flex items-center gap-2 text-xs">
						Heading of +Y (°)
						<input
							class="border-surface-content/20 bg-surface-50 w-20 rounded border px-2 py-1 font-mono text-xs"
							value={headingText}
							on:change={(e) => setHeading(e.currentTarget.value)}
						/>
					</label>
					<div class="text-surface-content/50 text-xs">
						North azimuth {northAzimuthFromHeading(pose.headingDeg).toFixed(1)}° (as site-index.json
						records it)
					</div>
					<div class="grid grid-cols-4 gap-1 text-xs">
						<Button size="sm" variant="outline" on:click={() => nudge(0, 0.25, 0)}>N</Button>
						<Button size="sm" variant="outline" on:click={() => nudge(0, -0.25, 0)}>S</Button>
						<Button size="sm" variant="outline" on:click={() => nudge(-0.25, 0, 0)}>W</Button>
						<Button size="sm" variant="outline" on:click={() => nudge(0.25, 0, 0)}>E</Button>
						<Button size="sm" variant="outline" on:click={() => nudge(0, 0, -1)}>↺ 1°</Button>
						<Button size="sm" variant="outline" on:click={() => nudge(0, 0, -0.2)}>↺ .2°</Button>
						<Button size="sm" variant="outline" on:click={() => nudge(0, 0, 0.2)}>↻ .2°</Button>
						<Button size="sm" variant="outline" on:click={() => nudge(0, 0, 1)}>↻ 1°</Button>
					</div>
					<p class="text-surface-content/50 text-xs">
						Drag the blue dot to move, the orange dot to turn. Nudges move 25 cm.
					</p>
					<div class="flex gap-2">
						<Button
							size="sm"
							variant="outline"
							disabled={snapping || !xyz}
							on:click={() => requestOsm('snap')}
							>{snapping ? 'Snapping…' : 'Snap to outlines'}</Button
						>
						{#if beforeSnap}
							<Button size="sm" variant="outline" on:click={undoSnap}>Undo snap</Button>
						{/if}
					</div>
					{#if lastSnap}
						<p class="text-surface-content/60 text-xs">
							Snap moved {Math.hypot(lastSnap.dEast, lastSnap.dNorth).toFixed(2)} m and {lastSnap.dHeadingDeg.toFixed(
								2
							)}°; {lastSnap.inliers} of {lastSnap.total} wall points within reach, RMS {Number.isFinite(
								lastSnap.rms
							)
								? `${lastSnap.rms.toFixed(2)} m`
								: '—'}.
						</p>
					{/if}
					<Button variant="fill" color="primary" disabled={saving} on:click={save}
						>{saving ? 'Saving…' : 'Save location'}</Button
					>
				</section>
			{/if}

			{#if error}
				<p class="rounded bg-red-50 px-3 py-2 text-xs text-red-600">{error}</p>
			{/if}
			{#if savedMessage}
				<p class="rounded bg-emerald-50 px-3 py-2 text-xs text-emerald-700">
					{savedMessage}
					{#if savedClipId}
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a href={`${resolve('/lidar/clips')}?${clipQuery(savedClipId)}`} class="ml-1 underline"
							>Open clip →</a
						>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					{/if}
				</p>
			{/if}
		</aside>

		<div class="relative min-h-[480px] flex-1 overflow-hidden rounded border">
			<div bind:this={mapContainer} class="absolute inset-0"></div>
			<canvas bind:this={canvas} class="pointer-events-none absolute inset-0 z-[450]"></canvas>
		</div>
	</div>
</main>

{#if pendingAction}
	<div
		class="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70 p-4"
		role="presentation"
	>
		<div
			class="w-full max-w-md rounded border border-neutral-700 bg-neutral-950 p-6 text-white shadow-2xl"
			role="alertdialog"
			aria-modal="true"
			aria-labelledby="osm-request-title"
		>
			<div class="mb-4 flex items-center justify-between gap-3">
				<h3 id="osm-request-title" class="text-lg font-semibold">Allow OpenStreetMap Requests?</h3>
				<button
					class="-mt-1 -mr-2 p-1 text-neutral-300 hover:text-white"
					on:click={() => (pendingAction = null)}
					aria-label="Close"
				>
					<svg class="h-5 w-5" viewBox="0 0 24 24"><path fill="currentColor" d={mdiClose} /></svg>
				</button>
			</div>
			<div class="space-y-3 text-sm text-neutral-100">
				<p>
					This page will request OpenStreetMap map tiles, search the address you type with
					Nominatim, and fetch building and wall outlines from Overpass for Snap.
				</p>
				<p>The address you type and the map area may be sent to those servers.</p>
				<p>LiDAR points, vehicle data, reports and raw sensor data are not sent.</p>
				<p class="text-neutral-300">
					To work without the network, cancel and type the coordinates on the Captures page instead.
				</p>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<Button on:click={() => (pendingAction = null)} variant="outline">Cancel</Button>
				<Button on:click={allowOsm} variant="fill" color="primary"
					>Allow OpenStreetMap This Session</Button
				>
			</div>
		</div>
	</div>
{/if}

<style>
	:global(.leaflet-container) {
		font-family: inherit;
	}
</style>
