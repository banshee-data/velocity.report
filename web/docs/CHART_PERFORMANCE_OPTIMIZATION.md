# Chart Performance Optimization Plan

## Executive Summary

The dashboard `loadChart()` function is experiencing severe performance degradation with 3.7s execution time, causing main thread blocking and UI jank. This document outlines a phased approach to reduce this to <500ms.

## Problem Statement

### Current Performance Profile

- **Total execution time**: 3,719.8ms
- **Main bottleneck**: Function call with 3,333.6ms self-time
- **Root cause**: Synchronous reactive updates triggering cascading Svelte component re-renders
- **Affected components**: BrushContext, SelectField, TooltipContext, Svg, Input, Voronoi, Group, CircleClipPath, ClipPath, Circle

### Impact

- Severe UI jank when changing grouping (1h, 2h, 4h, etc.)
- Poor user experience during date range selection
- Main thread blocking prevents other interactions
- Particularly bad with larger datasets (14+ days)

### Data Flow Analysis

For a typical 14-day period with 4h grouping:

- Input: ~84 rows (14 days × 6 intervals/day)
- Transformation: 84 rows × 4 metrics = 336 data points
- Problem: Each data point triggers reactive updates in multiple chart components
- Result: 336+ synchronous re-renders

## Optimization Strategy

### Phase 1: Immediate Wins (Week 1, 6 hours)

**Goal**: 30-50% reduction, better perceived performance
**Expected result**: 1.8-2.2s execution time

#### 1.1 Add Transformed Data Memoization Cache

**Effort**: 2 hours
**Expected gain**: 50-70% on cache hits (1.8-2.6s saved)

- Create `lastTransformedData` cache keyed by request params
- Reuse transformed chartData when only viewing/grouping changes
- Clear cache on date range or source changes

**Implementation**:

```typescript
let lastTransformedData: Array<{date: Date; metric: string; value: number}> | null = null;
let lastTransformKey = '';

// In loadChart(), before transformation:
const transformKey = `${requestKey}`;
if (lastTransformedData && transformKey === lastTransformKey) {
  chartData = lastTransformedData;
  return;
}
// ... do transformation ...
lastTransformedData = rows;
lastTransformKey = transformKey;
```

#### 1.2 Batch Reactive Updates with requestAnimationFrame

**Effort**: 3 hours
**Expected gain**: 30-40% reduction (1.1-1.5s saved)

- Defer `chartData` assignment to next animation frame
- Use Svelte's `tick()` to batch state updates
- Prevents multiple synchronous render passes

**Implementation**:

```typescript
import { tick } from 'svelte';

// In loadChart(), at the end:
await tick(); // flush pending updates
requestAnimationFrame(() => {
  chartData = rows;
});
```

#### 1.3 Add Chart Loading State

**Effort**: 1 hour
**Expected gain**: Better UX, no performance change

- Show subtle loading indicator during chart updates
- Prevent perceived jank
- Give user feedback during long operations

**Implementation**:

```typescript
let chartLoading = false;

// In loadChart():
chartLoading = true;
try {
  // ... transformation ...
} finally {
  chartLoading = false;
}
```

### Phase 2: Core Optimization (Week 2, 14 hours)

**Goal**: 60-80% reduction, <1s execution time
**Expected result**: 500-800ms execution time

#### 2.1 Web Worker for Data Transformation

**Effort**: 8 hours
**Expected gain**: Eliminate main thread blocking, 60-80% reduction (2.2-3s saved)

- Move row → multi-series transformation off main thread
- Use Comlink for easy worker communication
- Particularly beneficial for large datasets (>1000 rows)

**Files to create**:

- `lib/workers/chart-transform.worker.ts`
- `lib/workers/chart-transform.ts` (shared types)

#### 2.2 Incremental/Chunked Processing

**Effort**: 6 hours
**Expected gain**: Smoother UI, 40-50% reduction in blocking time

- Process data in batches using `requestIdleCallback()`
- Yield to browser between chunks
- Maintain responsiveness during processing

**Implementation**:

```typescript
async function processInChunks(validRows: RadarStats[], chunkSize = 50) {
  const rows: Array<{date: Date; metric: string; value: number}> = [];
  for (let i = 0; i < validRows.length; i += chunkSize) {
    const chunk = validRows.slice(i, i + chunkSize);
    // process chunk...
    await new Promise(resolve => requestIdleCallback(resolve));
  }
  return rows;
}
```

### Phase 3: Rendering Optimization (Week 3, 18 hours)

**Goal**: 70-90% reduction, <400ms execution time
**Expected result**: 300-500ms execution time

#### 3.1 Data Point Decimation/Sampling

**Effort**: 6 hours
**Expected gain**: 50-70% for large datasets (1.8-2.6s saved on 30+ day ranges)

- Implement LTTB (Largest Triangle Three Buckets) algorithm
- Reduce data points for wide date ranges
- Maintain visual fidelity while reducing DOM nodes

**When to apply**:

- > 500 data points (125+ input rows): sample to 250 points
- > 1000 data points (250+ input rows): sample to 400 points

#### 3.2 Virtual Scrolling for Data Table

**Effort**: 4 hours
**Expected gain**: 60-80% reduction in DOM nodes, faster initial render

- Use `svelte-virtual-list` for accessible data table
- Only render visible rows
- Dramatically reduce memory footprint

#### 3.3 Lazy Load Chart Components

**Effort**: 8 hours
**Expected gain**: Better perceived performance, 20-30% actual improvement

- Use `{#await}` to defer chart render until data ready
- Split chart into separate async component
- Show skeleton/placeholder during load

