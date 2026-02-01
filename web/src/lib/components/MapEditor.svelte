<script lang="ts">
	import { Button, Card, TextField } from 'svelte-ux';
	import { mdiMapMarker, mdiRotateRight, mdiDownload } from '@mdi/js';

	// Props
	export let latitude: number | null = null;
	export let longitude: number | null = null;
	export let radarAngle: number | null = null;
	export let bboxNELat: number | null = null;
	export let bboxNELng: number | null = null;
	export let bboxSWLat: number | null = null;
	export let bboxSWLng: number | null = null;
	export let mapRotation: number | null = null;
	export let mapSvgData: string | null = null;

	// Local state
	let downloading = false;
	let error = '';
	let previewUrl = '';

	// Calculate preview URL based on current settings
	$: {
		if (bboxNELat && bboxNELng && bboxSWLat && bboxSWLng) {
			// OpenStreetMap export API format: bbox=min_lon,min_lat,max_lon,max_lat
			const bbox = `${bboxSWLng},${bboxSWLat},${bboxNELng},${bboxNELat}`;
			previewUrl = `https://render.openstreetmap.org/cgi-bin/export?bbox=${bbox}&scale=10000&format=png`;
		} else if (latitude && longitude) {
			// Fallback: create a small bbox around the point
			const delta = 0.005; // ~500m
			const bbox = `${longitude - delta},${latitude - delta},${longitude + delta},${latitude + delta}`;
			previewUrl = `https://render.openstreetmap.org/cgi-bin/export?bbox=${bbox}&scale=10000&format=png`;
		}
	}

	function setDefaultBBox() {
		if (latitude && longitude) {
			const delta = 0.005; // ~500m area
			bboxNELat = latitude + delta;
			bboxNELng = longitude + delta;
			bboxSWLat = latitude - delta;
			bboxSWLng = longitude - delta;
		}
	}

	async function downloadMapSVG() {
		if (!bboxNELat || !bboxNELng || !bboxSWLat || !bboxSWLng) {
			error = 'Please set bounding box coordinates first';
			return;
		}

		downloading = true;
		error = '';

		try {
			// OpenStreetMap export API format: bbox=min_lon,min_lat,max_lon,max_lat
			const bbox = `${bboxSWLng},${bboxSWLat},${bboxNELng},${bboxNELat}`;
			const url = `https://render.openstreetmap.org/cgi-bin/export?bbox=${bbox}&scale=10000&format=svg`;

			const response = await fetch(url);
			if (!response.ok) {
				throw new Error(`Failed to download map: ${response.status} ${response.statusText}`);
			}

			const svgText = await response.text();

			// Check if the response is actually SVG
			if (!svgText.includes('<svg')) {
				throw new Error('Invalid SVG response from OpenStreetMap');
			}

			// Store as base64
			mapSvgData = btoa(svgText);
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to download map';
			console.error('Map download error:', e);
		} finally {
			downloading = false;
		}
	}

	function validateCoordinate(value: number | null, min: number, max: number): boolean {
		return value !== null && value >= min && value <= max;
	}

	$: isValidLatitude = latitude === null || validateCoordinate(latitude, -90, 90);
	$: isValidLongitude = longitude === null || validateCoordinate(longitude, -180, 180);
	$: isValidBBox =
		(bboxNELat === null || validateCoordinate(bboxNELat, -90, 90)) &&
		(bboxNELng === null || validateCoordinate(bboxNELng, -180, 180)) &&
		(bboxSWLat === null || validateCoordinate(bboxSWLat, -90, 90)) &&
		(bboxSWLng === null || validateCoordinate(bboxSWLng, -180, 180)) &&
		(bboxNELat === null || bboxSWLat === null || bboxNELat > bboxSWLat) &&
		(bboxNELng === null || bboxSWLng === null || bboxNELng > bboxSWLng);
</script>

