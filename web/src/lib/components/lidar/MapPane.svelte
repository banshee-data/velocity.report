<script lang="ts">
	import { browser } from '$app/environment';
	import type { BackgroundGrid, MissedRegion, Track, TrackObservation } from '$lib/types/lidar';
	import { TRACK_COLORS } from '$lib/types/lidar';
	import { onDestroy, onMount } from 'svelte';

	// Rendering constants
	const RANGE_SPREAD_THRESHOLD = 2.0; // meters - threshold for background stability
	const MIN_VELOCITY_FOR_ARROW = 0.5; // m/s - only draw velocity arrows for significant movement
	const TRACK_SELECTION_RADIUS = 2.0; // meters - click tolerance for track selection
	const MAX_TRACKS_FOR_LABELS = 20; // Show all labels when track count is below this
	const GRID_CIRCLE_INTERVAL = 10; // meters - spacing between grid circles
	const CROSSHAIR_SIZE = 12; // pixels

	export let tracks: Track[] = [];
	export let selectedTrackId: string | null = null;
	export let backgroundGrid: BackgroundGrid | null = null;
	export let onTrackSelect: (trackId: string) => void = () => {};
	// Current playback time (ms) - for progressive track reveal
	export let currentTime: number | null = null;
	// Observations for the selected track (optional, used for overlay of raw points)
	export let observations: TrackObservation[] = [];
	// Foreground observations overlay (time-window slice for debugging)
	export let foreground: TrackObservation[] = [];
	export let foregroundEnabled = true;
	export let foregroundOffset = { x: 0, y: 0 };
	// Missed regions (Phase 7)
	export let missedRegions: MissedRegion[] = [];
	export let markMissedMode = false;
	export let onMapClick: ((worldX: number, worldY: number) => void) | null = null;
	export let onDeleteMissedRegion: ((regionId: string) => void) | null = null;
	// Toggles (controlled locally)
	let showHistory = true;
	let showObservations = true;
	let showCrosshair = true;
	let showMouseCoords = true;

	let canvas: HTMLCanvasElement;
	let ctx: CanvasRenderingContext2D | null = null;
	let containerWidth = 800;
	let containerHeight = 600;

	/**
	 * View state and coordinate system:
	 * - The visualization uses a "world coordinate" system where units are meters, with (0,0) as the origin.
	 * - `scale` defines the zoom level: number of pixels per meter in world coordinates.
	 * - `offsetX` and `offsetY` represent the camera's position in world coordinates (meters).
	 *   The view is centered at (offsetX, offsetY) in world space.
	 * - When rendering, world coordinates are transformed to screen coordinates:
	 *     screenX = containerWidth / 2 + (worldX - offsetX) * scale
	 *     screenY = containerHeight / 2 - (worldY - offsetY) * scale
	 *   (Y axis is flipped so that positive Y is upwards on screen.)
	 * - Panning changes offsetX/offsetY; zooming changes scale.
	 * - This convention allows for intuitive panning and zooming of the map.
	 */
	let scale = 10; // pixels per meter
	let offsetX = 0; // world coordinates offset (camera position X)
	let offsetY = 0; // world coordinates offset (camera position Y)
	let isPanning = false;
	let lastMouseX = 0;
	let lastMouseY = 0;

	// Animation frame
	let animationFrame: number | null = null;
	let isDirty = true; // Track if re-render is needed

	// Offscreen canvas for background caching
	let bgCanvas: HTMLCanvasElement | null = null;
	let bgCtx: CanvasRenderingContext2D | null = null;
	let lastBgState = {
		scale: 0,
		offsetX: 0,
		offsetY: 0,
		width: 0,
		height: 0,
		dataVersion: 0
	};

	// Detect background grid changes
	let bgDataVersion = 0;
	$: if (backgroundGrid) {
		bgDataVersion++;
	}

	// Mouse world coordinate readout
	let hoverWorld: { x: number; y: number } | null = null;

	// Mark as dirty when props change
	$: if (
		tracks ||
		selectedTrackId ||
		backgroundGrid ||
		observations ||
		foreground ||
		foregroundOffset ||
		foregroundEnabled ||
		currentTime != null ||
		missedRegions ||
		markMissedMode
	) {
		markDirty();
	}

	// Pre-compute time-filtered observation subsets (avoids filtering inside render loop)
	$: visibleForegroundObs =
		currentTime != null
			? foreground.filter(
					(obs) => !obs.timestamp || new Date(obs.timestamp).getTime() <= currentTime
				)
			: foreground;

	$: visibleSelectedObs =
		currentTime != null
			? observations.filter(
					(obs) => !obs.timestamp || new Date(obs.timestamp).getTime() <= currentTime
				)
			: observations;

	// Mark view as needing re-render
	function markDirty() {
		isDirty = true;
	}

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

		// Draw tracks - log for debugging
		// console.log('[MapPane] View state:', {
		// 	scale,
		// 	offsetX,
		// 	offsetY,
		// 	containerWidth,
		// 	containerHeight
		// });
		// console.log('[MapPane] Rendering', tracks.length, 'tracks');
		// if (tracks.length > 0) {
		// 	console.log('[MapPane] First track:', tracks[0]);
		// 	console.log('[MapPane] Sample world to screen:', worldToScreen(0, 0), worldToScreen(10, 10));
		// }

		tracks.forEach((track) => {
			renderTrack(track, track.track_id === selectedTrackId);
		});

		// Foreground observation layer (time-window slice, progressive)
		if (foregroundEnabled && visibleForegroundObs.length > 0) {
			renderForeground();
		}

		// Draw observations for the selected track if provided (progressive)
		if (showObservations && visibleSelectedObs.length > 0) {
			renderObservations();
		}

		// Draw missed regions (Phase 7)
		if (missedRegions.length > 0) {
			renderMissedRegions();
		}

		// Draw crosshair at world origin
		if (showCrosshair) {
			renderCrosshair();
		}

		// Draw legend
		renderLegend();
	}

	function renderCrosshair() {
		const ctxLocal = ctx;
		if (!ctxLocal) return;
		const [cx, cy] = worldToScreen(0, 0);
		ctxLocal.save();
		ctxLocal.strokeStyle = '#4ade80';
		ctxLocal.lineWidth = 1.5;
		ctxLocal.beginPath();
		ctxLocal.moveTo(cx - CROSSHAIR_SIZE, cy);
		ctxLocal.lineTo(cx + CROSSHAIR_SIZE, cy);
		ctxLocal.moveTo(cx, cy - CROSSHAIR_SIZE);
		ctxLocal.lineTo(cx, cy + CROSSHAIR_SIZE);
		ctxLocal.stroke();
		ctxLocal.restore();
	}

	function renderObservations() {
		const ctxLocal = ctx;
		if (!ctxLocal) return;
		ctxLocal.save();
		ctxLocal.fillStyle = '#60a5fa';
		ctxLocal.globalAlpha = 0.8;
		const size = Math.max(2, 4 - scale * 0.02);
		visibleSelectedObs.forEach((obs) => {
			const [sx, sy] = worldToScreen(obs.position.x, obs.position.y);
			ctxLocal.beginPath();
			ctxLocal.arc(sx, sy, size, 0, Math.PI * 2);
			ctxLocal.fill();
		});
		ctxLocal.restore();
	}

	function renderForeground() {
		const ctxLocal = ctx;
		if (!ctxLocal) return;
		ctxLocal.save();
		ctxLocal.fillStyle = '#f472b6';
		ctxLocal.globalAlpha = 0.85;
		const size = Math.max(2, 4 - scale * 0.02);
		const offsetXLocal = foregroundOffset.x || 0;
		const offsetYLocal = foregroundOffset.y || 0;
		visibleForegroundObs.forEach((obs) => {
			const worldX = obs.position.x + offsetXLocal;
			const worldY = obs.position.y + offsetYLocal;
			const [sx, sy] = worldToScreen(worldX, worldY);
			ctxLocal.beginPath();
			ctxLocal.arc(sx, sy, size, 0, Math.PI * 2);
			ctxLocal.fill();
		});
		ctxLocal.restore();
	}

	// Render missed regions as dashed purple circles
	function renderMissedRegions() {
		const ctxLocal = ctx;
		if (!ctxLocal || missedRegions.length === 0) return;

		ctxLocal.save();
		missedRegions.forEach((region) => {
			const [cx, cy] = worldToScreen(region.center_x, region.center_y);
			const radiusPx = region.radius_m * scale;

			// Dashed purple circle
			ctxLocal.beginPath();
			ctxLocal.strokeStyle = '#a855f7';
			ctxLocal.lineWidth = 2;
			ctxLocal.setLineDash([6, 4]);
			ctxLocal.arc(cx, cy, radiusPx, 0, Math.PI * 2);
			ctxLocal.stroke();
			ctxLocal.setLineDash([]);

			// Semi-transparent fill
			ctxLocal.fillStyle = 'rgba(168, 85, 247, 0.15)';
			ctxLocal.fill();

			// "MISSED" label
			ctxLocal.fillStyle = '#a855f7';
			ctxLocal.font = '10px monospace';
			ctxLocal.textAlign = 'center';
			ctxLocal.fillText('MISSED', cx, cy - radiusPx - 5);
			ctxLocal.textAlign = 'start';

			// Delete button (small "x" in top-right of circle)
			if (onDeleteMissedRegion) {
				const btnX = cx + radiusPx * 0.7;
				const btnY = cy - radiusPx * 0.7;
				ctxLocal.fillStyle = 'rgba(239, 68, 68, 0.8)';
				ctxLocal.beginPath();
				ctxLocal.arc(btnX, btnY, 7, 0, Math.PI * 2);
				ctxLocal.fill();
				ctxLocal.strokeStyle = '#fff';
				ctxLocal.lineWidth = 1.5;
				ctxLocal.beginPath();
				ctxLocal.moveTo(btnX - 3, btnY - 3);
				ctxLocal.lineTo(btnX + 3, btnY + 3);
				ctxLocal.moveTo(btnX + 3, btnY - 3);
				ctxLocal.lineTo(btnX - 3, btnY + 3);
				ctxLocal.stroke();
			}
		});
		ctxLocal.restore();
	}

	// Render background grid overlay
	function renderBackgroundGrid() {
		if (!ctx || !backgroundGrid) return;

		// Initialize offscreen canvas if needed
		if (!bgCanvas) {
			bgCanvas = document.createElement('canvas');
			bgCtx = bgCanvas.getContext('2d');
		}

		if (!bgCanvas || !bgCtx) return;

		// Check if update needed
		const needsUpdate =
			scale !== lastBgState.scale ||
			offsetX !== lastBgState.offsetX ||
			offsetY !== lastBgState.offsetY ||
			containerWidth !== lastBgState.width ||
			containerHeight !== lastBgState.height ||
			bgDataVersion !== lastBgState.dataVersion;

		if (needsUpdate) {
			// Update canvas size
			if (bgCanvas.width !== containerWidth || bgCanvas.height !== containerHeight) {
				bgCanvas.width = containerWidth;
				bgCanvas.height = containerHeight;
			}

			// Clear offscreen
			bgCtx.clearRect(0, 0, containerWidth, containerHeight);

			// Render grid to offscreen
			bgCtx.save();
			bgCtx.globalAlpha = 0.5;

			// Use local variables to avoid scope lookups in loop
			const _scale = scale;
			const _offsetX = offsetX;
			const _offsetY = offsetY;
			const _cw = containerWidth;
			const _ch = containerHeight;
			const _cells = backgroundGrid.cells;

			// Pre-calculate constants
			const halfW = _cw / 2;
			const halfH = _ch / 2;

			for (let i = 0; i < _cells.length; i++) {
				const cell = _cells[i];

				// Inline worldToScreen for performance
				const screenX = halfW + (cell.x - _offsetX) * _scale;
				const screenY = halfH - (cell.y - _offsetY) * _scale;

				// Skip if out of bounds (culling)
				if (screenX < -5 || screenX > _cw + 5 || screenY < -5 || screenY > _ch + 5) continue;

				const stability = Math.max(0, 1 - cell.range_spread_meters / RANGE_SPREAD_THRESHOLD);
				bgCtx.fillStyle = `rgba(100, 150, 255, ${stability * 0.5})`;
				bgCtx.fillRect(screenX - 1.5, screenY - 1.5, 3, 3);
			}

			bgCtx.restore();

			// Update state
			lastBgState = {
				scale,
				offsetX,
				offsetY,
				width: containerWidth,
				height: containerHeight,
				dataVersion: bgDataVersion
			};
		}

		// Draw offscreen canvas to main canvas
		ctx.drawImage(bgCanvas, 0, 0);
	}

	// Render grid lines
	function renderGridLines() {
		if (!ctx) return;

		ctx.save();
		ctx.strokeStyle = '#333';
		ctx.lineWidth = 1;

		// Draw concentric circles (every GRID_CIRCLE_INTERVAL meters)
		for (let r = GRID_CIRCLE_INTERVAL; r <= 50; r += GRID_CIRCLE_INTERVAL) {
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

		// Skip if essential data is missing
		if (!track.position) return;

		// Use current position by default
		let pos = track.position;
		let useCurrentPos = true;

		// If current position is invalid (0,0), try to find last valid history point
		if (Math.abs(pos.x) < 0.01 && Math.abs(pos.y) < 0.01) {
			useCurrentPos = false;
			if (track.history && track.history.length > 0) {
				// Search backwards for a valid point
				for (let i = track.history.length - 1; i >= 0; i--) {
					const pt = track.history[i];
					if (Math.abs(pt.x) >= 0.01 || Math.abs(pt.y) >= 0.01) {
						pos = { x: pt.x, y: pt.y, z: 0 };
						break;
					}
				}
			}
		}

		// If we still have an invalid position after searching history, skip rendering
		if (Math.abs(pos.x) < 0.01 && Math.abs(pos.y) < 0.01) return;

		const [screenX, screenY] = worldToScreen(pos.x, pos.y);

		// Get color based on classification or state
		let color: string = TRACK_COLORS.other;
		if (track.state === 'tentative') {
			color = TRACK_COLORS.tentative;
		} else if (track.state === 'deleted') {
			color = TRACK_COLORS.deleted;
		} else if (track.object_class && track.object_class in TRACK_COLORS) {
			color = TRACK_COLORS[track.object_class as keyof typeof TRACK_COLORS];
		}

		// Draw history path
		if (showHistory && track.history && track.history.length > 1) {
			ctx.beginPath();
			ctx.strokeStyle = color;
			ctx.lineWidth = isSelected ? 2 : 1;
			ctx.globalAlpha = 0.5;

			// Sort history by timestamp to ensure coherent lines
			const sortedHistory = [...track.history].sort((a, b) => {
				// Handle missing timestamps gracefully (though they should exist based on type)
				const tA = a.timestamp ? new Date(a.timestamp).getTime() : 0;
				const tB = b.timestamp ? new Date(b.timestamp).getTime() : 0;
				return tA - tB;
			});

			// Filter history for progressive reveal during playback
			// Only show points up to and including the current playback time
			const visibleHistory = currentTime
				? sortedHistory.filter((pt) => {
						if (!pt.timestamp) return true; // Show points without timestamps
						const ptTime = new Date(pt.timestamp).getTime();
						return ptTime <= currentTime;
					})
				: sortedHistory;

			let firstPointDrawn = false;

			// Helper to check if point is valid
			const isValid = (pt: { x: number; y: number }) =>
				Math.abs(pt.x) >= 0.01 || Math.abs(pt.y) >= 0.01;

			for (let i = 0; i < visibleHistory.length; i++) {
				const pt = visibleHistory[i];

				// Skip invalid points entirely - connect line between valid neighbors
				if (!isValid(pt)) continue;

				const [x, y] = worldToScreen(pt.x, pt.y);

				if (!firstPointDrawn) {
					ctx.moveTo(x, y);
					firstPointDrawn = true;
				} else {
					ctx.lineTo(x, y);
				}
			}

			// If we drew path, stroke it
			if (firstPointDrawn) {
				ctx.stroke();
			}
			ctx.globalAlpha = 1.0;
		}

		ctx.save();

		// Only render bounding box / heading if we are using the current valid position
		if (useCurrentPos) {
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
			if (velLength > MIN_VELOCITY_FOR_ARROW) {
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
		} else {
			// If we are showing a historical "last known" point, just draw a small marker
			ctx.restore(); // Restore from save() before bb check
			ctx.fillStyle = color;
			ctx.globalAlpha = 0.5;
			ctx.beginPath();
			ctx.arc(screenX, screenY, 3, 0, Math.PI * 2);
			ctx.fill();
			ctx.globalAlpha = 1.0;
		}

		// Draw track ID label
		if (isSelected || tracks.length < MAX_TRACKS_FOR_LABELS) {
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

		// Overlay layers
		const overlays = [
			{ label: 'Track observations', color: '#60a5fa' },
			{ label: 'Foreground (window)', color: '#f472b6' },
			{ label: 'Missed region', color: '#a855f7' }
		];

		overlays.forEach(({ label, color }) => {
			ctx!.fillStyle = color;
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
		markDirty();
	}

	function handleMouseDown(e: MouseEvent) {
		if (e.button === 0) {
			// Left click - check for track selection or mark-missed mode
			const [worldX, worldY] = screenToWorld(e.offsetX, e.offsetY);

			// Check if clicking a missed region delete button
			if (onDeleteMissedRegion && missedRegions.length > 0) {
				for (const region of missedRegions) {
					const [cx, cy] = worldToScreen(region.center_x, region.center_y);
					const radiusPx = region.radius_m * scale;
					const btnX = cx + radiusPx * 0.7;
					const btnY = cy - radiusPx * 0.7;
					const dist = Math.hypot(e.offsetX - btnX, e.offsetY - btnY);
					if (dist <= 10) {
						onDeleteMissedRegion(region.region_id);
						return;
					}
				}
			}

			// Mark-missed mode: create a missed region at click location
			if (markMissedMode && onMapClick) {
				onMapClick(worldX, worldY);
				return;
			}

			// Find closest track
			let closestTrack: Track | null = null;
			let closestDist = Infinity;

			for (const track of tracks) {
				// Check distance to current position
				let minDist = Math.hypot(track.position.x - worldX, track.position.y - worldY);

				// Check distance to historical points (to allow clicking on trail)
				if (track.history) {
					for (const pt of track.history) {
						const dist = Math.hypot(pt.x - worldX, pt.y - worldY);
						if (dist < minDist) minDist = dist;
					}
				}

				if (minDist < TRACK_SELECTION_RADIUS && minDist < closestDist) {
					// Within selection radius
					closestTrack = track;
					closestDist = minDist;
				}
			}

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
			markDirty();
		}

		if (showMouseCoords) {
			const [wx, wy] = screenToWorld(e.offsetX, e.offsetY);
			hoverWorld = { x: wx, y: wy };
			markDirty();
		}
	}

	function handleMouseUp() {
		isPanning = false;
	}

	function handleContextMenu(e: MouseEvent) {
		e.preventDefault();
	}

	// Resize handler
	let resizeTimeout: ReturnType<typeof setTimeout> | null = null;
	let resizeObserver: ResizeObserver | null = null;
	function handleResize() {
		if (!browser) return;
		if (resizeTimeout !== null) {
			clearTimeout(resizeTimeout);
		}
		resizeTimeout = setTimeout(() => {
			updateCanvasSize();
			markDirty();
		}, 100);
	}

	// Animation loop with dirty flag optimization
	function startAnimation() {
		if (!browser) return;
		function animate() {
			if (isDirty) {
				render();
				isDirty = false;
			}
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

		// Observe parent container size changes (e.g. from drag-resize handle)
		const container = canvas?.parentElement;
		if (container) {
			resizeObserver = new ResizeObserver(() => {
				handleResize();
			});
			resizeObserver.observe(container);
		}

		startAnimation();
	});

	onDestroy(() => {
		if (typeof window !== 'undefined') {
			window.removeEventListener('resize', handleResize);
		}
		if (resizeObserver) {
			resizeObserver.disconnect();
			resizeObserver = null;
		}
		stopAnimation();
		if (resizeTimeout !== null) {
			clearTimeout(resizeTimeout);
			resizeTimeout = null;
		}
	});
</script>

<div class="relative h-full w-full">
	<canvas
		bind:this={canvas}
		on:wheel={handleWheel}
		on:mousedown={handleMouseDown}
		on:mousemove={handleMouseMove}
		on:mouseup={handleMouseUp}
		on:contextmenu={handleContextMenu}
		class={markMissedMode ? 'cursor-crosshair' : 'cursor-move'}
	></canvas>

	<!-- Controls overlay -->
	<div class="bg-opacity-75 absolute top-4 right-4 rounded bg-black p-3 text-sm text-white">
		<div class="font-mono">
			<div>Scale: {scale.toFixed(1)}x</div>
			<div class="mt-2 text-xs text-gray-400">
				<div>Left click: {markMissedMode ? 'Mark missed region' : 'Select track'}</div>
				<div>Right click + drag: Pan</div>
				<div>Scroll: Zoom</div>
			</div>
		</div>
	</div>

	<!-- Debug/toggle panel -->
	<div
		class="bg-opacity-80 absolute bottom-4 left-4 rounded bg-black p-3 text-xs text-white shadow-lg"
	>
		<div class="space-y-1 font-mono">
			<div class="flex items-center gap-2">
				<input id="toggle-crosshair" type="checkbox" bind:checked={showCrosshair} />
				<label for="toggle-crosshair">Crosshair</label>
			</div>
			<div class="flex items-center gap-2">
				<input id="toggle-history" type="checkbox" bind:checked={showHistory} />
				<label for="toggle-history">Track history</label>
			</div>
			<div class="flex items-center gap-2">
				<input id="toggle-obs" type="checkbox" bind:checked={showObservations} />
				<label for="toggle-obs">Raw observations</label>
			</div>
			<div class="flex items-center gap-2">
				<input id="toggle-mouse" type="checkbox" bind:checked={showMouseCoords} />
				<label for="toggle-mouse">Mouse world coords</label>
			</div>
			{#if hoverWorld && showMouseCoords}
				<div class="mt-2 text-blue-200">
					<span>World:</span>
					<span>{hoverWorld.x.toFixed(2)}, {hoverWorld.y.toFixed(2)} m</span>
				</div>
			{/if}
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
