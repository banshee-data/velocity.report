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
	let searchResults: Array<{ display_name: string; lat: string; lon: string }> = [];
	let searching = false;
	let downloading = false;
	let error = '';
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
				throw new Error('Failed to search for address');
			}

			searchResults = await response.json();

			if (searchResults.length === 0) {
				error = 'No results found. Try a different search query.';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to search for address';
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

	// Road styling by type - width and colour
	function getRoadStyle(highway: string): { width: number; color: string } {
		const styles: Record<string, { width: number; color: string }> = {
			motorway: { width: 4, color: '#e892a2' },
			motorway_link: { width: 3, color: '#e892a2' },
			trunk: { width: 3.5, color: '#f9b29c' },
			trunk_link: { width: 2.5, color: '#f9b29c' },
			primary: { width: 3, color: '#fcd6a4' },
			primary_link: { width: 2, color: '#fcd6a4' },
			secondary: { width: 2.5, color: '#f7fabf' },
			secondary_link: { width: 2, color: '#f7fabf' },
			tertiary: { width: 2, color: '#ffffff' },
			tertiary_link: { width: 1.5, color: '#ffffff' },
			residential: { width: 1.5, color: '#ffffff' },
			unclassified: { width: 1.5, color: '#ffffff' },
			living_street: { width: 1.5, color: '#ededed' },
			service: { width: 1, color: '#ffffff' },
			pedestrian: { width: 1.5, color: '#dddde8' },
			footway: { width: 0.5, color: '#fa8072' },
			cycleway: { width: 0.5, color: '#0000ff' },
			path: { width: 0.5, color: '#fa8072' },
			track: { width: 1, color: '#996600' }
		};
		return styles[highway] || { width: 1, color: '#cccccc' };
	}

	// Font size for labels based on road type
	function getLabelFontSize(highway: string): number {
		const sizes: Record<string, number> = {
			motorway: 10,
			motorway_link: 8,
			trunk: 10,
			trunk_link: 8,
			primary: 9,
			primary_link: 7,
			secondary: 8,
			secondary_link: 7,
			tertiary: 7,
			residential: 6,
			unclassified: 6
		};
		return sizes[highway] || 5;
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

	async function downloadMapSVG() {
		if (!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng) {
			error = 'Please set bounding box coordinates first';
			return;
		}

		downloading = true;
		error = '';

		try {
			// Fetch map data from Overpass API - roads, buildings, landuse, water
			const overpassQuery = `
				[out:json][timeout:30];
				(
					// Landuse areas (parks, forests, grass, etc.)
					way["landuse"~"^(grass|forest|meadow|recreation_ground|village_green|orchard|vineyard|farmland|farmyard|allotments|cemetery)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					relation["landuse"~"^(grass|forest|meadow|recreation_ground|village_green|orchard|vineyard|farmland|farmyard|allotments|cemetery)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					// Leisure areas (parks, gardens, pitches)
					way["leisure"~"^(park|garden|playground|pitch|golf_course|nature_reserve|common)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					relation["leisure"~"^(park|garden|playground|pitch|golf_course|nature_reserve|common)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					// Natural areas
					way["natural"~"^(wood|scrub|heath|grassland|wetland|water)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					relation["natural"~"^(wood|scrub|heath|grassland|wetland|water)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					// Water bodies
					way["water"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					way["waterway"~"^(river|stream|canal|drain|ditch)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					// Buildings
					way["building"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
					// Roads
					way["highway"~"^(motorway|motorway_link|trunk|trunk_link|primary|primary_link|secondary|secondary_link|tertiary|tertiary_link|residential|unclassified|living_street|service|pedestrian|footway|path)$"](${bboxSWLat},${bboxSWLng},${bboxNELat},${bboxNELng});
				);
				out body;
				>;
				out skel qt;
			`;

			console.log('Fetching map data from Overpass API...');
			const response = await fetch('https://overpass-api.de/api/interpreter', {
				method: 'POST',
				body: `data=${encodeURIComponent(overpassQuery)}`
			});

			if (!response.ok) {
				throw new Error(`Overpass API error: ${response.status}`);
			}

			const data = await response.json();

			// Build node lookup
			const nodes: Record<number, { lat: number; lon: number }> = {};
			for (const element of data.elements) {
				if (element.type === 'node') {
					nodes[element.id] = { lat: element.lat, lon: element.lon };
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
				nodes: Array<{ lat: number; lon: number }>;
			}> = [];

			for (const element of data.elements) {
				if (element.type !== 'way') continue;

				const wayNodes = element.nodes
					?.map((id: number) => nodes[id])
					.filter((n: { lat: number; lon: number } | undefined) => n);

				if (!wayNodes || wayNodes.length < 2) continue;

				const tags = element.tags || {};

				if (tags.building) {
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
						nodes: wayNodes
					});
				}
			}

			console.log(
				`Found: ${roads.length} roads, ${buildings.length} buildings, ${landuse.length} landuse, ${water.length} water`
			);

			// SVG dimensions (3:2 aspect ratio)
			const svgWidth = 600;
			const svgHeight = 400;

			// Coordinate conversion functions
			const latRange = bboxNELat - bboxSWLat;
			const lngRange = bboxNELng - bboxSWLng;
			const latToY = (lat: number) => ((bboxNELat - lat) / latRange) * svgHeight;
			const lngToX = (lng: number) => ((lng - bboxSWLng) / lngRange) * svgWidth;

			// Calculate map scale (metres per pixel) for scaling elements
			const centerLat = (bboxNELat + bboxSWLat) / 2;
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
					waterPaths += `<path d="${toLinePath(w.nodes)}" stroke="#aad3df" stroke-width="2" fill="none"/>`;
				} else {
					waterPaths += `<path d="${toPolygonPath(w.nodes)}" fill="#aad3df" stroke="#6699cc" stroke-width="0.5"/>`;
				}
			}

			// Generate buildings
			let buildingPaths = '';
			for (const bldg of buildings) {
				buildingPaths += `<path d="${toPolygonPath(bldg.nodes)}" fill="#d9d0c9" stroke="#bbb5b0" stroke-width="0.3"/>`;
			}

			// Sort roads by importance (draw minor roads first, major roads on top)
			const roadOrder = [
				'footway',
				'path',
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
				roadPaths += `<path d="${pathData}" stroke="#666666" stroke-width="${style.width + 1}" fill="none" stroke-linecap="round" stroke-linejoin="round"/>`;
				roadPaths += `<path d="${pathData}" stroke="${style.color}" stroke-width="${style.width}" fill="none" stroke-linecap="round" stroke-linejoin="round"/>`;
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
			const markerRadius = Math.max(4, Math.min(10, 20 / metersPerPixel));

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
	<!-- Street names -->
	${labels}
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
			error = e instanceof Error ? e.message : 'Failed to download map';
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
		<div class="grid gap-4 md:grid-cols-2">
			<div>
				<p class="text-surface-600-300-token mb-1 text-sm">Radar Position</p>
				<div class="grid grid-cols-2 gap-2">
					<TextField label="Latitude" value={latitude?.toFixed(6) || ''} disabled size="sm" />
					<TextField label="Longitude" value={longitude?.toFixed(6) || ''} disabled size="sm" />
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

	<!-- Radar Angle -->
	<TextField
		bind:value={radarAngle}
		label="Radar Angle (degrees)"
		type="number"
		step="1"
		placeholder="0"
	/>

	<!-- Download SVG -->
	<div class="space-y-2">
		<div class="flex items-center justify-between">
			<h4 class="font-medium">Download Map for Reports</h4>
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
