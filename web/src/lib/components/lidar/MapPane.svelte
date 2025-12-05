<script lang="ts">
	import { browser } from '$app/environment';
	import type { BackgroundGrid, Track } from '$lib/types/lidar';
	import { TRACK_COLORS } from '$lib/types/lidar';
	import { onDestroy, onMount } from 'svelte';

	export let tracks: Track[] = [];
	export let selectedTrackId: string | null = null;
	export let backgroundGrid: BackgroundGrid | null = null;
	export let onTrackSelect: (trackId: string) => void = () => {};

	let canvas: HTMLCanvasElement;
	let ctx: CanvasRenderingContext2D | null = null;
	let containerWidth = 800;
	let containerHeight = 600;

	// View state
	let scale = 10; // pixels per meter
	let offsetX = 0; // world coordinates offset
	let offsetY = 0;
	let isPanning = false;
	let lastMouseX = 0;
	let lastMouseY = 0;

	// Animation frame
	let animationFrame: number | null = null;

	// Initialize canvas
	function initCanvas() {
		if (!canvas) return;
		ctx = canvas.getContext('2d');
		updateCanvasSize();
		render();
	}

	// Update canvas size to match container
	function updateCanvasSize() {
		if (!canvas) return;
		const container = canvas.parentElement;
		if (container) {
			containerWidth = container.clientWidth;
			containerHeight = container.clientHeight;
			canvas.width = containerWidth;
			canvas.height = containerHeight;
		}
	}

	// Convert world coordinates to screen coordinates
	function worldToScreen(x: number, y: number): [number, number] {
		const screenX = containerWidth / 2 + (x - offsetX) * scale;
		const screenY = containerHeight / 2 - (y - offsetY) * scale; // Flip Y axis
		return [screenX, screenY];
	}

	// Convert screen coordinates to world coordinates
	function screenToWorld(screenX: number, screenY: number): [number, number] {
		const x = (screenX - containerWidth / 2) / scale + offsetX;
		const y = -(screenY - containerHeight / 2) / scale + offsetY; // Flip Y axis
		return [x, y];
	}

	// Render the scene
	function render() {
		if (!ctx || !canvas) return;

		// Clear canvas
		ctx.fillStyle = '#1a1a1a';
		ctx.fillRect(0, 0, canvas.width, canvas.height);

		// Draw background grid if available
		if (backgroundGrid) {
			renderBackgroundGrid();
		}

		// Draw grid lines
		renderGridLines();

		// Draw tracks
		tracks.forEach((track) => {
			renderTrack(track, track.track_id === selectedTrackId);
		});

		// Draw legend
		renderLegend();
	}

	// Render background grid overlay
	function renderBackgroundGrid() {
		if (!ctx || !backgroundGrid) return;

		ctx.save();
		ctx.globalAlpha = 0.3;

		// Draw simplified grid representation
		// For performance, we sample the grid rather than drawing all 72,000 cells
		const sampleRate = 10; // Sample every 10th cell

		backgroundGrid.cells.forEach((cell) => {
			if (cell.ring % sampleRate !== 0 || Math.floor(cell.azimuth_deg / 2) % sampleRate !== 0) {
				return;
			}

			// Convert polar to Cartesian
			const angleRad = (cell.azimuth_deg * Math.PI) / 180;
			const x = cell.average_range_meters * Math.cos(angleRad);
			const y = cell.average_range_meters * Math.sin(angleRad);

			const [screenX, screenY] = worldToScreen(x, y);

			// Color based on range spread (stability indicator)
			const stability = Math.max(0, 1 - cell.range_spread_meters / 2);
			ctx.fillStyle = `rgba(100, 150, 255, ${stability * 0.5})`;
			ctx.fillRect(screenX - 1, screenY - 1, 2, 2);
		});

		ctx.restore();
	}

	// Render grid lines
	function renderGridLines() {
		if (!ctx) return;

		ctx.save();
		ctx.strokeStyle = '#333';
		ctx.lineWidth = 1;

		// Draw concentric circles (every 10 meters)
		for (let r = 10; r <= 50; r += 10) {
			ctx.beginPath();
			ctx.arc(containerWidth / 2, containerHeight / 2, r * scale, 0, Math.PI * 2);
			ctx.stroke();

			// Label
			ctx.fillStyle = '#666';
			ctx.font = '10px monospace';
			ctx.fillText(`${r}m`, containerWidth / 2 + r * scale + 5, containerHeight / 2);
		}

		// Draw axes
		ctx.strokeStyle = '#555';
		ctx.lineWidth = 2;

		// X axis
		ctx.beginPath();
		ctx.moveTo(0, containerHeight / 2);
		ctx.lineTo(containerWidth, containerHeight / 2);
		ctx.stroke();

		// Y axis
		ctx.beginPath();
		ctx.moveTo(containerWidth / 2, 0);
		ctx.lineTo(containerWidth / 2, containerHeight);
		ctx.stroke();

		ctx.restore();
	}

	// Render a single track
	function renderTrack(track: Track, isSelected: boolean) {
		if (!ctx) return;

		const [screenX, screenY] = worldToScreen(track.position.x, track.position.y);

		// Get color based on classification or state
		let color = TRACK_COLORS.other;
		if (track.state === 'tentative') {
			color = TRACK_COLORS.tentative;
		} else if (track.state === 'deleted') {
			color = TRACK_COLORS.deleted;
		} else if (track.object_class && track.object_class in TRACK_COLORS) {
			color = TRACK_COLORS[track.object_class as keyof typeof TRACK_COLORS];
		}

		ctx.save();

		// Draw bounding box
		const bbox = track.bounding_box;
		const length = bbox.length_avg * scale;
		const width = bbox.width_avg * scale;

		ctx.translate(screenX, screenY);
		ctx.rotate(-track.heading_rad); // Negative because Y is flipped

		// Fill bounding box
		ctx.fillStyle = `${color}33`; // 20% opacity
		ctx.fillRect(-length / 2, -width / 2, length, width);

		// Stroke bounding box
		ctx.strokeStyle = color;
		ctx.lineWidth = isSelected ? 3 : 2;
		if (track.state === 'tentative') {
			ctx.setLineDash([5, 5]);
		}
		ctx.strokeRect(-length / 2, -width / 2, length, width);
		ctx.setLineDash([]);

		ctx.restore();

		// Draw velocity vector
		const velLength = Math.sqrt(track.velocity.vx ** 2 + track.velocity.vy ** 2);
		if (velLength > 0.5) {
			// Only draw if moving significantly
			ctx.strokeStyle = color;
			ctx.lineWidth = 2;
			ctx.beginPath();
			ctx.moveTo(screenX, screenY);
			const endX = screenX + track.velocity.vx * scale * 0.5;
			const endY = screenY - track.velocity.vy * scale * 0.5; // Flip Y
			ctx.lineTo(endX, endY);
			ctx.stroke();

			// Arrow head
			const angle = Math.atan2(-(endY - screenY), endX - screenX);
			ctx.beginPath();
			ctx.moveTo(endX, endY);
			ctx.lineTo(
				endX - 10 * Math.cos(angle - Math.PI / 6),
				endY - 10 * Math.sin(angle - Math.PI / 6)
			);
			ctx.lineTo(
				endX - 10 * Math.cos(angle + Math.PI / 6),
				endY - 10 * Math.sin(angle + Math.PI / 6)
			);
			ctx.closePath();
			ctx.fillStyle = color;
			ctx.fill();
		}

		// Draw track ID label
		if (isSelected || tracks.length < 20) {
			ctx.fillStyle = color;
			ctx.font = '12px monospace';
			ctx.fillText(`${track.track_id}`, screenX + 10, screenY - 10);
		}
	}

	// Render legend
	function renderLegend() {
		if (!ctx) return;

		const legendX = 20;
		const legendY = 20;
		const lineHeight = 25;

		ctx.save();
		ctx.font = '12px monospace';

		let y = legendY;

		// Track classes
		const classes: Array<{ label: string; key: keyof typeof TRACK_COLORS }> = [
			{ label: 'Pedestrian', key: 'pedestrian' },
			{ label: 'Car', key: 'car' },
			{ label: 'Bird', key: 'bird' },
			{ label: 'Other', key: 'other' },
			{ label: 'Tentative', key: 'tentative' }
		];

		classes.forEach(({ label, key }) => {
			ctx!.fillStyle = TRACK_COLORS[key];
			ctx!.fillRect(legendX, y, 15, 15);
			ctx!.fillStyle = '#fff';
			ctx!.fillText(label, legendX + 20, y + 12);
			y += lineHeight;
		});

		ctx.restore();
	}

	// Mouse event handlers
	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		const zoomFactor = 1.1;
		if (e.deltaY < 0) {
			scale *= zoomFactor;
		} else {
			scale /= zoomFactor;
		}
		scale = Math.max(1, Math.min(100, scale)); // Clamp between 1 and 100
		render();
	}

	function handleMouseDown(e: MouseEvent) {
		if (e.button === 0) {
			// Left click - check for track selection
			const [worldX, worldY] = screenToWorld(e.offsetX, e.offsetY);

			// Find closest track
			let closestTrack: Track | null = null;
			let closestDist = Infinity;

			tracks.forEach((track) => {
				const dx = track.position.x - worldX;
				const dy = track.position.y - worldY;
				const dist = Math.sqrt(dx * dx + dy * dy);

				if (dist < 2 && dist < closestDist) {
					// Within 2 meters
					closestTrack = track;
					closestDist = dist;
				}
			});

			if (closestTrack) {
				onTrackSelect(closestTrack.track_id);
			}
		} else if (e.button === 2) {
			// Right click - pan
			isPanning = true;
			lastMouseX = e.clientX;
			lastMouseY = e.clientY;
		}
	}

	function handleMouseMove(e: MouseEvent) {
		if (isPanning) {
			const dx = e.clientX - lastMouseX;
			const dy = e.clientY - lastMouseY;
			offsetX -= dx / scale;
			offsetY += dy / scale; // Flip Y
			lastMouseX = e.clientX;
			lastMouseY = e.clientY;
			render();
		}
	}

	function handleMouseUp() {
		isPanning = false;
	}

	function handleContextMenu(e: MouseEvent) {
		e.preventDefault();
	}

	// Resize handler
	let resizeTimeout: number | null = null;
	function handleResize() {
		if (!browser) return;
		if (resizeTimeout !== null) {
			clearTimeout(resizeTimeout);
		}
		resizeTimeout = window.setTimeout(() => {
			updateCanvasSize();
			render();
		}, 100);
	}

	// Animation loop
	function startAnimation() {
		if (!browser) return;
		function animate() {
			render();
			animationFrame = requestAnimationFrame(animate);
		}
		animate();
	}

	function stopAnimation() {
		if (!browser) return;
		if (animationFrame !== null) {
			cancelAnimationFrame(animationFrame);
			animationFrame = null;
		}
	}

	// Lifecycle
	onMount(() => {
		if (!browser) return;
		initCanvas();
		window.addEventListener('resize', handleResize);
		startAnimation();
	});

	onDestroy(() => {
		if (!browser) return;
		window.removeEventListener('resize', handleResize);
		stopAnimation();
	});

	// Reactive rendering when tracks change
	$: if (ctx && (tracks || selectedTrackId || backgroundGrid)) {
		render();
	}
</script>

<div class="relative h-full w-full">
	<canvas
		bind:this={canvas}
		on:wheel={handleWheel}
		on:mousedown={handleMouseDown}
		on:mousemove={handleMouseMove}
		on:mouseup={handleMouseUp}
		on:contextmenu={handleContextMenu}
		class="cursor-move"
	/>

	<!-- Controls overlay -->
	<div class="top-4 right-4 bg-black bg-opacity-75 text-white p-3 rounded text-sm absolute">
		<div class="font-mono">
			<div>Scale: {scale.toFixed(1)}x</div>
			<div class="text-xs mt-2 text-gray-400">
				<div>Left click: Select track</div>
				<div>Right click + drag: Pan</div>
				<div>Scroll: Zoom</div>
			</div>
		</div>
	</div>
</div>

<style>
	canvas {
		display: block;
		width: 100%;
		height: 100%;
	}
</style>
