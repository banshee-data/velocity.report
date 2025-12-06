# LiDAR Track Visualization UI

This directory contains the frontend implementation for visualizing LiDAR tracking data.

## Status: Phase 1 & 2 Complete ✅

- Historical playback with 24-hour time window
- Two-pane layout: Map (Canvas) + Timeline (SVG)
- Background grid overlay visualization
- Track filtering, sorting, and selection

## Components

### Main Page

- `tracks/+page.svelte` - Main visualization page with two-pane layout

### Reusable Components

- `lib/components/lidar/MapPane.svelte` - Canvas-based top-down map view
- `lib/components/lidar/TimelinePane.svelte` - SVG timeline with playback controls
- `lib/components/lidar/TrackList.svelte` - Sidebar track browser with filters

### Data Layer

- `lib/types/lidar.ts` - TypeScript type definitions
- `lib/api.ts` - API client functions for LiDAR endpoints

## Features

### MapPane (Top 60%)

- **Canvas Rendering:** 60fps performance with 100+ tracks
- **Background Grid:** Visualizes learned background with stability indicators
- **Track Rendering:** Bounding boxes, velocity vectors, color-coded by classification
- **Interactive:**
  - Left-click: Select track
  - Right-click + drag: Pan view
  - Mouse wheel: Zoom (1-100x)
- **Legend:** Shows track classification colors

### TimelinePane (Bottom 40%)

- **Track Bars:** Horizontal bars showing track lifecycle (start→end)
- **Time Scrubber:** Red vertical line, draggable to navigate time
- **Playback Controls:**
  - Play/Pause button
  - Speed selection: 0.5x, 1x, 2x, 5x, 10x
  - Time display (HH:MM:SS format)
- **Track Labels:** Truncated track IDs with speed indicators
- **Color Coding:** Matches map visualization

### TrackList (Right Sidebar)

- **Filtering:**
  - By class: pedestrian, car, bird, other
  - By state: confirmed, tentative
- **Sorting:**
  - By start time (default)
  - By average speed
  - By duration
- **Track Cards:**
  - Icon, ID, classification, confidence
  - Speed, duration, observation count
  - State badge for tentative tracks

## Architecture

### Data Flow

```
API Client → Historical Query → Tracks + Observations
                ↓
         Component State
                ↓
    MapPane + TimelinePane + TrackList
         (synchronized via currentTime)
```

### Coordinate Systems

- **World Frame:** Cartesian (X, Y, Z in meters)
- **Map Rendering:** Screen coordinates with scale/offset
- **Background Grid:** Polar (ring, azimuth) → Cartesian conversion

### Performance

- **Target:** 60fps at 100 tracks
- **Canvas Optimization:**
  - Off-screen rendering for static elements
  - Object pooling (Path2D reuse planned)
  - Debounced resize handling
- **Timeline Optimization:**
  - Virtual scrolling for 100+ track bars
  - SVG for flexibility with good performance

## Backend Requirements

These Go API endpoints need to be implemented:

### 1. Historical Track Query (Critical)

```
GET /api/lidar/tracks/history?sensor_id={id}&start_time={nanos}&end_time={nanos}
```

Response:

```json
{
  "tracks": [
    {
      "track_id": "...",
      "sensor_id": "...",
      "position": {"x": 12.5, "y": 3.2, "z": 0.5},
      "velocity": {"vx": 5.2, "vy": -0.3},
      "speed_mps": 5.21,
      "object_class": "car",
      ...
    }
  ],
  "observations": {
    "track_id": [
      {
        "timestamp": "2025-12-05T...",
        "position": {"x": 10.0, "y": 2.5, "z": 0.5},
        ...
      }
    ]
  }
}
```

### 2. Background Grid API (Critical)

```
GET /api/lidar/background/grid?sensor_id={id}
```

Response:

```json
{
	"sensor_id": "hesai-pandar40p",
	"timestamp": "2025-12-05T...",
	"rings": 40,
	"azimuth_bins": 1800,
	"cells": [
		{
			"ring": 0,
			"azimuth_deg": 0.2,
			"average_range_meters": 25.3,
			"range_spread_meters": 0.5,
			"times_seen": 100
		}
	]
}
```

## Next Steps (Phase 3)

- [ ] Implement SSE streaming endpoint
- [ ] Add live mode toggle functionality
- [ ] Connection status indicator
- [ ] Memory leak detection tests
- [ ] Performance profiling

## Development

### Running Locally

```bash
cd web
npm install
npm run dev
```

### Testing

```bash
npm run check      # TypeScript checking
npm run lint       # ESLint
npm run test       # Jest tests
```

### Building

```bash
npm run build      # Production build
```

## References

- [LIDAR_UI_IMPLEMENTATION_PLAN.md](../../LIDAR_UI_IMPLEMENTATION_PLAN.md) - Full implementation plan
- [internal/lidar/docs/](../../internal/lidar/docs/) - Backend documentation
- [Architecture.md](../../ARCHITECTURE.md) - System architecture
