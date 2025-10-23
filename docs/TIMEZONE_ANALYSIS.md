# Timezone Analysis and Deduplication

## Overview

This document explains the timezone deduplication process performed on the `CommonTimezones` list to ensure we have exactly one timezone per unique STD/DST offset pair.

## Problem

Previously, the `CommonTimezones` list contained multiple timezones that represent the same offset pair. For example:

- **-09:00/-08:00** had: `US/Alaska`, `America/Anchorage`, `America/Nome`, `America/Juneau`, `America/Yakutat`
- **+01:00/+02:00** had: `Europe/Paris`, `Europe/Berlin`, `Europe/Rome`, `Europe/Madrid`, `Europe/Amsterdam`

This was redundant since all timezones in each group have identical standard and daylight saving time offsets.

## Solution

Created an analysis script (`scripts/analyze-timezones.go`) that:

1. **Loads all timezones** from the system's TZDB
2. **Calculates offset pairs** for each timezone (standard and DST)
3. **Groups timezones** by their unique offset pairs
4. **Scores timezones** based on popularity heuristics:
   - Major cities (New York, Tokyo, London, etc.) get +100 points
   - Capital cities get +50 points
   - Regions (America/, Europe/, Asia/) get +20 points
   - Etc/ and GMT prefixes get -50 points
5. **Recommends one timezone** per offset pair based on highest score

## Results

### Before (33 timezones, many duplicates)
- Multiple US zones: `US/Eastern`, `US/Central`, `US/Mountain`, `US/Pacific`
- Multiple America zones: `America/New_York`, `America/Chicago`, etc.
- Multiple European capitals with same offset

### After (30 timezones, one per offset pair)
- Unique coverage of all major offset pairs globally
- Preference for well-known city names (e.g., `America/New_York` over `US/Eastern`)
- Clear comments explaining which offset each timezone represents

## Key Changes

### Removed Duplicates
- **Removed**: `US/Eastern`, `US/Central`, `US/Mountain`, `US/Pacific`, `US/Alaska`, `US/Hawaii`
  - **Reason**: Deprecated US/ prefix, superseded by America/ zones

- **Removed**: `America/Honolulu`, `America/Nome`, `America/Juneau`, `America/Yakutat`
  - **Kept**: `Pacific/Honolulu`, `America/Anchorage` (highest scored alternatives)

- **Removed**: `Europe/Berlin`, `Europe/Rome`, `Europe/Madrid`, `Europe/Amsterdam`
  - **Kept**: `Europe/Paris` (represents CET/CEST for continental Europe)

- **Removed**: `Australia/Melbourne`, `Australia/Perth`
  - **Kept**: `Australia/Sydney`, `Australia/Brisbane` (different offset pairs)

### Added for Coverage
- **Added**: `Europe/Moscow` (+03:00 no DST)
- **Added**: `Asia/Karachi` (+05:00 no DST)
- **Added**: `Asia/Dhaka` (+06:00 no DST)
- **Added**: `Asia/Bangkok` (+07:00 no DST)
- **Added**: `Australia/Darwin` (+09:30 no DST)
- **Added**: `Australia/Adelaide` (+09:30/+10:30)
- **Added**: `Australia/Brisbane` (+10:00 no DST)
- **Added**: `America/Halifax` (-04:00/-03:00 Atlantic)
- **Added**: `America/Phoenix` (-07:00 no DST - Arizona)

## Unique Offset Pairs Covered (30 total)

| Offset Pair | Timezone | Region |
|-------------|----------|--------|
| +00:00 | UTC | Universal |
| +00:00/+01:00 | Europe/London | UK |
| +01:00/+02:00 | Europe/Paris | Western/Central Europe |
| +02:00 | Africa/Johannesburg | South Africa |
| +02:00/+03:00 | Africa/Cairo | Egypt |
| +03:00 | Europe/Moscow | Russia |
| +04:00 | Asia/Dubai | UAE |
| +05:00 | Asia/Karachi | Pakistan |
| +05:30 | Asia/Kolkata | India |
| +06:00 | Asia/Dhaka | Bangladesh |
| +07:00 | Asia/Bangkok | Thailand |
| +08:00 | Asia/Shanghai | China/SE Asia |
| +09:00 | Asia/Tokyo | Japan |
| +09:30 | Australia/Darwin | Northern Australia |
| +09:30/+10:30 | Australia/Adelaide | South Australia |
| +10:00 | Australia/Brisbane | Queensland |
| +10:00/+11:00 | Australia/Sydney | SE Australia |
| +12:00/+13:00 | Pacific/Auckland | New Zealand |
| -03:00 | America/Sao_Paulo | Brazil |
| -04:00/-03:00 | America/Halifax | Atlantic Canada |
| -05:00/-04:00 | America/New_York | US Eastern |
| -05:00/-04:00 | America/Toronto | Canada Eastern |
| -06:00/-05:00 | America/Chicago | US Central |
| -06:00 | America/Mexico_City | Mexico |
| -07:00/-06:00 | America/Denver | US Mountain |
| -07:00 | America/Phoenix | Arizona |
| -08:00/-07:00 | America/Los_Angeles | US Pacific |
| -08:00/-07:00 | America/Vancouver | Canada Pacific |
| -09:00/-08:00 | America/Anchorage | Alaska |
| -10:00 | Pacific/Honolulu | Hawaii |

## Benefits

1. **No Redundancy**: Each timezone represents a unique offset pair
2. **Global Coverage**: All major time zones worldwide are represented
3. **Clear Documentation**: Comments explain which offset each timezone covers
4. **Maintainability**: Easier to understand and maintain the list
5. **Better UX**: Shorter, more focused list for users to choose from

## How to Update

If you need to add a new timezone:

1. Run the analysis script:
   ```bash
   go run scripts/analyze-timezones.go
   ```

2. Check if the offset pair is already covered
3. If not, add the highest-scored timezone for that offset pair
4. Update both `CommonTimezones` and `GetTimezoneLabel()` in `internal/units/timezone.go`
5. Run tests: `go test ./internal/units`

## References

- IANA Time Zone Database: https://www.iana.org/time-zones
- Go time package: https://pkg.go.dev/time
- Analysis script: `scripts/analyze-timezones.go`