### Phase 4: Advanced Optimizations (Future, 40+ hours)

**Goal**: 90%+ reduction, constant-time rendering
**Expected result**: <200ms regardless of dataset size

#### 4.1 Progressive Rendering

**Effort**: 8 hours
**Expected gain**: Instant initial render, full detail in 2-3 frames

- Render low-resolution chart first (every 10th point)
- Progressively add detail in subsequent frames
- User sees instant feedback

#### 4.2 Canvas Rendering for Large Datasets

**Effort**: 16+ hours (requires layerchart replacement)
**Expected gain**: 70-90% for datasets >1000 points

- Switch from SVG to Canvas for >500 points
- Massive reduction in DOM nodes
- May require custom chart implementation

#### 4.3 Chart Virtualization

**Effort**: 16+ hours
**Expected gain**: Constant rendering time regardless of dataset size

- Only render visible x-axis range
- Dynamically load data as user pans/zooms
- Implement viewport-based rendering

#### 4.4 Backend API Optimization

**Effort**: 8-16 hours (backend work)
**Expected gain**: 70-90% reduction in client-side processing

- Return pre-transformed multi-series data from API
- Eliminate client-side transformation entirely
- Add API parameter: `format=multi-series`

## Implementation Timeline

### Week 1: Quick Wins (November 4-8, 2025)

- ✅ Monday: Document plan (this file)
- 🔲 Monday-Tuesday: Implement memoization cache (1.1)
- 🔲 Wednesday-Thursday: Implement batched updates with rAF (1.2)
- 🔲 Friday: Add loading state (1.3)
- **Deliverable**: 30-50% performance improvement, better UX

### Week 2: Core Optimization (November 11-15, 2025)

- 🔲 Monday-Wednesday: Web Worker implementation (2.1)
- 🔲 Thursday-Friday: Chunked processing (2.2)
- **Deliverable**: <1s chart load time

### Week 3: Rendering Optimization (November 18-22, 2025)

- 🔲 Monday-Tuesday: LTTB decimation (3.1)
- 🔲 Wednesday: Virtual scrolling for table (3.2)
- 🔲 Thursday-Friday: Lazy chart loading (3.3)
- **Deliverable**: <500ms chart load time

### Future Phases (TBD)

- Progressive rendering (Phase 4.1)
- Canvas rendering evaluation (Phase 4.2)
- Chart virtualization (Phase 4.3)
- Backend API changes (Phase 4.4)

## Success Metrics

### Performance Targets

- **Critical (P0)**: `loadChart()` < 500ms for typical datasets (14 days, 4h grouping)
- **Important (P1)**: Main thread blocking < 100ms
- **Nice-to-have (P2)**: No perceived jank when changing grouping

### Measurement Strategy

- Add `performance.mark()` at key points in `loadChart()`
- Log execution time in development builds
- Use Chrome DevTools Performance profiler to validate improvements
- Track metrics in each phase

### Acceptance Criteria

- ✅ Chart updates feel instant when changing grouping
- ✅ No UI freeze during chart render
- ✅ Accessible data table renders smoothly with 1000+ rows
- ✅ Works well on mid-range devices (tested on 2020 MacBook Air)
- ✅ No visual regression (charts look identical)

## Risk Assessment

### Technical Risks

1. **Web Worker overhead**: May not be worth it for small datasets
   - Mitigation: Only use worker for >200 rows
2. **Svelte reactivity complexity**: Batching may break reactive chains
   - Mitigation: Thorough testing of reactive dependencies
3. **LTTB sampling artifacts**: May miss important data spikes
   - Mitigation: Make sampling threshold configurable

### User Experience Risks

1. **Loading states**: Too many spinners may feel worse than smooth delay
   - Mitigation: Use subtle, non-intrusive indicators
2. **Data fidelity**: Sampling may hide anomalies
   - Mitigation: Keep raw data table, only sample chart

## Testing Strategy

### Unit Tests

- Test data transformation logic in isolation
- Test memoization cache behavior
- Test LTTB algorithm accuracy

### Integration Tests

- Verify chart renders correctly with various dataset sizes
- Test reactive updates when changing params
- Verify accessibility features still work

### Performance Tests

- Benchmark each phase improvement
- Test on various dataset sizes: 10, 50, 100, 500, 1000 rows
- Test on lower-end devices

## Monitoring & Rollback

### Feature Flags

- `ENABLE_CHART_WORKER`: Toggle Web Worker (default: true)
- `ENABLE_CHART_SAMPLING`: Toggle LTTB decimation (default: true)
- `CHART_SAMPLE_THRESHOLD`: Minimum points before sampling (default: 500)

### Rollback Plan

Each phase is independently deployable. If issues arise:

1. Disable feature flag
2. Revert specific commit
3. All code is backward compatible

## References

### Performance Profiling Data

- Initial profile: 3,719.8ms total, 3,333.6ms in Function call
- Heavy processes in chunk-NUVYNP6B.js: 186.4ms, 56.5ms, 87.9ms
- Multiple flush_queued_effects: 62.9ms, 54.3ms

### Related Documentation

- [Svelte Performance Best Practices](https://svelte.dev/docs/svelte/performance)
- [LTTB Algorithm](https://github.com/sveinn-steinarsson/flot-downsample)
- [Web Workers in SvelteKit](https://kit.svelte.dev/docs/web-workers)

### Libraries to Evaluate

- `comlink`: Easy Web Worker communication
- `svelte-virtual-list`: Virtual scrolling for large lists
- `d3-downsample`: LTTB implementation for time series
