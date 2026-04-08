<script lang="ts">
	import {
		buildNodeLookup,
		buildOverpassQueries,
		categoriseElements,
		generateMapSvg,
		orderedEndpoints,
		OVERPASS_MIRRORS,
		svgToBase64
	} from '$lib/map-svg';
	import {
		mdiAlert,
		mdiCheckCircle,
		mdiClose,
		mdiCrosshairsGps,
		mdiDelete,
		mdiDownload,
		mdiMagnify
	} from '@mdi/js';
	import type { LatLngBounds, Map as LeafletMap, Marker, Rectangle } from 'leaflet';
	import { onDestroy, onMount, tick } from 'svelte';
	import {
		Button,
		Dialog,
		Notification,
		NumberStepper,
		ProgressCircle,
		SelectField,
		Switch,
		TextField,
		ToggleGroup,
		ToggleOption
	} from 'svelte-ux';

	// Props
	export let latitude: number | null = null;
	export let longitude: number | null = null;
	export let radarAngle: number | null = null;
	export let bboxNELat: number | null = null;
	export let bboxNELng: number | null = null;
	export let bboxSWLat: number | null = null;
	export let bboxSWLng: number | null = null;
	export let mapSvgData: string | null = null;
	export let includeMap: boolean = true;
	export let radarSvgX: number | null = null;
	export let radarSvgY: number | null = null;

	// Local state
	let map: LeafletMap | null = null;
	let radarMarker: Marker | null = null;
	let bboxRect: Rectangle | null = null;
	let fovPolygon: L.Polygon | null = null;
	let fovTipMarker: Marker | null = null;
	let mapContainer: HTMLElement;
	let searchQuery = '';
	let searchResults: Array<{
		display_name: string;
		lat: string;
		lon: string;
		place_id?: number | string;
	}> = [];
	let searching = false;
	let downloading = false;
	let downloadStep = ''; // e.g. '1/2 Roads…', '2/2 Detail…'
	let error = '';
	let selectedMirror = '';
	let abortController: AbortController | null = null;
	let L: typeof import('leaflet') | null = null;
	let isDraggingFovTip = false; // Flag to prevent reactive updates during drag
	let lastSearchTime = 0; // Track last API call for rate limiting
	let mapJustDownloaded = false; // Track if map was just downloaded (not loaded from DB)

	// Confirmation modal state for mode switching
	let showDeleteMapModal = false;
	let pendingModeSwitch: 'interactive' | 'upload' | null = null;
	let toggleResetKey = 0;

	// Confirmation modal state for replacing an uploaded SVG
	let showReplaceMapModal = false;

	/** Request a mode switch. If existing map data would be lost, show confirmation. */
	function requestModeSwitch(target: 'interactive' | 'upload') {
		if (mapSvgData) {
			pendingModeSwitch = target;
			showDeleteMapModal = true;
		} else {
			applyModeSwitch(target);
		}
	}

	/** Apply the mode switch, clearing existing map data. */
	async function applyModeSwitch(target: 'interactive' | 'upload') {
		showDeleteMapModal = false;
		pendingModeSwitch = null;
		if (target === 'interactive') {
			useCustomSvg = false;
			radarSvgX = null;
			radarSvgY = null;
			mapSvgData = null;
			mapJustDownloaded = false;
			await tick();
			if (!map) initializeMap();
		} else {
			useCustomSvg = true;
			mapSvgData = null;
			mapJustDownloaded = false;
			fileInput.click();
		}
	}

	function cancelModeSwitch() {
		showDeleteMapModal = false;
		pendingModeSwitch = null;
		mapMode = useCustomSvg ? 'upload' : 'interactive';
		toggleResetKey++;
	}

	// Auto-enable the "Include map in reports" toggle when the user actively
	// configures the map — interacting with any placement, angle, download, or
	// upload control signals intent to include a map.
	function activateIncludeMap() {
		if (!includeMap) includeMap = true;
	}

	// Custom SVG upload — restore mode when a custom SVG is stored without geographic bounds.
	// Generated SVGs always have bbox set; custom uploads clear bbox in handleSvgUpload.
	let useCustomSvg =
		(radarSvgX !== null && radarSvgY !== null) ||
		(mapSvgData !== null &&
			bboxNELat === null &&
			bboxNELng === null &&
			bboxSWLat === null &&
			bboxSWLng === null);
	let mapMode: 'interactive' | 'upload' = useCustomSvg ? 'upload' : 'interactive';
	let fileInput: HTMLInputElement;
	let svgPreviewContainer: HTMLDivElement;
	let isDraggingSvgDot = false;

	function handleSvgPreviewClick(event: MouseEvent) {
		if (!svgPreviewContainer) return;
		const rect = svgPreviewContainer.getBoundingClientRect();
		const x = ((event.clientX - rect.left) / rect.width) * 100;
		const y = ((event.clientY - rect.top) / rect.height) * 100;
		// Clamp: 5% border left/right, 10% border top/bottom
		radarSvgX = Math.max(5, Math.min(95, x));
		radarSvgY = Math.max(10, Math.min(90, y));
		activateIncludeMap();
	}

	function handleSvgDotDrag(event: MouseEvent) {
		if (!isDraggingSvgDot || !svgPreviewContainer) return;
		event.preventDefault();
		const rect = svgPreviewContainer.getBoundingClientRect();
		const x = ((event.clientX - rect.left) / rect.width) * 100;
		const y = ((event.clientY - rect.top) / rect.height) * 100;
		// Clamp: 5% border left/right, 10% border top/bottom
		radarSvgX = Math.max(5, Math.min(95, x));
		radarSvgY = Math.max(10, Math.min(90, y));
	}

	function stopSvgDotDrag() {
		isDraggingSvgDot = false;
		window.removeEventListener('mousemove', handleSvgDotDrag);
		window.removeEventListener('mouseup', stopSvgDotDrag);
	}

	function startSvgDotDrag(event: MouseEvent) {
		event.stopPropagation();
		event.preventDefault();
		isDraggingSvgDot = true;
		window.addEventListener('mousemove', handleSvgDotDrag);
		window.addEventListener('mouseup', stopSvgDotDrag);
	}

	/** Handle file-picker cancel: revert to interactive if no SVG was loaded. */
	function handleFileInputCancel() {
		if (!mapSvgData) {
			useCustomSvg = false;
			mapMode = 'interactive';
			toggleResetKey++;
			tick().then(() => {
				if (!map) initializeMap();
			});
		}
	}

	/** Confirm removal of uploaded SVG, then open the file picker for a replacement. */
	function confirmReplaceMap() {
		showReplaceMapModal = false;
		mapSvgData = null;
		radarSvgX = null;
		radarSvgY = null;
		mapJustDownloaded = false;
		fileInput.click();
	}

	function handleSvgUpload(event: Event) {
		const input = event.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		if (!file.name.toLowerCase().endsWith('.svg') && file.type !== 'image/svg+xml') {
			error = 'Please select an SVG file.';
			return;
		}
		const reader = new FileReader();
		reader.onload = () => {
			const text = reader.result as string;
			// Encode to base64
			const encoder = new TextEncoder();
			const bytes = encoder.encode(text);
			const chunkSize = 8192;
			let binaryString = '';
			for (let i = 0; i < bytes.length; i += chunkSize) {
				const chunk = bytes.slice(i, i + chunkSize);
				binaryString += String.fromCharCode(...chunk);
			}
			mapSvgData = btoa(binaryString);
			useCustomSvg = true;
			error = '';
			mapJustDownloaded = true;
			// Clear geographic bounds — custom SVGs have no bbox.
			// This ensures the page correctly restores custom SVG mode on reload.
			bboxNELat = null;
			bboxNELng = null;
			bboxSWLat = null;
			bboxSWLng = null;
			activateIncludeMap();
		};
		reader.onerror = () => {
			error = 'Failed to read file.';
		};
		reader.readAsText(file);
		// Reset input so the same file can be re-selected
		input.value = '';
	}

	// NumberStepper needs number, but radarAngle prop is number|null.
	// localAngle is the display value; setAngle() syncs both and redraws FOV.
	let localAngle: number = radarAngle ?? 0;

	function setAngle(deg: number) {
		// Wrap to 0–359 so stepping past either end loops around
		const wrapped = ((deg % 360) + 360) % 360;
		localAngle = wrapped;
		radarAngle = wrapped;
		activateIncludeMap();
		if (map && latitude !== null && longitude !== null && !isDraggingFovTip) {
			updateFOVTriangle();
		}
	}

	// Default location (San Francisco, USA)
	const defaultLat = 37.7749;
	const defaultLng = -122.4194;

	onMount(async () => {
		// Dynamically import Leaflet to avoid SSR issues
		L = await import('leaflet');

		// Import Leaflet CSS
		const link = document.createElement('link');
		link.rel = 'stylesheet';
		link.href = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
		document.head.appendChild(link);

		initializeMap();
	});

	onDestroy(() => {
		if (map) {
			map.remove();
		}
	});

	function initializeMap() {
		if (!L || !mapContainer) return;

		// Use existing coordinates or defaults
		const centerLat = latitude || defaultLat;
		const centerLng = longitude || defaultLng;

		// Create map
		map = L.map(mapContainer, {
			center: [centerLat, centerLng],
			zoom: 15,
			zoomControl: true
		});

		// Add OpenStreetMap tiles
		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			attribution: '© OpenStreetMap contributors',
			maxZoom: 19
		}).addTo(map);

		// Create custom icon for radar marker
		const radarIcon = L.icon({
			iconUrl:
				'data:image/svg+xml;base64,' +
				btoa(`
				<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="32" height="32">
					<circle cx="12" cy="12" r="10" fill="#3b82f6" stroke="white" stroke-width="2"/>
					<path d="M12 2 L12 12 L20 12" stroke="white" stroke-width="2" fill="none"/>
				</svg>
			`),
			iconSize: [32, 32],
			iconAnchor: [16, 16]
		});

		// Add radar marker
		radarMarker = L.marker([centerLat, centerLng], {
			icon: radarIcon,
			draggable: true
		}).addTo(map);

		// Initialize coordinates if not set
		if (latitude === null) latitude = centerLat;
		if (longitude === null) longitude = centerLng;
		if (radarAngle === null) setAngle(0); // Default to north

		radarMarker.on('dragend', () => {
			if (!radarMarker) return;
			const pos = radarMarker.getLatLng();
			latitude = pos.lat;
			longitude = pos.lng;
			updateBBoxAroundRadar(true); // true = maintain size
			activateIncludeMap();
		});

		// Add bounding box if it exists
		if (bboxNELat && bboxNELng && bboxSWLat && bboxSWLng) {
			const bboxBounds = L.latLngBounds([bboxSWLat, bboxSWLng], [bboxNELat, bboxNELng]);
			addBoundingBox(bboxBounds);
			// Fit the viewport to the bounding box so the map area is visible on load
			map.fitBounds(bboxBounds.pad(0.15));
		} else {
			// Create default bounding box around radar
			updateBBoxAroundRadar();
		}

		// Initialize FOV triangle if angle is set
		updateFOVTriangle();
	}

	function updateFOVTriangle() {
		if (!L || !map || latitude === null || longitude === null) return;

		// Remove existing FOV polygon and marker
		if (fovPolygon) {
			map.removeLayer(fovPolygon);
			fovPolygon = null;
		}
		if (fovTipMarker) {
			map.removeLayer(fovTipMarker);
			fovTipMarker = null;
		}

		// Only draw if angle is set
		if (radarAngle === null) return;

		// FOV parameters
		const fovWidthDegrees = 20; // Field of view width in degrees
		const fovDistanceMeters = 100; // Distance in meters

		// Convert 100m to degrees (approximate: 1 degree lat ≈ 111km)
		const metersPerDegreeLat = 111320;
		const metersPerDegreeLng = 111320 * Math.cos((latitude * Math.PI) / 180);
		const fovDistanceLat = fovDistanceMeters / metersPerDegreeLat;
		const fovDistanceLng = fovDistanceMeters / metersPerDegreeLng;

		// Radar angle: 0 = North, 90 = East, 180 = South, 270 = West
		// Map bearing is the same convention
		const bearingDegrees = radarAngle;
		const bearingRad = (bearingDegrees * Math.PI) / 180;
		const leftBearingRad = ((bearingDegrees - fovWidthDegrees / 2) * Math.PI) / 180;
		const rightBearingRad = ((bearingDegrees + fovWidthDegrees / 2) * Math.PI) / 180;

		// Calculate center tip point
		const tipLat = latitude + Math.cos(bearingRad) * fovDistanceLat;
		const tipLng = longitude + Math.sin(bearingRad) * fovDistanceLng;

		// Calculate left and right edge points at 100m distance
		const leftLat = latitude + Math.cos(leftBearingRad) * fovDistanceLat;
		const leftLng = longitude + Math.sin(leftBearingRad) * fovDistanceLng;
		const rightLat = latitude + Math.cos(rightBearingRad) * fovDistanceLat;
		const rightLng = longitude + Math.sin(rightBearingRad) * fovDistanceLng;

		// Validate all coordinates
		if (
			isNaN(leftLat) ||
			isNaN(leftLng) ||
			isNaN(rightLat) ||
			isNaN(rightLng) ||
			isNaN(tipLat) ||
			isNaN(tipLng)
		) {
			console.error('Invalid FOV coordinates calculated');
			return;
		}

		// Create triangle: radar origin -> left edge -> right edge -> back to origin
		fovPolygon = L.polygon(
			[
				[latitude, longitude], // Radar position (origin)
				[leftLat, leftLng], // Left edge at 100m
				[rightLat, rightLng] // Right edge at 100m
			],
			{
				color: '#ef4444',
				fillColor: '#ef4444',
				fillOpacity: 0.3,
				weight: 2
			}
		).addTo(map);

		// Add draggable marker at the tip
		const tipIcon = L.divIcon({
			html: '<div style="width: 12px; height: 12px; background: #ef4444; border: 2px solid white; border-radius: 50%; cursor: move;"></div>',
			iconSize: [12, 12],
			iconAnchor: [6, 6],
			className: ''
		});

		fovTipMarker = L.marker([tipLat, tipLng], {
			icon: tipIcon,
			draggable: true,
			zIndexOffset: 1000
		}).addTo(map);

		// Set flag when drag starts to prevent reactive updates from recreating marker
		fovTipMarker.on('dragstart', () => {
			isDraggingFovTip = true;
		});

		// Update angle and polygon during drag (without recreating marker)
		fovTipMarker.on('drag', () => {
			if (!fovTipMarker || !fovPolygon || latitude === null || longitude === null) return;
			const tipPos = fovTipMarker.getLatLng();

			// Calculate angle from radar to tip
			const dLat = tipPos.lat - latitude;
			const dLng = tipPos.lng - longitude;

			// atan2(dLng, dLat) gives angle where 0 = North
			let angle = Math.atan2(dLng, dLat) * (180 / Math.PI);

			// Normalize to 0-360
			if (angle < 0) angle += 360;

			localAngle = Math.round(angle);
			radarAngle = localAngle;

			// Update just the polygon shape without recreating marker
			const newLeftBearingRad = ((localAngle - fovWidthDegrees / 2) * Math.PI) / 180;
			const newRightBearingRad = ((localAngle + fovWidthDegrees / 2) * Math.PI) / 180;

			const newLeftLat = latitude + Math.cos(newLeftBearingRad) * fovDistanceLat;
			const newLeftLng = longitude + Math.sin(newLeftBearingRad) * fovDistanceLng;
			const newRightLat = latitude + Math.cos(newRightBearingRad) * fovDistanceLat;
			const newRightLng = longitude + Math.sin(newRightBearingRad) * fovDistanceLng;

			// Update polygon coordinates
			fovPolygon.setLatLngs([
				[latitude, longitude],
				[newLeftLat, newLeftLng],
				[newRightLat, newRightLng]
			]);
		});

		// Clear flag when drag ends
		fovTipMarker.on('dragend', () => {
			isDraggingFovTip = false;
			activateIncludeMap();
		});
	}

	function addBoundingBox(bounds: LatLngBounds) {
		if (!L || !map) return;

		// Remove existing rectangle
		if (bboxRect) {
			map.removeLayer(bboxRect);
		}

		// Create draggable rectangle
		bboxRect = L.rectangle(bounds, {
			color: '#f59e0b',
			weight: 2,
			fillOpacity: 0.1
		}).addTo(map);

		// Make the rectangle editable by listening to map clicks within bounds
		bboxRect.on('click', () => {
			if (!bboxRect) return;
			// Enable manual editing by allowing corner dragging
			// This is a simplified version - full edit mode would need a library like Leaflet.draw
		});

		// Update coordinates from bounds
		const sw = bounds.getSouthWest();
		const ne = bounds.getNorthEast();
		bboxSWLat = sw.lat;
		bboxSWLng = sw.lng;
		bboxNELat = ne.lat;
		bboxNELng = ne.lng;
	}

	function updateBBoxAroundRadar(maintainSize: boolean = false) {
		if (!L || !latitude || !longitude) return;

		let heightDelta: number;
		let widthDelta: number;

		if (maintainSize && bboxNELat && bboxSWLat && bboxNELng && bboxSWLng) {
			// Maintain current size
			heightDelta = (bboxNELat - bboxSWLat) / 2;
			widthDelta = (bboxNELng - bboxSWLng) / 2;
		} else {
			// Create new bbox with 3:2 landscape ratio (width:height)
			// Account for longitude compression at given latitude
			const metersPerDegreeLat = 111320;
			const metersPerDegreeLng = 111320 * Math.cos((latitude * Math.PI) / 180);

			const heightMeters = 300; // 300m height
			const widthMeters = 450; // 450m width (3:2 ratio)

			heightDelta = heightMeters / metersPerDegreeLat / 2;
			widthDelta = widthMeters / metersPerDegreeLng / 2;
		}

		const bounds = L.latLngBounds(
			[latitude - heightDelta, longitude - widthDelta],
			[latitude + heightDelta, longitude + widthDelta]
		);
		addBoundingBox(bounds);
	}

	function centerOnRadar() {
		if (!map || !latitude || !longitude) return;
		map.setView([latitude, longitude], 15);
	}

	// Update FOV triangle when radar position changes (not during drag)
	$: if (map && latitude !== null && longitude !== null && !isDraggingFovTip) {
		updateFOVTriangle();
	}

	async function searchAddress() {
		if (!searchQuery.trim()) return;

		searching = true;
		error = '';
		searchResults = [];

		try {
			// Nominatim Usage Policy: max 1 request per second
			const now = Date.now();
			const timeSinceLastSearch = now - lastSearchTime;
			if (timeSinceLastSearch < 1000) {
				// Wait to comply with rate limit
				await new Promise((resolve) => setTimeout(resolve, 1000 - timeSinceLastSearch));
			}
			lastSearchTime = Date.now();

			// Use Nominatim (OpenStreetMap geocoding service)
			const response = await fetch(
				`https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(searchQuery)}&limit=5`,
				{
					headers: {
						'User-Agent': 'velocity.report-map-editor'
					}
				}
			);

			if (!response.ok) {
				throw new Error('Could not search for that address.');
			}

			searchResults = await response.json();

			if (searchResults.length === 0) {
				error = 'No results found. Try a different search query.';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not search for that address.';
			console.error('Address search error:', e);
		} finally {
			searching = false;
		}
	}

	function selectLocation(result: { lat: string; lon: string; display_name: string }) {
		const lat = parseFloat(result.lat);
		const lng = parseFloat(result.lon);

		latitude = lat;
		longitude = lng;

		searchResults = [];
		searchQuery = '';

		if (map && radarMarker && L) {
			// Invalidate first — clearing searchResults above changes layout, so
			// Leaflet needs to recalculate its container size before panning.
			map.invalidateSize();
			radarMarker.setLatLng([lat, lng]);
			map.setView([lat, lng], 15);
			// Only update bbox if it doesn't exist yet
			if (!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng) {
				updateBBoxAroundRadar(false);
			} else {
				updateBBoxAroundRadar(true); // Maintain size when moving to searched location
			}
		}

		activateIncludeMap();
	}

	function adjustBBoxSize(increase: boolean) {
		if (!L || !latitude || !longitude) return;

		const currentHeightDelta = bboxNELat && latitude ? Math.abs(bboxNELat - latitude) : 0.003;
		const newHeightDelta = increase ? currentHeightDelta * 1.5 : currentHeightDelta / 1.5;
		const newWidthDelta = newHeightDelta * 1.5; // Maintain 3:2 ratio

		const bounds = L.latLngBounds(
			[latitude - newHeightDelta, longitude - newWidthDelta],
			[latitude + newHeightDelta, longitude + newWidthDelta]
		);
		addBoundingBox(bounds);
	}

	// Last mirror that successfully returned data.
	let activeMirrorId = '';

	// Fetch from Overpass with retry across mirror endpoints.
	// Tries mirrors in declaration order (selected mirror first).
	// A short connection timeout (8 s) moves quickly to the next mirror when
	// a server is unresponsive, but once response headers arrive the body
	// stream is read without a timeout so large payloads complete.
	async function fetchOverpassWithRetry(
		query: string,
		maxRetries: number = 1,
		signal?: AbortSignal
	): Promise<{ elements: Array<Record<string, unknown>> }> {
		const CONNECTION_TIMEOUT_MS = 8_000;
		const endpoints = orderedEndpoints(selectedMirror);
		let lastError: Error | null = null;

		for (const ep of endpoints) {
			for (let attempt = 0; attempt <= maxRetries; attempt++) {
				if (attempt > 0) {
					const delay = 2000 * attempt;
					console.log(`Retrying ${ep.url} in ${delay / 1000}s (attempt ${attempt + 1})...`);
					await new Promise((resolve) => setTimeout(resolve, delay));
				}

				// Per-request abort: fires after CONNECTION_TIMEOUT_MS unless
				// the caller's signal fires first.
				const timeoutCtrl = new AbortController();
				const timeoutId = setTimeout(() => timeoutCtrl.abort(), CONNECTION_TIMEOUT_MS);
				// If the caller aborts, forward to our per-request controller.
				const onCallerAbort = () => timeoutCtrl.abort();
				signal?.addEventListener('abort', onCallerAbort, { once: true });

				try {
					const response = await fetch(ep.url, {
						method: 'POST',
						body: `data=${encodeURIComponent(query)}`,
						signal: timeoutCtrl.signal
					});

					// Headers arrived — cancel the connection timeout so the
					// body stream can complete without being killed.
					clearTimeout(timeoutId);

					if (response.ok) {
						const contentType = response.headers.get('content-type') || '';
						if (contentType.includes('text/html')) {
							const body = await response.text();
							throw new Error(
								body.includes('timeout')
									? 'Server too busy (timeout). Trying next endpoint...'
									: `Unexpected HTML response from ${ep.url}`
							);
						}
						activeMirrorId = ep.id;
						return await response.json();
					}

					// 429 (rate limit) or 504 (gateway timeout) — worth retrying
					if (response.status === 429 || response.status === 504) {
						lastError = new Error(`${ep.url}: HTTP ${response.status}`);
						continue;
					}

					// Other errors — skip to next endpoint
					lastError = new Error(`${ep.url}: HTTP ${response.status}`);
					break;
				} catch (e) {
					clearTimeout(timeoutId);
					lastError = e instanceof Error ? e : new Error(String(e));
					// Caller cancelled — propagate immediately.
					if (signal?.aborted) throw lastError;
					if (
						lastError.name === 'AbortError' ||
						lastError.message.includes('timeout') ||
						lastError.message.includes('fetch') ||
						lastError.message.includes('HTTP 429') ||
						lastError.message.includes('HTTP 504')
					) {
						continue;
					}
					break;
				} finally {
					signal?.removeEventListener('abort', onCallerAbort);
				}
			}
			console.warn(`Overpass endpoint ${ep.url} failed, trying next...`);
		}

		throw new Error(
			lastError?.message || 'All Overpass API endpoints failed. Please try again later.'
		);
	}

	async function downloadMapSVG() {
		if (!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng) {
			error = 'Please set bounding box coordinates first';
			return;
		}

		downloading = true;
		error = '';
		abortController = new AbortController();
		const { signal } = abortController;

		try {
			const bbox = `${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng}`;

			const { essentialQuery, enrichmentQuery } = buildOverpassQueries(bbox);

			console.log('Fetching essential map data (roads + buildings)...');
			downloadStep = '1 🛣️ Roads…';
			const essentialData = await fetchOverpassWithRetry(essentialQuery, 1, signal);

			// Enrichment is best-effort: failures produce a sparser but still useful map.
			let enrichmentData: { elements: Array<Record<string, unknown>> } = { elements: [] };
			try {
				console.log('Fetching enrichment data (landuse, water, railways)...');
				downloadStep = '2 ⛰️ Detail…';
				enrichmentData = await fetchOverpassWithRetry(enrichmentQuery, 1, signal);
			} catch (enrichErr) {
				console.warn(
					'Enrichment query failed — proceeding with roads and buildings only.',
					enrichErr
				);
			}

			downloadStep = '3 ⚙️ Render…';

			const allElements = [...essentialData.elements, ...enrichmentData.elements];
			const nodeLookup = buildNodeLookup(allElements);
			const categorised = categoriseElements(allElements, nodeLookup);

			console.log(
				`Found: ${categorised.roads.length} roads, ${categorised.buildings.length} buildings, ${categorised.landuse.length} landuse, ${categorised.water.length} water, ${categorised.railways.length} railways, ${categorised.poiLabels.length} POIs`
			);

			const svgText = generateMapSvg({
				...categorised,
				bboxNELat: bboxNELat!,
				bboxNELng: bboxNELng!,
				bboxSWLat: bboxSWLat!,
				bboxSWLng: bboxSWLng!,
				latitude,
				longitude,
				radarAngle
			});

			mapSvgData = svgToBase64(svgText);
			mapJustDownloaded = true;
			error = '';
			activateIncludeMap();
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') {
				console.log('Download cancelled by user.');
			} else {
				error = e instanceof Error ? e.message : 'Could not download the map.';
				console.error('Map download error:', e);
			}
		} finally {
			downloading = false;
			downloadStep = '';
			abortController = null;
		}
	}

	function cancelDownload() {
		if (abortController) {
			abortController.abort();
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			searchAddress();
		}
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<p class="text-surface-600-300-token text-sm">
			Search for an address, or upload your own SVG map, then visually adjust the radar position.
		</p>
		<div class="flex items-center gap-3">
			<input
				bind:this={fileInput}
				type="file"
				accept=".svg,image/svg+xml"
				class="hidden"
				on:change={handleSvgUpload}
				on:cancel={handleFileInputCancel}
			/>
			{#key toggleResetKey}
				<ToggleGroup
					bind:value={mapMode}
					on:change={(e) => {
						const target = e.detail.value;
						if ((target === 'upload') === useCustomSvg) return;
						requestModeSwitch(target);
					}}
					variant="outline"
					rounded="full"
					inset
					classes={{
						indicator: 'h-full w-full bg-primary rounded-full [grid-column:1] [grid-row:1]',
						option: '[grid-column:1] [grid-row:1] z-[1]',
						label: '[&.selected]:text-white'
					}}
				>
					<ToggleOption value="interactive">Interactive</ToggleOption>
					<ToggleOption value="upload">Upload</ToggleOption>
				</ToggleGroup>
			{/key}
			<span class="text-sm font-medium">Include</span>
			<Switch bind:checked={includeMap} />
		</div>
	</div>

	{#if useCustomSvg && mapSvgData}
		<!-- Interactive SVG preview (custom upload) -->
		<div class="space-y-4">
			<h4 class="font-medium">Uploaded Map Preview</h4>
			<!-- svelte-ignore a11y-click-events-have-key-events -->
			<!-- svelte-ignore a11y-no-static-element-interactions -->
			<div
				bind:this={svgPreviewContainer}
				class="relative cursor-crosshair overflow-hidden rounded border border-gray-300 bg-white p-2"
				on:click={handleSvgPreviewClick}
			>
				<img
					src="data:image/svg+xml;base64,{mapSvgData}"
					alt="Uploaded map SVG"
					class="h-auto max-h-[500px] w-full object-contain"
					draggable="false"
				/>
				{#if radarSvgX !== null && radarSvgY !== null}
					<!-- FOV triangle overlay -->
					{@const angle = ((radarAngle ?? 0) * Math.PI) / 180}
					{@const fovHalf = (10 * Math.PI) / 180}
					{@const tipLen = 15}
					{@const leftAngle = angle - fovHalf}
					{@const rightAngle = angle + fovHalf}
					{@const tipX = radarSvgX + Math.sin(angle) * tipLen}
					{@const tipY = radarSvgY - Math.cos(angle) * tipLen}
					{@const leftX = radarSvgX + Math.sin(leftAngle) * tipLen}
					{@const leftY = radarSvgY - Math.cos(leftAngle) * tipLen}
					{@const rightX = radarSvgX + Math.sin(rightAngle) * tipLen}
					{@const rightY = radarSvgY - Math.cos(rightAngle) * tipLen}
					<svg
						class="pointer-events-none absolute inset-0 h-full w-full"
						preserveAspectRatio="none"
						viewBox="0 0 100 100"
					>
						<polygon
							points="{radarSvgX},{radarSvgY} {leftX},{leftY} {rightX},{rightY}"
							fill="rgba(59,130,246,0.25)"
							stroke="#3b82f6"
							stroke-width="0.4"
						/>
						<!-- Tip dot (red, draggable) -->
						<circle cx={tipX} cy={tipY} r="1.2" fill="#ef4444" stroke="white" stroke-width="0.3" />
					</svg>
					<!-- Radar dot (blue, draggable) -->
					<!-- svelte-ignore a11y-no-static-element-interactions -->
					<div
						class="absolute h-4 w-4 -translate-x-1/2 -translate-y-1/2 cursor-grab rounded-full border-2 border-white bg-blue-500 shadow-md"
						style="left: {radarSvgX}%; top: {radarSvgY}%;"
						on:mousedown={startSvgDotDrag}
					/>
				{/if}
			</div>
			<p class="text-surface-600-300-token text-xs">
				{#if radarSvgX === null}
					Click on the map to place the radar position.
				{:else}
					Click to reposition. Drag the blue dot to adjust. Use the angle stepper below.
				{/if}
			</p>

			<!-- Angle stepper and remove button for custom SVG mode -->
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-4">
					<p class="text-surface-600-300-token text-sm">Angle</p>
					<NumberStepper
						bind:value={localAngle}
						step={1}
						class="w-32"
						on:change={(e) => setAngle(e.detail.value)}
					>
						<span slot="suffix">°</span>
					</NumberStepper>
				</div>
				<Button
					variant="outline"
					color="danger"
					icon={mdiDelete}
					on:click={() => (showReplaceMapModal = true)}
				>
					Remove Map
				</Button>
			</div>
		</div>
	{:else}
		<!-- Address Search -->
		<div class="space-y-2">
			<h4 class="flex items-center gap-2 font-medium">
				<span class="text-primary-500">
					<svg class="h-5 w-5" viewBox="0 0 24 24">
						<path fill="currentColor" d={mdiMagnify} />
					</svg>
				</span>
				Address Search
			</h4>
			<div class="flex gap-2">
				<div class="flex-1">
					<TextField
						bind:value={searchQuery}
						placeholder="Enter address or location..."
						on:keydown={handleKeydown}
					/>
				</div>
				<Button on:click={searchAddress} disabled={searching || !searchQuery.trim()}>
					{searching ? 'Searching...' : 'Search'}
				</Button>
			</div>

			{#if searchResults.length > 0}
				<div class="bg-surface-100 border-surface-content/10 mt-2 rounded border">
					{#each searchResults as result (result.place_id)}
						<button
							class="text-surface-content hover:bg-surface-content/5 border-surface-content/10 block w-full border-b px-4 py-2 text-left text-sm last:border-b-0"
							on:click={() => selectLocation(result)}
						>
							{result.display_name}
						</button>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Interactive Map -->
		<div class="space-y-2">
			<div class="flex items-center justify-between">
				<h4 class="font-medium">Interactive Map</h4>
				<div class="flex gap-2">
					<Button size="sm" variant="outline" on:click={centerOnRadar} disabled={!latitude}>
						<svg class="h-4 w-4" viewBox="0 0 24 24">
							<path fill="currentColor" d={mdiCrosshairsGps} />
						</svg>
						Center
					</Button>
					<Button size="sm" variant="outline" on:click={() => adjustBBoxSize(false)}>- Area</Button>
					<Button size="sm" variant="outline" on:click={() => adjustBBoxSize(true)}>+ Area</Button>
				</div>
			</div>

			<div
				bind:this={mapContainer}
				class="h-96 w-full rounded border border-gray-300"
				style="min-height: 400px;"
			></div>

			<p class="text-surface-600-300-token text-xs">
				Drag the blue marker to set radar position. Drag the red dot at the triangle tip to adjust
				radar angle. The orange rectangle shows the map area for reports.
			</p>
		</div>

		<!-- Coordinate Display (Read-only) -->
		<div class="space-y-4">
			<h4 class="font-medium">Current Coordinates</h4>
			<div class="grid grid-cols-[auto_1fr] gap-4">
				<div class="max-w-xs">
					<p class="text-surface-600-300-token mb-1 text-sm">Radar Position</p>
					<div class="grid grid-cols-2 gap-2">
						<TextField label="Lat" value={latitude?.toFixed(6) || ''} disabled size="sm" />
						<TextField label="Lng" value={longitude?.toFixed(6) || ''} disabled size="sm" />
					</div>
					<div class="mt-2">
						<p class="text-surface-600-300-token mb-1 text-sm">Map Angle</p>
						<NumberStepper
							bind:value={localAngle}
							step={1}
							class="w-32"
							on:change={(e) => setAngle(e.detail.value)}
						>
							<span slot="suffix">°</span>
						</NumberStepper>
					</div>
				</div>
				<div>
					<p class="text-surface-600-300-token mb-1 text-sm">Bounding Box</p>
					<div class="grid grid-cols-2 gap-2">
						<TextField label="NE Lat" value={bboxNELat?.toFixed(6) || ''} disabled size="sm" />
						<TextField label="NE Lng" value={bboxNELng?.toFixed(6) || ''} disabled size="sm" />
						<TextField label="SW Lat" value={bboxSWLat?.toFixed(6) || ''} disabled size="sm" />
						<TextField label="SW Lng" value={bboxSWLng?.toFixed(6) || ''} disabled size="sm" />
					</div>
				</div>
			</div>
		</div>
	{/if}

	{#if !useCustomSvg}
		<!-- Download SVG -->
		<div class="space-y-2">
			<div class="flex items-center justify-between">
				<h4 class="font-medium">Download Map for Reports</h4>
				<div class="flex items-center gap-3">
					<div class="flex items-center gap-1.5">
						<SelectField
							bind:value={selectedMirror}
							options={[
								{ value: '', label: '🌐 Auto' },
								...OVERPASS_MIRRORS.map((m) => ({ value: m.id, label: `${m.flag} ${m.name}` }))
							]}
							clearable={false}
							classes={{ root: 'w-48' }}
							dense
							placeholder="Mirror"
						/>
						{#if activeMirrorId}
							{@const active = OVERPASS_MIRRORS.find((m) => m.id === activeMirrorId)}
							{#if active}
								<span class="text-surface-500-400-token text-xs" title="Last successful mirror"
									>→ {active.flag}</span
								>
							{/if}
						{/if}
					</div>
					{#if downloading}
						<span class="text-primary-500 flex items-center gap-1.5 text-sm">
							{downloadStep}
							<ProgressCircle size={20} width={2} />
						</span>
						<Button on:click={cancelDownload} variant="outline" color="danger" size="sm">
							Cancel
						</Button>
					{:else}
						<Button
							on:click={downloadMapSVG}
							disabled={!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng}
							icon={mdiDownload}
							variant="fill"
							color="primary"
							size="sm"
						>
							Download Map SVG
						</Button>
					{/if}
				</div>
			</div>
		</div>
	{/if}

	{#if error}
		<Notification
			color="danger"
			open
			icon={mdiAlert}
			classes={{
				root: 'bg-red-50 border-red-200 dark:bg-red-950 dark:border-red-800',
				title: 'text-red-800 dark:text-red-200',
				description: 'text-red-700 dark:text-red-300'
			}}
		>
			<span slot="title">Error</span>
			<span slot="description">{error}</span>
		</Notification>
	{/if}

	{#if mapJustDownloaded}
		<Notification
			color="success"
			open
			icon={mdiCheckCircle}
			classes={{
				root: 'bg-green-50 border-green-200 dark:bg-green-950 dark:border-green-800',
				title: 'text-green-800 dark:text-green-200',
				description: 'text-green-700 dark:text-green-300'
			}}
		>
			<span slot="title">Map Ready</span>
			<span slot="description"
				>Click <strong>Save Changes</strong> below to persist to the database.</span
			>
		</Notification>
	{/if}
</div>

<!-- Confirmation modal: warn before discarding existing map data -->
<Dialog
	bind:open={showDeleteMapModal}
	on:close={cancelModeSwitch}
	aria-modal="true"
	role="alertdialog"
	classes={{ dialog: 'max-w-sm' }}
>
	<div slot="title" class="flex items-center justify-between">
		<span>Replace Existing Map?</span>
		<button
			class="text-surface-500 hover:text-surface-700 -mt-1 -mr-2 p-1"
			on:click={cancelModeSwitch}
			aria-label="Close"
		>
			<svg class="h-5 w-5" viewBox="0 0 24 24"><path fill="currentColor" d={mdiClose} /></svg>
		</button>
	</div>

	<div class="space-y-3 px-6 pb-2">
		<p>
			This site already has map data. Switching modes will <strong>permanently delete</strong> the existing
			map image when you save.
		</p>
		<p class="text-surface-content/60 text-sm">This cannot be undone.</p>
	</div>

	<div slot="actions">
		<Button on:click={cancelModeSwitch} variant="outline">Cancel</Button>
		<Button
			on:click={() => {
				if (pendingModeSwitch) applyModeSwitch(pendingModeSwitch);
			}}
			variant="fill"
			color="danger"
		>
			Delete Map
		</Button>
	</div>
</Dialog>

<!-- Confirmation modal: warn before removing uploaded SVG -->
<Dialog
	bind:open={showReplaceMapModal}
	on:close={() => (showReplaceMapModal = false)}
	aria-modal="true"
	role="alertdialog"
	classes={{ dialog: 'max-w-sm' }}
>
	<div slot="title" class="flex items-center justify-between">
		<span>Remove Uploaded Map?</span>
		<button
			class="text-surface-500 hover:text-surface-700 -mt-1 -mr-2 p-1"
			on:click={() => (showReplaceMapModal = false)}
			aria-label="Close"
		>
			<svg class="h-5 w-5" viewBox="0 0 24 24"><path fill="currentColor" d={mdiClose} /></svg>
		</button>
	</div>

	<div class="space-y-3 px-6 pb-2">
		<p>
			This will <strong>permanently delete</strong> the current uploaded map image. You will be prompted
			to upload a replacement.
		</p>
		<p class="text-surface-content/60 text-sm">This cannot be undone.</p>
	</div>

	<div slot="actions">
		<Button on:click={() => (showReplaceMapModal = false)} variant="outline">Cancel</Button>
		<Button on:click={confirmReplaceMap} variant="fill" color="danger">Remove Map</Button>
	</div>
</Dialog>

<style>
	:global(.leaflet-container) {
		font-family: inherit;
	}
</style>
