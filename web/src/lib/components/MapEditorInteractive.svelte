<script lang="ts">
	import { mdiCrosshairsGps, mdiDownload, mdiMagnify } from '@mdi/js';
	import type { LatLngBounds, Map as LeafletMap, Marker, Rectangle } from 'leaflet';
	import { onDestroy, onMount } from 'svelte';
	import { Button, Card, TextField } from 'svelte-ux';

	// Props
	export let latitude: number | null = null;
	export let longitude: number | null = null;
	export let radarAngle: number | null = null;
	export let bboxNELat: number | null = null;
	export let bboxNELng: number | null = null;
	export let bboxSWLat: number | null = null;
	export let bboxSWLng: number | null = null;
	export let mapSvgData: string | null = null;

	// Local state
	let map: LeafletMap | null = null;
	let radarMarker: Marker | null = null;
	let bboxRect: Rectangle | null = null;
	let mapContainer: HTMLElement;
	let searchQuery = '';
	let searchResults: Array<{ display_name: string; lat: string; lon: string }> = [];
	let searching = false;
	let downloading = false;
	let error = '';
	let L: typeof import('leaflet') | null = null;

	// Default location (London, UK)
	const defaultLat = 51.5074;
	const defaultLng = -0.1278;

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
			heightDelta = 0.003; // ~300m height
			widthDelta = heightDelta * 1.5; // 450m width for 3:2 ratio
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

	async function searchAddress() {
		if (!searchQuery.trim()) return;

		searching = true;
		error = '';
		searchResults = [];

		try {
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

	async function downloadMapSVG() {
		if (!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng) {
			error = 'Please set bounding box coordinates first';
			return;
		}

		downloading = true;
		error = '';

		try {
			const bbox = `${bboxSWLng},${bboxSWLat},${bboxNELng},${bboxNELat}`;
			const url = `https://render.openstreetmap.org/cgi-bin/export?bbox=${bbox}&scale=10000&format=svg`;

			const response = await fetch(url);
			if (!response.ok) {
				throw new Error(`Failed to download map: ${response.status} ${response.statusText}`);
			}

			const svgText = await response.text();

			if (!svgText.includes('<svg')) {
				throw new Error('Invalid SVG response from OpenStreetMap');
			}

			// Store as base64, handling UTF-8 characters properly
			const encoder = new TextEncoder();
			const utf8Bytes = encoder.encode(svgText);
			const base64String = btoa(String.fromCharCode.apply(null, Array.from(utf8Bytes)));
			mapSvgData = base64String;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to download map';
			console.error('Map download error:', e);
		} finally {
			downloading = false;
		}
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter') {
			searchAddress();
		}
	}
</script>

<Card>
	<div class="space-y-6 p-6">
		<div>
			<h3 class="mb-2 text-lg font-semibold">Map Configuration</h3>
			<p class="text-surface-600-300-token text-sm">
				Search for an address, then adjust the radar position and map area visually.
			</p>
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
				Drag the blue marker to set radar position. The orange rectangle shows the map area for
				reports.
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

			{#if mapSvgData}
				<div class="rounded border border-green-300 bg-green-50 p-3 text-sm text-green-800">
					<strong>Success:</strong>
					Map SVG downloaded and ready to save.
				</div>
			{/if}
		</div>
	</div>
</Card>

<style>
	:global(.leaflet-container) {
		font-family: inherit;
	}
</style>
