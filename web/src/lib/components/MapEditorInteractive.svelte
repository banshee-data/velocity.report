<script lang="ts">
	import { svgToBase64 } from '$lib/map-svg';
	import 'leaflet/dist/leaflet.css';
	import {
		mdiAlert,
		mdiCheckCircle,
		mdiClose,
		mdiCrosshairsGps,
		mdiDelete,
		mdiDownload,
		mdiEye,
		mdiEyeOff,
		mdiMap
	} from '@mdi/js';
	import type { LatLngBounds, Map as LeafletMap, Marker, Rectangle, TileLayer } from 'leaflet';
	import { onDestroy, onMount, tick } from 'svelte';
	import {
		Button,
		Dialog,
		Notification,
		NumberStepper,
		ProgressCircle,
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
	let osmTileLayer: TileLayer | null = null;
	let fovPolygon: L.Polygon | null = null;
	let fovTipMarker: Marker | null = null;
	let mapContainer: HTMLElement;
	let downloading = false;
	let downloadStep = ''; // e.g. '1/2 Roads…', '2/2 Detail…'
	let error = '';
	let abortController: AbortController | null = null;
	let L: typeof import('leaflet') | null = null;
	let isDraggingFovTip = false; // Flag to prevent reactive updates during drag
	let mapJustDownloaded = false; // Track if map was just downloaded (not loaded from DB)

	// Confirmation modal state for mode switching
	let showDeleteMapModal = false;
	let pendingModeSwitch: 'interactive' | 'upload' | null = null;
	let toggleResetKey = 0;

	// Confirmation modal state for replacing an uploaded SVG
	let showReplaceMapModal = false;

	type ExternalMapRequest = 'map-tiles' | 'report-map-svg';
	let externalMapRequestConsent = false;
	let showExternalMapRequestModal = false;
	let pendingExternalMapRequest: ExternalMapRequest | null = null;
	let showReportMapPreview = false;
	// Bounds the current generated preview was built for. Used to detect when the
	// report bounds have changed since generation, which would make the preview
	// (and the SVG that gets saved) no longer match the orange bbox rectangle.
	let generatedBbox: { swLat: number; swLng: number; neLat: number; neLng: number } | null = null;

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

	function requestExternalMapRequest(request: ExternalMapRequest) {
		if (externalMapRequestConsent) {
			void runExternalMapRequest(request);
			return;
		}

		pendingExternalMapRequest = request;
		showExternalMapRequestModal = true;
	}

	async function runExternalMapRequest(request: ExternalMapRequest) {
		// Each branch performs exactly one external action, only after consent.
		if (request === 'map-tiles') {
			addOsmTileLayer();
			return;
		}
		await downloadTileMapSVG();
	}

	function confirmExternalMapRequest() {
		const request = pendingExternalMapRequest;
		externalMapRequestConsent = true;
		showExternalMapRequestModal = false;
		pendingExternalMapRequest = null;
		if (request) void runExternalMapRequest(request);
	}

	function cancelExternalMapRequest() {
		showExternalMapRequestModal = false;
		pendingExternalMapRequest = null;
	}

	function requestReportMapSvg() {
		requestExternalMapRequest('report-map-svg');
	}

	function requestMapTiles() {
		requestExternalMapRequest('map-tiles');
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
			showReportMapPreview = true;
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

		// OSM tiles are added only after explicit per-session consent.

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

	function addOsmTileLayer() {
		// Hard privacy gate: OSM tiles are an external fetch. Never load them
		// without this-session consent (granted via the modal), regardless of which
		// caller invokes this — the gate lives here so no path can leak a fetch.
		if (!L || !map || osmTileLayer || !externalMapRequestConsent) return;

		osmTileLayer = L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			attribution: '© OpenStreetMap contributors',
			maxZoom: 19
		}).addTo(map);
	}

	$: if (map && externalMapRequestConsent) addOsmTileLayer();

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

	// A generated preview is only valid for the bounds it was generated with.
	// When the report bounds change afterwards (drag, resize, or a new search),
	// flag the preview as stale so the user knows to regenerate. We never
	// silently re-contact external services to refresh it.
	$: reportMapStale =
		mapJustDownloaded &&
		generatedBbox !== null &&
		(bboxSWLat !== generatedBbox.swLat ||
			bboxSWLng !== generatedBbox.swLng ||
			bboxNELat !== generatedBbox.neLat ||
			bboxNELng !== generatedBbox.neLng);

	function adjustBBoxSize(increase: boolean) {
		if (!L || !latitude || !longitude) return;

		const currentHeightDelta = bboxNELat && latitude ? Math.abs(bboxNELat - latitude) : 0.003;
		const newHeightDelta = increase ? currentHeightDelta * 1.5 : currentHeightDelta / 1.5;
		// Maintain a 3:2 ratio in ground metres (not degrees): longitude degrees
		// are compressed by cos(lat), so the degree-space width must be divided by
		// cos(lat). This matches updateBBoxAroundRadar and keeps the box 3:2 in
		// metres so the fixed 1200×800 report canvas renders it without stretching.
		const lngCompression = Math.cos((latitude * Math.PI) / 180);
		const newWidthDelta = (newHeightDelta * 1.5) / lngCompression;

		const bounds = L.latLngBounds(
			[latitude - newHeightDelta, longitude - newWidthDelta],
			[latitude + newHeightDelta, longitude + newWidthDelta]
		);
		addBoundingBox(bounds);
	}

	// Tiles style: stitch the OSM raster tiles covering the report bbox into a
	// single PNG and wrap it in an SVG, so the saved artifact (and the PDF) match
	// the OSM-tile base shown in the editor. Done entirely client-side — the PDF
	// just embeds the saved SVG, so no server renderer is involved. Tiles are
	// fetched with CORS so the canvas stays untainted and exportable.
	async function downloadTileMapSVG() {
		// Defensive privacy gate: tile fetches are external.
		if (!externalMapRequestConsent) return;
		if (!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng) {
			error = 'Please set bounding box coordinates first';
			return;
		}

		downloading = true;
		error = '';
		abortController = new AbortController();
		const { signal } = abortController;

		try {
			downloadStep = 'Tiles...';
			const tileSize = 256;
			const spanLng = bboxNELng - bboxSWLng;
			// Pick a zoom so the report area is ~1200px wide (matches the vector
			// canvas), clamped to OSM's max zoom.
			let zoom = Math.round(Math.log2((1200 * 360) / (tileSize * spanLng)));
			zoom = Math.max(1, Math.min(19, zoom));

			const lon2tile = (lon: number) => ((lon + 180) / 360) * 2 ** zoom;
			const lat2tile = (lat: number) => {
				const r = (lat * Math.PI) / 180;
				return ((1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2) * 2 ** zoom;
			};
			const xMinF = lon2tile(bboxSWLng);
			const xMaxF = lon2tile(bboxNELng);
			const yMinF = lat2tile(bboxNELat); // north edge → smaller y
			const yMaxF = lat2tile(bboxSWLat);
			const xMin = Math.floor(xMinF);
			const xMax = Math.floor(xMaxF);
			const yMin = Math.floor(yMinF);
			const yMax = Math.floor(yMaxF);

			const full = document.createElement('canvas');
			full.width = (xMax - xMin + 1) * tileSize;
			full.height = (yMax - yMin + 1) * tileSize;
			const fctx = full.getContext('2d');
			if (!fctx) throw new Error('Canvas 2D is not available');

			// Fetch tiles via CORS, decode, and draw. fetch() supports the abort
			// signal, and a CORS-clean blob keeps the canvas exportable.
			const jobs: Promise<void>[] = [];
			for (let x = xMin; x <= xMax; x++) {
				for (let y = yMin; y <= yMax; y++) {
					const tx = x;
					const ty = y;
					jobs.push(
						(async () => {
							const resp = await fetch(`https://tile.openstreetmap.org/${zoom}/${tx}/${ty}.png`, {
								signal
							});
							if (!resp.ok) throw new Error(`Tile ${zoom}/${tx}/${ty}: HTTP ${resp.status}`);
							const bmp = await createImageBitmap(await resp.blob());
							fctx.drawImage(bmp, (tx - xMin) * tileSize, (ty - yMin) * tileSize);
							bmp.close();
						})()
					);
				}
			}
			await Promise.all(jobs);

			// Crop the stitched grid to exactly the report bbox.
			const cropX = (xMinF - xMin) * tileSize;
			const cropY = (yMinF - yMin) * tileSize;
			const cropW = Math.max(1, Math.round((xMaxF - xMinF) * tileSize));
			const cropH = Math.max(1, Math.round((yMaxF - yMinF) * tileSize));
			const out = document.createElement('canvas');
			out.width = cropW;
			out.height = cropH;
			const octx = out.getContext('2d');
			if (!octx) throw new Error('Canvas 2D is not available');
			octx.drawImage(full, cropX, cropY, cropW, cropH, 0, 0, cropW, cropH);

			const png = out.toDataURL('image/png');
			const svg =
				`<svg xmlns="http://www.w3.org/2000/svg" width="${cropW}" height="${cropH}" viewBox="0 0 ${cropW} ${cropH}">` +
				`<image href="${png}" width="${cropW}" height="${cropH}"/>` +
				`<rect x="${cropW - 232}" y="${cropH - 18}" width="228" height="14" fill="#ffffff" fill-opacity="0.75"/>` +
				`<text x="${cropW - 6}" y="${cropH - 7}" font-family="Arial, sans-serif" font-size="10" fill="#333333" text-anchor="end">© OpenStreetMap contributors</text>` +
				`</svg>`;

			mapSvgData = svgToBase64(svg);
			generatedBbox = {
				swLat: bboxSWLat!,
				swLng: bboxSWLng!,
				neLat: bboxNELat!,
				neLng: bboxNELng!
			};
			mapJustDownloaded = true;
			showReportMapPreview = true;
			activateIncludeMap();
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') {
				console.log('Tile download cancelled by user.');
			} else {
				error = e instanceof Error ? e.message : 'Could not build the tile map.';
				console.error('Tile map error:', e);
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
</script>

<div class="space-y-6">
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
		<p class="text-surface-600-300-token text-sm">
			Load OpenStreetMap tiles for this editor session, or upload your own SVG map for a no-network
			workflow, then visually adjust the radar position.
		</p>
		<!-- On mobile the Interactive/Upload toggle stacks above the Include
		     toggle; from sm: up they share a single row. -->
		<div class="flex flex-col items-start gap-3 sm:flex-row sm:items-center">
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
			<div class="flex items-center gap-2">
				<span class="text-sm font-medium">Include</span>
				<Switch bind:checked={includeMap} />
			</div>
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
					></div>
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

			<div class="relative">
				<div
					bind:this={mapContainer}
					class="h-96 w-full rounded border border-gray-300"
					style="min-height: 400px;"
				></div>

				{#if !externalMapRequestConsent && !showExternalMapRequestModal}
					<div class="pointer-events-none absolute inset-0 flex items-center justify-center p-4">
						<div
							class="bg-surface-100 text-surface-content border-surface-content/20 pointer-events-auto max-w-md rounded border p-4 text-sm shadow-lg"
						>
							<div class="mb-2 flex items-center gap-2 font-semibold">
								<svg class="h-5 w-5" viewBox="0 0 24 24">
									<path fill="currentColor" d={mdiMap} />
								</svg>
								Map Tiles Not Loaded
							</div>
							<p class="text-surface-content/70 mb-3">
								Load OpenStreetMap tiles for this editor session to position the radar and build the
								report map snapshot. This sends the map area to external tile servers.
							</p>
							<Button size="sm" variant="fill" color="primary" on:click={requestMapTiles}>
								Load Map Tiles
							</Button>
						</div>
					</div>
				{/if}
			</div>

			{#if !externalMapRequestConsent}
				<p class="text-surface-600-300-token text-xs">
					Tile requests are optional. Use Upload if this app should not make external map requests.
				</p>
			{/if}

			<p class="text-surface-600-300-token text-xs">
				Drag the blue marker to set radar position. Drag the red dot at the triangle tip to adjust
				radar angle. The orange rectangle shows the map area for reports.
			</p>
		</div>

		<!-- Coordinate Display (Read-only) -->
		<!--
			Responsive layout: the Radar Position and Bounding Box blocks stack
			vertically on narrow viewports (readable on mobile) and sit side by
			side from md: up so they use the available desktop width.
		-->
		<div class="space-y-4">
			<h4 class="font-medium">Current Coordinates</h4>
			<div class="grid max-w-2xl grid-cols-1 gap-4 md:grid-cols-2">
				<div>
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
		<!-- Generate report SVG -->
		<div class="space-y-2">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<h4 class="font-medium">Report Map Snapshot</h4>
				<div class="flex flex-wrap items-center gap-3">
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
							on:click={requestReportMapSvg}
							disabled={!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng}
							icon={mdiDownload}
							variant="fill"
							color="primary"
							size="sm"
						>
							Generate Tile Snapshot
						</Button>
					{/if}
				</div>
			</div>
			<p class="text-surface-content/60 text-xs">
				Generates a snapshot of the OpenStreetMap tiles covering the orange report area and saves it
				as the report map. Use Upload if this app should not make external map requests.
			</p>
		</div>
	{/if}

	{#if !useCustomSvg && mapSvgData}
		<div class="space-y-3">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<div>
					<h4 class="font-medium">
						{mapJustDownloaded
							? 'Generated Report Map Preview'
							: 'Existing Saved Report Map (Database)'}
					</h4>
					<p class="text-surface-600-300-token text-xs">
						{#if mapJustDownloaded && reportMapStale}
							<span class="text-amber-600 dark:text-amber-400">
								Report bounds changed since this map was generated — regenerate so the saved SVG
								matches the orange rectangle.
							</span>
						{:else if mapJustDownloaded}
							This SVG is in the editor state. Save changes to write it to
							<code>site.map_svg_data</code>.
						{:else}
							This is the current <code>site.map_svg_data</code> value loaded from the database. PDF reports
							embed this saved SVG until you generate and save a replacement.
						{/if}
					</p>
				</div>
				<Button
					size="sm"
					variant="outline"
					icon={showReportMapPreview ? mdiEyeOff : mdiEye}
					on:click={() => (showReportMapPreview = !showReportMapPreview)}
				>
					{showReportMapPreview ? 'Hide Preview' : 'Show Preview'}
				</Button>
			</div>
			{#if showReportMapPreview}
				<div class="overflow-hidden rounded border border-gray-300 bg-white p-2">
					<img
						src="data:image/svg+xml;base64,{mapSvgData}"
						alt={mapJustDownloaded
							? 'Generated report map SVG'
							: 'Existing saved report map SVG from database'}
						class="h-auto max-h-[500px] w-full object-contain"
						draggable="false"
					/>
				</div>
			{/if}
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
			<span slot="title">Map error</span>
			<span slot="description">{error}</span>
		</Notification>
	{/if}

	{#if mapJustDownloaded && reportMapStale}
		<Notification
			open
			icon={mdiAlert}
			class="border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-100"
		>
			<span slot="title">Report bounds changed</span>
			<span slot="description"
				>This preview was generated for a different map area. Regenerate the report map so it
				matches the current orange rectangle before saving.</span
			>
		</Notification>
	{:else if mapJustDownloaded}
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
				>The Report Map Preview is ready. Click <strong>Save Changes</strong> to keep it.</span
			>
		</Notification>
	{/if}
</div>

{#if showExternalMapRequestModal}
	<div
		class="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70 p-4"
		role="presentation"
	>
		<div
			class="w-full max-w-md rounded border border-neutral-700 bg-neutral-950 p-6 text-white shadow-2xl"
			role="alertdialog"
			aria-modal="true"
			aria-labelledby="map-tile-request-title"
		>
			<div class="mb-4 flex items-center justify-between gap-3">
				<h3 id="map-tile-request-title" class="text-lg font-semibold">Allow Map Tile Requests?</h3>
				<button
					class="-mt-1 -mr-2 p-1 text-neutral-300 hover:text-white"
					on:click={cancelExternalMapRequest}
					aria-label="Close"
				>
					<svg class="h-5 w-5" viewBox="0 0 24 24"><path fill="currentColor" d={mdiClose} /></svg>
				</button>
			</div>

			<div class="space-y-3 text-sm text-neutral-100">
				<p>
					This editor will request OpenStreetMap raster tiles to display the map and generate the
					report map snapshot.
				</p>
				<p>The site coordinates and report map area may be sent to external tile servers.</p>
				<p>Radar observations, vehicle data, reports, and raw sensor data are not sent.</p>
				<p class="text-neutral-300">For a no-network workflow, cancel and use Upload instead.</p>
			</div>

			<div class="mt-6 flex justify-end gap-2">
				<Button on:click={cancelExternalMapRequest} variant="outline">Cancel</Button>
				<Button on:click={confirmExternalMapRequest} variant="fill" color="primary">
					Allow Tiles This Session
				</Button>
			</div>
		</div>
	</div>
{/if}

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
			Replace Map
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
