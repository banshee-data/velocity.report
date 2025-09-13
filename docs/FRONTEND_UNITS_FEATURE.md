# Frontend Units Override Feature

This document describes the implementation of the frontend units override feature that allows users to customize their speed unit preferences independently of the CLI-set units.

## Overview

Users can now override the CLI-set units through the frontend settings page. Their preference is stored in localStorage and used for all API calls, while the database continues to store speeds in m/s.

## Architecture

### 1. API Layer Changes

#### Enhanced `/api/radar_stats` endpoint
- **New Parameter**: `units` (optional query parameter)
- **Valid Values**: `mps`, `mph`, `kmph`, `kph`
- **Behavior**: Overrides the CLI-set units for this specific request
- **Example**: `/api/radar_stats?days=14&units=kmph`

#### Enhanced `/api/events` endpoint
- **New Parameter**: `units` (optional query parameter)
- **Behavior**: Same as radar_stats, applies unit conversion to speed fields
- **Example**: `/api/events?units=mph`

#### Validation
- Invalid units return HTTP 400 with error message
- If no units parameter provided, uses CLI-set default

### 2. Frontend Implementation

#### Units Management (`/src/lib/units.ts`)
```typescript
export type Unit = 'mps' | 'mph' | 'kmph' | 'kph';

// localStorage management
export function getStoredUnits(): Unit | null
export function setStoredUnits(units: Unit): void
export function getDisplayUnits(defaultUnits: string): Unit

// UI helpers
export function getUnitLabel(unit: Unit): string
export const AVAILABLE_UNITS: { value: Unit; label: string }[]
```
# Frontend Units & Timezone Feature

This document describes the implementation of the frontend units override feature and the related timezone handling that allows users to customize their display preferences independently of the CLI-set defaults.

## Overview

Users can now override the CLI-set units and timezone through the frontend settings page. Their preferences are stored in localStorage and used for all API calls that accept overrides, while the database continues to store canonical data (speeds in m/s, timestamps in UTC).

## Architecture

### 1. API Layer Changes

#### Enhanced `/api/radar_stats` endpoint
- **New Parameters**:
  - `units` (optional query parameter)
  - `timezone` (optional query parameter)
- **Valid Units**: `mps`, `mph`, `kmph`, `kph`
- **Valid Timezones**: any tz database timezone (commonly we surface a curated list such as `UTC`, `US/Eastern`, `Europe/London`, etc.)
- **Behavior**: Each parameter overrides the CLI-set default for that specific request. If omitted, the server default (CLI flag) is used.
- **Example**: `/api/radar_stats?days=14&units=kmph&timezone=Europe/Paris`

#### Enhanced `/api/events` endpoint
- **New Parameters**: `units`, `timezone` (optional)
- **Behavior**: Same as `radar_stats`; applies unit conversion to speed fields and (when timestamps are available) will return timestamps converted to the requested timezone.

#### Validation
- Invalid units return HTTP 400 with an error message listing valid units.
- Invalid timezones return HTTP 400 with a helpful message and a list of commonly suggested timezones (the server actually validates against the system tz database).

### 2. Frontend Implementation

#### Units Management (`/src/lib/units.ts`)
```typescript
export type Unit = 'mps' | 'mph' | 'kmph' | 'kph';

// localStorage management
export function getStoredUnits(): Unit | null
export function setStoredUnits(units: Unit): void
export function getDisplayUnits(defaultUnits: string): Unit

// UI helpers
export function getUnitLabel(unit: Unit): string
export const AVAILABLE_UNITS: { value: Unit; label: string }[]
```

#### Timezone Management (`/src/lib/timezone.ts`)
```typescript
export type Timezone = string; // validated against tz database on server

export function getStoredTimezone(): Timezone | null
export function setStoredTimezone(tz: Timezone): void
export function getDisplayTimezone(defaultTimezone: string): Timezone
export function getTimezoneLabel(tz: Timezone): string
export const AVAILABLE_TIMEZONES: { value: Timezone; label: string }[]
```

