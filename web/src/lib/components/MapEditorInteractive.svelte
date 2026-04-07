<script lang="ts">
	import { mdiCrosshairsGps, mdiDownload, mdiMagnify } from '@mdi/js';
	import type { LatLngBounds, Map as LeafletMap, Marker, Rectangle } from 'leaflet';
	import { onDestroy, onMount } from 'svelte';
	import { Button, Switch, TextField } from 'svelte-ux';

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
	let error = '';
	let selectedMirror = '';
	let L: typeof import('leaflet') | null = null;
	let isDraggingFovTip = false; // Flag to prevent reactive updates during drag
	let lastSearchTime = 0; // Track last API call for rate limiting
	let mapJustDownloaded = false; // Track if map was just downloaded (not loaded from DB)

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
		if (radarAngle === null) radarAngle = 0; // Default to north

		radarMarker.on('dragend', () => {
			if (!radarMarker) return;
			const pos = radarMarker.getLatLng();
			latitude = pos.lat;
			longitude = pos.lng;
			updateBBoxAroundRadar(true); // true = maintain size
		});

		// Add bounding box if it exists
		if (bboxNELat && bboxNELng && bboxSWLat && bboxSWLng) {
			addBoundingBox(L.latLngBounds([bboxSWLat, bboxSWLng], [bboxNELat, bboxNELng]));
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

			radarAngle = Math.round(angle);

			// Update just the polygon shape without recreating marker
			const newLeftBearingRad = ((radarAngle - fovWidthDegrees / 2) * Math.PI) / 180;
			const newRightBearingRad = ((radarAngle + fovWidthDegrees / 2) * Math.PI) / 180;

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

		if (map && radarMarker && L) {
			radarMarker.setLatLng([lat, lng]);
			map.setView([lat, lng], 15);
			// Only update bbox if it doesn't exist yet
			if (!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng) {
				updateBBoxAroundRadar(false);
			} else {
				updateBBoxAroundRadar(true); // Maintain size when moving to searched location
			}
		}

		searchResults = [];
		searchQuery = '';
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

	// Road styling by type - width and colour (scaled for 1200×800 viewBox)
	function getRoadStyle(highway: string): {
		width: number;
		color: string;
		casing: number;
		dash?: string;
	} {
		const styles: Record<string, { width: number; color: string; casing: number; dash?: string }> =
			{
				motorway: { width: 8, color: '#e892a2', casing: 10 },
				motorway_link: { width: 6, color: '#e892a2', casing: 8 },
				trunk: { width: 7, color: '#f9b29c', casing: 9 },
				trunk_link: { width: 5, color: '#f9b29c', casing: 7 },
				primary: { width: 6, color: '#fcd6a4', casing: 8 },
				primary_link: { width: 4, color: '#fcd6a4', casing: 6 },
				secondary: { width: 5, color: '#f7fabf', casing: 7 },
				secondary_link: { width: 4, color: '#f7fabf', casing: 6 },
				tertiary: { width: 4, color: '#ffffff', casing: 6 },
				tertiary_link: { width: 3, color: '#ffffff', casing: 5 },
				residential: { width: 3, color: '#ffffff', casing: 5 },
				unclassified: { width: 3, color: '#ffffff', casing: 5 },
				living_street: { width: 3, color: '#ededed', casing: 5 },
				service: { width: 2, color: '#ffffff', casing: 3 },
				pedestrian: { width: 3, color: '#dddde8', casing: 5 },
				footway: { width: 1, color: '#fa8072', casing: 0, dash: '4,2' },
				cycleway: { width: 1, color: '#0000ff', casing: 0, dash: '4,2' },
				path: { width: 1, color: '#fa8072', casing: 0, dash: '2,2' },
				track: { width: 2, color: '#996600', casing: 0, dash: '6,3' }
			};
		return styles[highway] || { width: 2, color: '#cccccc', casing: 3 };
	}

	// Font size for labels based on road type (scaled for 1200×800 viewBox)
	function getLabelFontSize(highway: string): number {
		const sizes: Record<string, number> = {
			motorway: 18,
			motorway_link: 14,
			trunk: 18,
			trunk_link: 14,
			primary: 16,
			primary_link: 12,
			secondary: 14,
			secondary_link: 12,
			tertiary: 12,
			residential: 10,
			unclassified: 10
		};
		return sizes[highway] || 9;
	}

	// Calculate midpoint and angle of a path for label placement
	function getPathMidpoint(
		nodes: Array<{ lat: number; lon: number }>,
		latToY: (lat: number) => number,
		lngToX: (lng: number) => number
	): { x: number; y: number; angle: number } | null {
		if (nodes.length < 2) return null;

		// Find total length and midpoint
		let totalLength = 0;
		const segments: Array<{ x1: number; y1: number; x2: number; y2: number; length: number }> = [];

		for (let i = 0; i < nodes.length - 1; i++) {
			const x1 = lngToX(nodes[i].lon);
			const y1 = latToY(nodes[i].lat);
			const x2 = lngToX(nodes[i + 1].lon);
			const y2 = latToY(nodes[i + 1].lat);
			const length = Math.sqrt((x2 - x1) ** 2 + (y2 - y1) ** 2);
			segments.push({ x1, y1, x2, y2, length });
			totalLength += length;
		}

		// Find the segment containing the midpoint
		let runningLength = 0;
		const midLength = totalLength / 2;

		for (const seg of segments) {
			if (runningLength + seg.length >= midLength) {
				const t = (midLength - runningLength) / seg.length;
				const x = seg.x1 + t * (seg.x2 - seg.x1);
				const y = seg.y1 + t * (seg.y2 - seg.y1);
				let angle = (Math.atan2(seg.y2 - seg.y1, seg.x2 - seg.x1) * 180) / Math.PI;
				// Flip text if it would be upside down
				if (angle > 90) angle -= 180;
				if (angle < -90) angle += 180;
				return { x, y, angle };
			}
			runningLength += seg.length;
		}

		return null;
	}

	// Overpass mirror endpoints with display metadata.
	const OVERPASS_MIRRORS: Array<{ id: string; name: string; flag: string; url: string }> = [
		{ id: 'de', name: 'Germany', flag: '🇩🇪', url: 'https://overpass-api.de/api/interpreter' },
		{
			id: 'ch',
			name: 'Switzerland',
			flag: '🇨🇭',
			url: 'https://overpass.kumi.systems/api/interpreter'
		},
		{
			id: 'ru',
			name: 'Russia',
			flag: '🇷🇺',
			url: 'https://maps.mail.ru/osm/tools/overpass/api/interpreter'
		},
		{
			id: 'fr',
			name: 'France',
			flag: '🇫🇷',
			url: 'https://overpass.openstreetmap.fr/api/interpreter'
		},
		{
			id: 'at',
			name: 'Austria',
			flag: '🇦🇹',
			url: 'https://overpass.private.coffee/api/interpreter'
		}
	];

	// Last mirror that successfully returned data.
	let activeMirrorId = '';

	function orderedEndpoints(): Array<{ id: string; url: string }> {
		const selected = selectedMirror ? OVERPASS_MIRRORS.find((m) => m.id === selectedMirror) : null;
		const rest = OVERPASS_MIRRORS.filter((m) => m.id !== selectedMirror);
		// Shuffle the non-selected mirrors
		for (let i = rest.length - 1; i > 0; i--) {
			const j = Math.floor(Math.random() * (i + 1));
			[rest[i], rest[j]] = [rest[j], rest[i]];
		}
		const ordered = selected ? [selected, ...rest] : rest;
		return ordered.map((m) => ({ id: m.id, url: m.url }));
	}

	// Fetch from Overpass with retry across mirror endpoints.
	// If user has selected a mirror, try it first; otherwise shuffle.
	async function fetchOverpassWithRetry(
		query: string,
		maxRetries: number = 1
	): Promise<{ elements: Array<Record<string, unknown>> }> {
		const endpoints = orderedEndpoints();
		let lastError: Error | null = null;

		for (const ep of endpoints) {
			for (let attempt = 0; attempt <= maxRetries; attempt++) {
				if (attempt > 0) {
					const delay = 2000 * attempt;
					console.log(`Retrying ${ep.url} in ${delay / 1000}s (attempt ${attempt + 1})...`);
					await new Promise((resolve) => setTimeout(resolve, delay));
				}

				try {
					const response = await fetch(ep.url, {
						method: 'POST',
						body: `data=${encodeURIComponent(query)}`
					});

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
					lastError = e instanceof Error ? e : new Error(String(e));
					if (
						lastError.message.includes('timeout') ||
						lastError.message.includes('fetch') ||
						lastError.message.includes('HTTP 429') ||
						lastError.message.includes('HTTP 504')
					) {
						continue;
					}
					break;
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

		try {
			const bbox = `${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng}`;

			// --- Essential query: roads + buildings (the skeleton of any neighbourhood map) ---
			const essentialQuery = `
				[out:json][timeout:30];
				(
					way["building"](${bbox});
					way["highway"~"^(motorway|motorway_link|trunk|trunk_link|primary|primary_link|secondary|secondary_link|tertiary|tertiary_link|residential|unclassified|living_street|service|pedestrian|footway|cycleway|path|track)$"](${bbox});
				);
				out body;
				>;
				out skel qt;
			`;

			// --- Enrichment query: landuse, water, railways, place names ---
			// Kept deliberately lightweight — no amenity/tourism/historic node scans.
			const enrichmentQuery = `
				[out:json][timeout:30];
				(
					way["landuse"](${bbox});
					way["leisure"~"^(park|garden|playground|pitch|common)$"](${bbox});
					way["natural"~"^(wood|scrub|water|wetland|grassland|heath)$"](${bbox});
					way["water"](${bbox});
					way["waterway"~"^(river|stream|canal)$"](${bbox});
					way["railway"~"^(rail|light_rail|tram)$"](${bbox});
					node["place"~"^(suburb|neighbourhood|quarter|village|hamlet)$"]["name"](${bbox});
					node["amenity"~"^(school|place_of_worship|community_centre|library|hospital|clinic|kindergarten|college|university)$"]["name"](${bbox});
					node["office"]["name"](${bbox});
					node["shop"]["name"](${bbox});
					node["leisure"~"^(park|garden|playground|sports_centre)$"]["name"](${bbox});
					node["tourism"~"^(museum|gallery|hotel|attraction)$"]["name"](${bbox});
					node["historic"]["name"](${bbox});
					nwr["amenity"~"^(school|place_of_worship|community_centre|library|hospital|clinic|kindergarten|college|university)$"]["name"](${bbox});
				);
				out body;
				>;
				out skel qt;
			`;

			console.log('Fetching essential map data (roads + buildings)...');
			const essentialData = await fetchOverpassWithRetry(essentialQuery);

			// Enrichment is best-effort: failures produce a sparser but still useful map.
			let enrichmentData: { elements: Array<Record<string, unknown>> } = { elements: [] };
			try {
				console.log('Fetching enrichment data (landuse, water, railways)...');
				enrichmentData = await fetchOverpassWithRetry(enrichmentQuery);
			} catch (enrichErr) {
				console.warn(
					'Enrichment query failed — proceeding with roads and buildings only.',
					enrichErr
				);
			}

			// Merge both result sets
			const allElements = [...essentialData.elements, ...enrichmentData.elements];

			// Build node lookup
			const nodes: Record<number, { lat: number; lon: number }> = {};
			for (const element of allElements) {
				if (element.type === 'node') {
					nodes[element.id as number] = { lat: element.lat as number, lon: element.lon as number };
				}
			}

			// Categorise ways
			const buildings: Array<{ nodes: Array<{ lat: number; lon: number }> }> = [];
			const landuse: Array<{
				type: string;
				nodes: Array<{ lat: number; lon: number }>;
			}> = [];
			const water: Array<{
				isLine: boolean;
				nodes: Array<{ lat: number; lon: number }>;
			}> = [];
			const roads: Array<{
				highway: string;
				name?: string;
				bridge?: boolean;
				tunnel?: boolean;
				nodes: Array<{ lat: number; lon: number }>;
			}> = [];
			const railways: Array<{
				type: string;
				nodes: Array<{ lat: number; lon: number }>;
			}> = [];

			// Collect place and landmark labels from named elements
			const poiLabels: Array<{
				name: string;
				lat: number;
				lon: number;
				category: 'place' | 'landmark';
				type: string;
			}> = [];

			// De-duplicate by name+approximate position (Overpass can return same feature from nwr + node)
			const seenPoi = new Set<string>();

			for (const element of allElements) {
				const tags = (element.tags || {}) as Record<string, string>;

				// --- Collect named POI labels (place names, schools, churches, etc.) ---
				if (tags.name) {
					// Determine lat/lon: nodes have lat/lon directly;
					// ways use centroid of their resolved nodes
					let lat: number | undefined;
					let lon: number | undefined;
					if (element.type === 'node') {
						lat = element.lat as number;
						lon = element.lon as number;
					} else if (element.type === 'way' && element.nodes) {
						const wayCoords = (element.nodes as number[])
							.map((id: number) => nodes[id])
							.filter((n): n is { lat: number; lon: number } => !!n);
						if (wayCoords.length > 0) {
							lat = wayCoords.reduce((s, n) => s + n.lat, 0) / wayCoords.length;
							lon = wayCoords.reduce((s, n) => s + n.lon, 0) / wayCoords.length;
						}
					}

					if (lat !== undefined && lon !== undefined) {
						// Skip house numbers — we only want named landmarks and places
						const isHouseNumber =
							tags['addr:housenumber'] &&
							!tags.amenity &&
							!tags.shop &&
							!tags.office &&
							!tags.leisure &&
							!tags.tourism &&
							!tags.historic &&
							!tags.place;
						if (!isHouseNumber) {
							// De-duplicate: round to ~11m precision
							const key = `${tags.name}|${lat.toFixed(4)}|${lon.toFixed(4)}`;
							if (!seenPoi.has(key)) {
								seenPoi.add(key);
								if (tags.place) {
									poiLabels.push({
										name: tags.name,
										lat,
										lon,
										category: 'place',
										type: tags.place
									});
								} else if (
									tags.amenity ||
									tags.shop ||
									tags.office ||
									tags.leisure ||
									tags.tourism ||
									tags.historic
								) {
									const type =
										tags.amenity ||
										tags.shop ||
										tags.office ||
										tags.leisure ||
										tags.tourism ||
										tags.historic;
									poiLabels.push({ name: tags.name, lat, lon, category: 'landmark', type });
								}
							}
						}
					}
				}

				// --- Categorise ways for geometry rendering ---
				if (element.type !== 'way') continue;

				const wayNodes = (element.nodes as number[])
					?.map((id: number) => nodes[id])
					.filter((n: { lat: number; lon: number } | undefined) => n);

				if (!wayNodes || wayNodes.length < 2) continue;

				if (tags.railway) {
					railways.push({ type: tags.railway, nodes: wayNodes });
				} else if (tags.building) {
					buildings.push({ nodes: wayNodes });
				} else if (tags.natural === 'water' || tags.water) {
					water.push({ isLine: false, nodes: wayNodes });
				} else if (tags.waterway) {
					water.push({ isLine: true, nodes: wayNodes });
				} else if (tags.landuse || tags.leisure || tags.natural) {
					const type = tags.landuse || tags.leisure || tags.natural;
					landuse.push({ type, nodes: wayNodes });
				} else if (tags.highway) {
					roads.push({
						highway: tags.highway,
						name: tags.name,
						bridge: tags.bridge === 'yes',
						tunnel: tags.tunnel === 'yes',
						nodes: wayNodes
					});
				}
			}

			console.log(
				`Found: ${roads.length} roads, ${buildings.length} buildings, ${landuse.length} landuse, ${water.length} water, ${railways.length} railways, ${poiLabels.length} POIs`
			);

			// SVG dimensions (3:2 aspect ratio, high-res for PDF)
			const svgWidth = 1200;
			const svgHeight = 800;

			// Coordinate conversion functions
			const latRange = bboxNELat! - bboxSWLat!;
			const lngRange = bboxNELng! - bboxSWLng!;
			const latToY = (lat: number) => ((bboxNELat! - lat) / latRange) * svgHeight;
			const lngToX = (lng: number) => ((lng - bboxSWLng!) / lngRange) * svgWidth;

			// Calculate map scale (metres per pixel) for scaling elements
			const centerLat = (bboxNELat! + bboxSWLat!) / 2;
			const metersPerDegreeLng = 111320 * Math.cos((centerLat * Math.PI) / 180);
			const mapWidthMeters = lngRange * metersPerDegreeLng;
			const metersPerPixel = mapWidthMeters / svgWidth;

			// Helper to create polygon path
			const toPolygonPath = (nodeList: Array<{ lat: number; lon: number }>) => {
				return (
					nodeList
						.map(
							(n, i) =>
								`${i === 0 ? 'M' : 'L'} ${lngToX(n.lon).toFixed(1)} ${latToY(n.lat).toFixed(1)}`
						)
						.join(' ') + ' Z'
				);
			};

			// Helper to create line path
			const toLinePath = (nodeList: Array<{ lat: number; lon: number }>) => {
				return nodeList
					.map(
						(n, i) =>
							`${i === 0 ? 'M' : 'L'} ${lngToX(n.lon).toFixed(1)} ${latToY(n.lat).toFixed(1)}`
					)
					.join(' ');
			};

			// Landuse colours (OSM-style)
			const getLanduseColor = (type: string): string => {
				const colors: Record<string, string> = {
					// Green areas
					grass: '#cdebb0',
					forest: '#add19e',
					wood: '#add19e',
					park: '#c8facc',
					garden: '#cdebb0',
					meadow: '#cdebb0',
					recreation_ground: '#c8facc',
					village_green: '#c8facc',
					playground: '#c8facc',
					pitch: '#aae0cb',
					golf_course: '#b5e3b5',
					nature_reserve: '#c8facc',
					common: '#c8facc',
					scrub: '#c8d7ab',
					heath: '#d6d99f',
					grassland: '#cdebb0',
					// Brown/tan areas
					farmland: '#eef0d5',
					farmyard: '#f5dcba',
					orchard: '#aedfa3',
					vineyard: '#b3e2a8',
					allotments: '#c9e1bf',
					cemetery: '#aacbaf',
					// Wetland
					wetland: '#b5d0d0'
				};
				return colors[type] || '#e0e0e0';
			};

			// Generate landuse polygons
			let landusePaths = '';
			for (const area of landuse) {
				const color = getLanduseColor(area.type);
				landusePaths += `<path d="${toPolygonPath(area.nodes)}" fill="${color}" stroke="none"/>`;
			}

			// Generate water
			let waterPaths = '';
			for (const w of water) {
				if (w.isLine) {
					waterPaths += `<path d="${toLinePath(w.nodes)}" stroke="#aad3df" stroke-width="4" fill="none"/>`;
				} else {
					waterPaths += `<path d="${toPolygonPath(w.nodes)}" fill="#aad3df" stroke="#6699cc" stroke-width="1"/>`;
				}
			}

			// Generate buildings
			let buildingPaths = '';
			for (const bldg of buildings) {
				buildingPaths += `<path d="${toPolygonPath(bldg.nodes)}" fill="#d9d0c9" stroke="#bbb5b0" stroke-width="0.6"/>`;
			}

			// Sort roads by importance (draw minor roads first, major roads on top)
			const roadOrder = [
				'footway',
				'path',
				'cycleway',
				'track',
				'service',
				'pedestrian',
				'living_street',
				'unclassified',
				'residential',
				'tertiary_link',
				'tertiary',
				'secondary_link',
				'secondary',
				'primary_link',
				'primary',
				'trunk_link',
				'trunk',
				'motorway_link',
				'motorway'
			];
			roads.sort((a, b) => roadOrder.indexOf(a.highway) - roadOrder.indexOf(b.highway));

			// Generate road paths
			let roadPaths = '';
			for (const way of roads) {
				const style = getRoadStyle(way.highway);
				const pathData = toLinePath(way.nodes);

				// Draw road outline (casing) then fill
				if (style.casing > 0) {
					const casingColor = way.bridge ? '#000000' : '#666666';
					roadPaths += `<path d="${pathData}" stroke="${casingColor}" stroke-width="${style.casing}" fill="none" stroke-linecap="round" stroke-linejoin="round"/>`;
				}
				let roadAttrs = `stroke="${style.color}" stroke-width="${style.width}" fill="none" stroke-linecap="round" stroke-linejoin="round"`;
				if (style.dash) {
					roadAttrs += ` stroke-dasharray="${style.dash}"`;
				}
				if (way.tunnel) {
					roadAttrs += ` opacity="0.5"`;
				}
				roadPaths += `<path d="${pathData}" ${roadAttrs}/>`;
			}

			// Generate street name labels (deduplicated)
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			const labelledNames = new Set<string>();
			let labels = '';

			for (const way of roads) {
				if (!way.name || labelledNames.has(way.name)) continue;

				// Only label significant roads
				const labelRoads = [
					'motorway',
					'trunk',
					'primary',
					'secondary',
					'tertiary',
					'residential',
					'unclassified'
				];
				if (!labelRoads.includes(way.highway)) continue;

				const midpoint = getPathMidpoint(way.nodes, latToY, lngToX);
				if (!midpoint) continue;

				// Skip if midpoint is outside the visible area (with padding)
				if (
					midpoint.x < 10 ||
					midpoint.x > svgWidth - 10 ||
					midpoint.y < 10 ||
					midpoint.y > svgHeight - 10
				) {
					continue;
				}

				const fontSize = getLabelFontSize(way.highway);
				labelledNames.add(way.name);

				// Text with white halo for readability
				labels += `<text x="${midpoint.x.toFixed(1)}" y="${midpoint.y.toFixed(1)}" font-family="Arial, sans-serif" font-size="${fontSize}" fill="#333333" text-anchor="middle" dominant-baseline="middle" transform="rotate(${midpoint.angle.toFixed(1)}, ${midpoint.x.toFixed(1)}, ${midpoint.y.toFixed(1)})" stroke="white" stroke-width="2" paint-order="stroke">${escapeXml(way.name)}</text>`;
			}

			// Generate railway paths
			let railwayPaths = '';
			for (const rail of railways) {
				const pathData = toLinePath(rail.nodes);
				// White base, then dark dashed overlay (standard rail map style)
				railwayPaths += `<path d="${pathData}" stroke="#999999" stroke-width="4" fill="none" stroke-linecap="butt"/>`;
				railwayPaths += `<path d="${pathData}" stroke="#ffffff" stroke-width="2" fill="none" stroke-linecap="butt" stroke-dasharray="8,8"/>`;
			}

			// Generate POI/place labels
			let poiLabelPaths = '';
			for (const poi of poiLabels) {
				const x = lngToX(poi.lon);
				const y = latToY(poi.lat);

				// Skip if outside visible area
				if (x < 20 || x > svgWidth - 20 || y < 20 || y > svgHeight - 20) continue;

				if (poi.category === 'place') {
					// Place names: larger, italic
					const placeFontSize = poi.type === 'village' ? 14 : poi.type === 'hamlet' ? 12 : 11;
					poiLabelPaths += `<text x="${x.toFixed(1)}" y="${y.toFixed(1)}" font-family="Arial, sans-serif" font-size="${placeFontSize}" font-style="italic" fill="#444444" text-anchor="middle" dominant-baseline="middle" stroke="white" stroke-width="3" paint-order="stroke">${escapeXml(poi.name)}</text>`;
				} else {
					// Landmarks: small dot + label
					poiLabelPaths += `<circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="2" fill="#734a08"/>`;
					poiLabelPaths += `<text x="${(x + 4).toFixed(1)}" y="${y.toFixed(1)}" font-family="Arial, sans-serif" font-size="8" fill="#734a08" dominant-baseline="middle" stroke="white" stroke-width="2" paint-order="stroke">${escapeXml(poi.name)}</text>`;
				}
			}

			// Radar position and FOV triangle - scale based on map size
			const radarX = latitude !== null ? lngToX(longitude!) : svgWidth / 2;
			const radarY = latitude !== null ? latToY(latitude!) : svgHeight / 2;

			// Triangle size: 100 metres in real world, scaled to pixels
			const triangleLengthMeters = 100;
			const triangleLengthPixels = triangleLengthMeters / metersPerPixel;

			// FOV triangle (20 degree width)
			const angle = radarAngle || 0;
			const fovWidthDegrees = 20;
			const leftAngleRad = ((angle - fovWidthDegrees / 2) * Math.PI) / 180;
			const rightAngleRad = ((angle + fovWidthDegrees / 2) * Math.PI) / 180;

			// Triangle points (note: SVG y increases downward, so we flip sin)
			const leftX = radarX + Math.sin(leftAngleRad) * triangleLengthPixels;
			const leftY = radarY - Math.cos(leftAngleRad) * triangleLengthPixels;
			const rightX = radarX + Math.sin(rightAngleRad) * triangleLengthPixels;
			const rightY = radarY - Math.cos(rightAngleRad) * triangleLengthPixels;

			// Marker size also scales with map
			const markerRadius = Math.max(8, Math.min(20, 40 / metersPerPixel));

			// Create complete SVG
			const svgText = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${svgWidth}" height="${svgHeight}" viewBox="0 0 ${svgWidth} ${svgHeight}">
	<title>Map Export - velocity.report</title>
	<desc>Data © OpenStreetMap contributors</desc>
	<!-- Background -->
	<rect x="0" y="0" width="${svgWidth}" height="${svgHeight}" fill="#f2efe9"/>
	<!-- Landuse (parks, forests, etc.) -->
	${landusePaths}
	<!-- Water -->
	${waterPaths}
	<!-- Buildings -->
	${buildingPaths}
	<!-- Roads -->
	${roadPaths}
	<!-- Railways -->
	${railwayPaths}
	<!-- Street names -->
	${labels}
	<!-- Place names and POIs -->
	${poiLabelPaths}
	<!-- Radar FOV triangle -->
	<polygon points="${radarX.toFixed(1)},${radarY.toFixed(1)} ${leftX.toFixed(1)},${leftY.toFixed(1)} ${rightX.toFixed(1)},${rightY.toFixed(1)}" fill="#ef4444" fill-opacity="0.4" stroke="#ef4444" stroke-width="1"/>
	<!-- Radar position marker -->
	<circle cx="${radarX.toFixed(1)}" cy="${radarY.toFixed(1)}" r="${markerRadius.toFixed(1)}" fill="#3b82f6" stroke="white" stroke-width="2"/>
</svg>`;

			// Store as base64 (UTF-8 safe)
			// Convert to base64 in chunks to avoid stack overflow on large SVGs
			const encoder = new TextEncoder();
			const bytes = encoder.encode(svgText);

			// Process in chunks to avoid "Maximum call stack size exceeded"
			const chunkSize = 8192;
			let binaryString = '';
			for (let i = 0; i < bytes.length; i += chunkSize) {
				const chunk = bytes.slice(i, i + chunkSize);
				binaryString += String.fromCharCode(...chunk);
			}
			mapSvgData = btoa(binaryString);
			mapJustDownloaded = true;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not download the map.';
			console.error('Map download error:', e);
		} finally {
			downloading = false;
		}
	}

	// Escape special XML characters
	function escapeXml(str: string): string {
		return str
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;')
			.replace(/'/g, '&#39;');
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
			Search for an address, then adjust the radar position and map area visually.
		</p>
		<div class="flex items-center gap-3">
			<span class="text-sm font-medium">Include map in reports</span>
			<Switch bind:checked={includeMap} />
		</div>
	</div>

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
			<div class="mt-2 rounded border border-gray-300 bg-white">
				{#each searchResults as result (result.place_id)}
					<button
						class="block w-full px-4 py-2 text-left hover:bg-gray-100"
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
				<div class="mt-2 w-24">
					<TextField
						bind:value={radarAngle}
						label="Angle °"
						type="number"
						placeholder="0"
						size="sm"
					/>
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

	<!-- Download SVG -->
	<div class="space-y-2">
		<div class="flex items-center justify-between">
			<h4 class="font-medium">Download Map for Reports</h4>
			<div class="flex items-center gap-3">
				<label class="flex items-center gap-1.5 text-sm">
					<span class="text-surface-600-300-token">Mirror:</span>
					<select
						bind:value={selectedMirror}
						class="rounded border border-gray-300 bg-white px-2 py-1 text-sm"
					>
						<option value="">Auto</option>
						{#each OVERPASS_MIRRORS as mirror (mirror.id)}
							<option value={mirror.id}>{mirror.flag} {mirror.name}</option>
						{/each}
					</select>
					{#if activeMirrorId}
						{@const active = OVERPASS_MIRRORS.find((m) => m.id === activeMirrorId)}
						{#if active}
							<span class="text-surface-500-400-token text-xs" title="Last successful mirror"
								>→ {active.flag}</span
							>
						{/if}
					{/if}
				</label>
				<Button
					on:click={downloadMapSVG}
					disabled={!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng || downloading}
					icon={mdiDownload}
					variant="fill"
					color="primary"
					size="sm"
				>
					{downloading ? 'Downloading...' : 'Download Map SVG'}
				</Button>
			</div>
		</div>

		{#if error}
			<div role="alert" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-800">
				<strong>Error:</strong>
				{error}
			</div>
		{/if}

		{#if mapJustDownloaded}
			<div class="rounded border border-amber-300 bg-amber-50 p-3 text-sm text-amber-800">
				<strong>Map Ready:</strong>
				SVG downloaded. Click <strong>"Save Changes"</strong> below to persist to the database.
			</div>
		{/if}
	</div>
</div>

<style>
	:global(.leaflet-container) {
		font-family: inherit;
	}
</style>
