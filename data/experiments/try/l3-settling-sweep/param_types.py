"""Go-side types of the L3 ema_baseline_v1 keys these experiments vary.

config/tuning.defaults.json can't be used to infer these (closeness_multiplier
is written as `3`, an int, but is a float64 in internal/config/tuning.go), so
they are listed explicitly from that struct. settling-eval's JSON unmarshal is
strict: a float for an int field (or the reverse for a bool) rejects the whole
config, which on 2026-09-18 silently voided half of an interaction grid. Every
script that writes a config from swept values calls coerce() first.
"""

IS_INT = {
    "neighbour_confirmation_count",
    "min_confidence_floor",
    "locked_baseline_threshold",
}
IS_BOOL = {"seed_from_first"}


def coerce(key, value):
    if key in IS_BOOL:
        if isinstance(value, str):
            return value.strip().lower() == "true"
        return bool(value)
    if key in IS_INT:
        as_float = float(value)
        if not as_float.is_integer():
            raise ValueError(f"{key} must be an integer, got {value!r}")
        return int(as_float)
    return float(value)