<Card>
	<div class="space-y-6 p-6">
		<div>
			<h3 class="mb-2 text-lg font-semibold">Map Configuration</h3>
			<p class="text-surface-600-300-token text-sm">
				Configure the map area and radar position for inclusion in PDF reports.
			</p>
		</div>

		<!-- Radar Location -->
		<div class="space-y-4">
			<h4 class="flex items-center gap-2 font-medium">
				<span class="text-primary-500">
					<svg class="h-5 w-5" viewBox="0 0 24 24">
						<path fill="currentColor" d={mdiMapMarker} />
					</svg>
				</span>
				Radar Location
			</h4>

			<div class="grid gap-4 md:grid-cols-3">
				<TextField
					bind:value={latitude}
					label="Latitude"
					type="number"
					step="0.000001"
					placeholder="51.5074"
					error={!isValidLatitude ? 'Invalid latitude (-90 to 90)' : ''}
				/>
				<TextField
					bind:value={longitude}
					label="Longitude"
					type="number"
					step="0.000001"
					placeholder="-0.1278"
					error={!isValidLongitude ? 'Invalid longitude (-180 to 180)' : ''}
				/>
				<TextField
					bind:value={radarAngle}
					label="Radar Angle (degrees)"
					type="number"
					step="1"
					placeholder="0"
				/>
			</div>
		</div>

		<!-- Map Bounding Box -->
		<div class="space-y-4">
			<div class="flex items-center justify-between">
				<h4 class="font-medium">Map Bounding Box</h4>
				<Button
					size="sm"
					variant="outline"
					on:click={setDefaultBBox}
					disabled={!latitude || !longitude}
				>
					Set Default (~500m)
				</Button>
			</div>

			<div class="grid gap-4 md:grid-cols-2">
				<div>
					<p class="text-surface-600-300-token mb-2 text-sm">Northeast Corner</p>
					<div class="grid gap-2 md:grid-cols-2">
						<TextField
							bind:value={bboxNELat}
							label="NE Latitude"
							type="number"
							step="0.000001"
							placeholder="51.5124"
						/>
						<TextField
							bind:value={bboxNELng}
							label="NE Longitude"
							type="number"
							step="0.000001"
							placeholder="-0.1228"
						/>
					</div>
				</div>
				<div>
					<p class="text-surface-600-300-token mb-2 text-sm">Southwest Corner</p>
					<div class="grid gap-2 md:grid-cols-2">
						<TextField
							bind:value={bboxSWLat}
							label="SW Latitude"
							type="number"
							step="0.000001"
							placeholder="51.5024"
						/>
						<TextField
							bind:value={bboxSWLng}
							label="SW Longitude"
							type="number"
							step="0.000001"
							placeholder="-0.1328"
						/>
					</div>
				</div>
			</div>

			{#if !isValidBBox}
				<p class="text-danger-500 text-sm">
					Invalid bounding box: Ensure NE coordinates are greater than SW coordinates.
				</p>
			{/if}
		</div>

		<!-- Map Rotation -->
		<div class="space-y-4">
			<h4 class="flex items-center gap-2 font-medium">
				<span class="text-primary-500">
					<svg class="h-5 w-5" viewBox="0 0 24 24">
						<path fill="currentColor" d={mdiRotateRight} />
					</svg>
				</span>
				Map Rotation
			</h4>
			<TextField
				bind:value={mapRotation}
				label="Rotation (degrees)"
				type="number"
				step="1"
				placeholder="0"
			/>
		</div>

		<!-- Map Preview & Download -->
		<div class="space-y-4">
			<div class="flex items-center justify-between">
				<h4 class="font-medium">Map Preview & Download</h4>
				<Button
					on:click={downloadMapSVG}
					disabled={!isValidBBox || downloading}
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

			{#if previewUrl}
				<div class="rounded border border-gray-300 bg-gray-50 p-4">
					<p class="text-surface-600-300-token mb-2 text-sm">
						Preview (PNG approximation of the area):
					</p>
					<img
						src={previewUrl}
						alt="Map preview"
						class="max-h-96 w-full rounded border border-gray-200 object-contain"
						on:error={() => {
							error = 'Failed to load map preview. Check bounding box coordinates.';
						}}
					/>
					<p class="text-surface-600-300-token mt-2 text-xs">
						Note: Actual SVG may differ. Click "Download Map SVG" to fetch the vector version.
					</p>
				</div>
			{/if}
		</div>
	</div>
</Card>