#### State Management (`/src/lib/stores/units.ts` and `/src/lib/stores/timezone.ts`)
```typescript
// Svelte stores for reactive preferences
export const displayUnits = writable<Unit>('mph');
export const displayTimezone = writable<Timezone>('UTC');

// Initialize from server config + localStorage
export function initializeUnits(serverDefault: string): Unit
export function initializeTimezone(serverDefault: string): Timezone

// Update preference and persist to localStorage
export function updateUnits(newUnits: Unit): void
export function updateTimezone(newTimezone: Timezone): void
```

#### Settings Page (`/src/routes/settings/+page.svelte`)
- **Features**:
  - Dropdowns to select preferred units and timezone
  - Shows server default vs user preference in a compact table
  - Auto-save behavior when selections change
  - Visual confirmation messages and validation feedback

#### Dashboard Updates (`/src/routes/+page.svelte`)
- **Features**:
  - Reactive to units and timezone changes
  - Automatically reloads data when preferences change (`/api/radar_stats?units=USER_PREF&timezone=USER_TZ`)
  - Displays proper unit labels and timezone-aware timestamps

### 3. Data Flow

1. Page loads → Fetch server config (`/api/config`)
2. Initialize stores (server defaults + localStorage overrides)
3. Load data with user's preferred units/timezone (`/api/radar_stats?units=USER_PREF&timezone=USER_TZ`)
4. User changes settings → Update localStorage + store
5. Dashboard reactively reloads with new units/timezone

### 4. User Experience

#### Initial Load
1. App loads with CLI-set defaults for units and timezone
2. If user has saved preferences, those override immediately
3. Dashboard shows data in user's preferred units and timestamps converted to their timezone

#### Changing Preferences
1. Navigate to Settings via sidebar
2. Select new units and/or timezone from dropdowns
3. Preferences are saved automatically (no explicit Save button required)
4. Dashboard reloads data using the updated preferences

#### Reset to Default
1. In Settings, clear the stored preference (Reset)
2. Clears localStorage and store value falls back to server CLI default

## Examples

### CLI vs Frontend Preferences
```bash
# Server started with km/h and UTC timezone
./radar --dev --units kmph --timezone UTC

# User can override to mph and Europe/Paris in frontend
# API calls will use: /api/radar_stats?units=mph&timezone=Europe/Paris
```

### API Usage
```javascript
// Get data in user's preferred units/timezone
const stats = await getRadarStats('mph', 'Europe/Paris');

// Get data in specific units/timezone (override)
const statsInKmh = await getRadarStats('kmph', 'UTC');

// Get data in server defaults (no override)
const defaultStats = await getRadarStats();
```

### localStorage Structure
```json
{
  "velocity-report-units": "mph",
  "velocity-report-timezone": "Europe/Paris"
}
```

## Testing

### API Tests
```
# Units override
curl "http://localhost:8080/api/radar_stats?units=kmph"
curl "http://localhost:8080/api/radar_stats?units=mph"

# Timezone override
curl "http://localhost:8080/api/radar_stats?timezone=Europe/Paris"

# Validation
curl "http://localhost:8080/api/radar_stats?units=invalid"
# Returns: {"error":"Invalid 'units' parameter. Must be one of: mps, mph, kmph, kph"}

curl "http://localhost:8080/api/config?timezone=Invalid/Timezone"
# Returns: {"error":"Invalid 'timezone' parameter. Must be one of: <common tz list>"}
```

### Unit/Timezone Conversion Verification
- Speed conversions (unit tests): 26.19 mph ↔ 42.16 km/h ↔ 11.7 m/s
- Timezone conversions: verify timestamps are converted from UTC into requested timezone (using a fixed UTC timestamp in tests)

## Benefits

1. **User Choice**: Users can choose their preferred units and timezone
2. **Persistence**: Preferences saved across sessions
3. **No Data Migration**: Database keeps canonical formats (m/s, UTC)
4. **Backward Compatible**: Works with existing CLI defaults
5. **Real-time**: Changes apply immediately without restart
6. **Consistent**: Same conversion and timezone logic across all endpoints
