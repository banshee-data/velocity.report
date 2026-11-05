"""The city frame Gerardus's city tiles are drawn in, and a site's place in it.

Gerardus (banshee-data/gerardus, src/city_tiles.rs, format
gerardus-city-tiles-1) bakes San Francisco's ground, footprints and streets as
3D Tiles in one fixed frame: east-north-up at latitude 37.76, longitude
-122.44, NAD83(2011) horizontally, NAVD88 heights passed to the ENU
conversion as if they were ellipsoidal. So z is NAVD88 height less the
earth's curvature away from the origin, about d^2 / 2R: -2.9 m at 6 km. The
tileset's root transform carries the frame onto ECEF with NAVD88 zero at an
ellipsoidal height of about -32.2 m; that offset is for placing the city on
the globe and touches no coordinate in the frame.

A Gerardus site frame is built the same way at the centre of the site's S2
L16 cell: east-north-up, NAD83(2011), NAVD88 zero at the origin, heights
taken as ellipsoidal (src/site.rs, src/geojson.rs Frame). So a site-frame
point goes into the city frame through ECEF exactly, with no datum step:

    p_city = R_city * (ECEF_site_origin - ECEF_city_origin) + R_city * R_site^T * p_site

which is a rotation (the two frames' meridians differ by a few hundredths of
a degree across the city) and a translation. This is the one place a client's
placement is computed; clients multiply matrices and do no geodesy.

Everything is double precision, GRS80, and ported from Gerardus's src/geo.rs
so the two agree to well under a millimetre. The S2 decoding is the cell
centre only, as Gerardus's cell::centre (the s2 crate's LatLng::from(CellID)).
"""

import math

GRS80_A = 6_378_137.0
GRS80_F = 1.0 / 298.257_222_101
_E2 = GRS80_F * (2.0 - GRS80_F)

# The city origin (Gerardus CITY-VIEW-TILES T0, 2026-11-04). It never moves
# between epochs, so placements in the city frame stay valid.
CITY_FORMAT = "gerardus-city-tiles-1"
CITY_ORIGIN_LAT = 37.76
CITY_ORIGIN_LON = -122.44
# NAVD88 zero as an ellipsoidal height at the origin (GEOID18, about +-0.3 m),
# used by the tileset's root transform only.
NAVD88_ZERO_ELLIPSOIDAL_M = -32.2

SITE_LEVEL = 16


# =============================================================================
# GEODESY (GRS80), as Gerardus src/geo.rs
# =============================================================================


def geodetic_to_ecef(lat_deg, lon_deg, h):
    sl, cl = math.sin(math.radians(lat_deg)), math.cos(math.radians(lat_deg))
    so, co = math.sin(math.radians(lon_deg)), math.cos(math.radians(lon_deg))
    n = GRS80_A / math.sqrt(1.0 - _E2 * sl * sl)
    return ((n + h) * cl * co, (n + h) * cl * so, (n * (1.0 - _E2) + h) * sl)


def ecef_to_geodetic(p):
    lon = math.atan2(p[1], p[0])
    r = math.hypot(p[0], p[1])
    lat = math.atan2(p[2], r * (1.0 - _E2))
    h = 0.0
    for _ in range(5):
        s = math.sin(lat)
        n = GRS80_A / math.sqrt(1.0 - _E2 * s * s)
        h = r / math.cos(lat) - n
        lat = math.atan2(p[2], r * (1.0 - _E2 * n / (n + h)))
    return math.degrees(lat), math.degrees(lon), h


def enu_rows(lat_deg, lon_deg):
    """East, north and up unit vectors in ECEF: the rows of ECEF-to-ENU."""
    sl, cl = math.sin(math.radians(lat_deg)), math.cos(math.radians(lat_deg))
    so, co = math.sin(math.radians(lon_deg)), math.cos(math.radians(lon_deg))
    return ((-so, co, 0.0), (-sl * co, -sl * so, cl), (cl * co, cl * so, sl))


class Enu:
    """A local east-north-up frame at a geodetic origin."""

    def __init__(self, lat_deg, lon_deg, h=0.0):
        self.lat_deg, self.lon_deg, self.h = lat_deg, lon_deg, h
        self.origin = geodetic_to_ecef(lat_deg, lon_deg, h)
        self.rows = enu_rows(lat_deg, lon_deg)

    def from_ecef(self, p):
        d = [p[i] - self.origin[i] for i in range(3)]
        return [sum(r[i] * d[i] for i in range(3)) for r in self.rows]

    def from_geodetic(self, lat_deg, lon_deg, h=0.0):
        return self.from_ecef(geodetic_to_ecef(lat_deg, lon_deg, h))

    def to_ecef(self, enu):
        r = self.rows
        return [
            self.origin[i] + r[0][i] * enu[0] + r[1][i] * enu[1] + r[2][i] * enu[2]
            for i in range(3)
        ]

    def to_geodetic(self, enu):
        return ecef_to_geodetic(self.to_ecef(enu))


def _helmert_at(epoch):
    """ITRF2014 -> NAD83(2011): translation, rotation, scale at `epoch`."""
    arcsec = math.pi / (180.0 * 3600.0)
    dt = epoch - 2010.0
    t = (1.0053 + 0.0008 * dt, -1.9092 - 0.0006 * dt, -0.5416 - 0.0014 * dt)
    rx = (0.0267814 + 6.67e-5 * dt) * arcsec
    ry = (-0.0004203 - 0.0007574 * dt) * arcsec
    rz = (0.0109321 - 5.13e-5 * dt) * arcsec
    s = 1.0 + (0.00037 - 7e-5 * dt) * 1e-6
    return t, ((1.0, rz, -ry), (-rz, 1.0, rx), (ry, -rx, 1.0)), s


def nad83_2011_to_itrf2014(lat_deg, lon_deg, h, epoch):
    """NAD83(2011) to ITRF2014 (WGS 84 to a few cm), as Gerardus's inverse."""
    t, r, s = _helmert_at(epoch)
    q = geodetic_to_ecef(lat_deg, lon_deg, h)
    d = [(q[i] - t[i]) / s for i in range(3)]
    p = [r[0][i] * d[0] + r[1][i] * d[1] + r[2][i] * d[2] for i in range(3)]
    return ecef_to_geodetic(p)


# =============================================================================
# S2 CELL CENTRES
# =============================================================================

# Hilbert-curve tables (S2 kPosToIJ, kPosToOrientation).
_POS_TO_IJ = ((0, 1, 3, 2), (0, 2, 3, 1), (3, 2, 0, 1), (3, 1, 0, 2))
_POS_TO_ORIENTATION = (1, 0, 0, 3)


def cell_id(token):
    if not token or len(token) > 16:
        raise ValueError(f"bad S2 token {token!r}")
    value = int(token.ljust(16, "0"), 16)
    if value == 0 or (value >> 61) > 5:
        raise ValueError(f"bad S2 token {token!r}")
    return value


def cell_level(cid):
    trailing = (cid & -cid).bit_length() - 1
    if trailing % 2:
        raise ValueError(f"bad S2 cell id {cid:#x}")
    return 30 - trailing // 2


def _st_to_uv(s):
    if s >= 0.5:
        return (1.0 / 3.0) * (4.0 * s * s - 1.0)
    return (1.0 / 3.0) * (1.0 - 4.0 * (1.0 - s) * (1.0 - s))


def _face_uv_to_xyz(face, u, v):
    return (
        (1.0, u, v),
        (-u, 1.0, v),
        (-u, -v, 1.0),
        (-1.0, -v, -u),
        (v, -1.0, -u),
        (v, u, -1.0),
    )[face]


def cell_centre(token):
    """(lat, lon) in degrees of an S2 cell's centre, as the s2 crate gives it."""
    cid = cell_id(token)
    level = cell_level(cid)
    face = cid >> 61
    orientation = face & 1
    i = j = 0
    for k in range(level):
        pos = (cid >> (59 - 2 * k)) & 3
        ij = _POS_TO_IJ[orientation][pos]
        i = (i << 1) | (ij >> 1)
        j = (j << 1) | (ij & 1)
        orientation ^= _POS_TO_ORIENTATION[pos]
    size = float(1 << level)
    x, y, z = _face_uv_to_xyz(
        face, _st_to_uv((i + 0.5) / size), _st_to_uv((j + 0.5) / size)
    )
    return math.degrees(math.atan2(z, math.hypot(x, y))), math.degrees(math.atan2(y, x))


# =============================================================================
# SITE TO CITY
# =============================================================================


def city_frame():
    return Enu(CITY_ORIGIN_LAT, CITY_ORIGIN_LON, 0.0)


def site_origin(l16):
    """A Gerardus site frame's origin: its L16 cell centre at NAVD88 0."""
    cid = cell_id(l16)
    if cell_level(cid) != SITE_LEVEL:
        raise ValueError(f"{l16} is not an S2 L{SITE_LEVEL} token")
    return cell_centre(l16)


def site_to_city(l16):
    """The 4x4 row-major matrix taking site-frame metres to city-frame metres."""
    lat, lon = site_origin(l16)
    city = city_frame()
    site = Enu(lat, lon, 0.0)
    t = city.from_ecef(site.origin)
    # R_city * R_site^T: column c is the site's axis c expressed in city axes.
    r = [
        [sum(city.rows[a][i] * site.rows[c][i] for i in range(3)) for c in range(3)]
        for a in range(3)
    ]
    return [r[0] + [t[0]], r[1] + [t[1]], r[2] + [t[2]], [0.0, 0.0, 0.0, 1.0]]


def compose(a, b):
    """a * b for 4x4 row-major matrices."""
    return [
        [sum(a[i][k] * b[k][j] for k in range(4)) for j in range(4)] for i in range(4)
    ]


def apply(m, p):
    return [
        m[i][0] * p[0] + m[i][1] * p[1] + m[i][2] * p[2] + m[i][3] for i in range(3)
    ]


def pose_matrix(rotation, translation):
    """The 4x4 row-major matrix p_site = R p_sensor + t."""
    return [list(rotation[i]) + [translation[i]] for i in range(3)] + [
        [0.0, 0.0, 0.0, 1.0]
    ]


def root_transform():
    """The tileset root transform (city frame to ECEF), column-major, as Gerardus writes it."""
    r = enu_rows(CITY_ORIGIN_LAT, CITY_ORIGIN_LON)
    o = geodetic_to_ecef(CITY_ORIGIN_LAT, CITY_ORIGIN_LON, NAVD88_ZERO_ELLIPSOIDAL_M)
    return [
        r[0][0], r[0][1], r[0][2], 0.0,
        r[1][0], r[1][1], r[1][2], 0.0,
        r[2][0], r[2][1], r[2][2], 0.0,
        o[0], o[1], o[2], 1.0,
    ]  # fmt: skip


def check_tileset(tileset, rotation_tolerance=1e-12, translation_tolerance_m=1e-3):
    """Refuse a tileset whose root transform is not this module's city frame.

    The origin is fixed across epochs, so a different root means a different
    frame, and every placement computed here would be wrong in it.
    """
    root = tileset.get("root", {}).get("transform")
    if not root or len(root) != 16:
        raise ValueError("tileset has no root transform")
    want = root_transform()
    rotation_off = max(abs(root[i] - want[i]) for i in range(12))
    translation_off = max(abs(root[i] - want[i]) for i in range(12, 15))
    if rotation_off > rotation_tolerance or translation_off > translation_tolerance_m:
        raise ValueError(
            f"tileset root transform is not the {CITY_FORMAT} city frame"
            f" (rotation off by {rotation_off:g}, origin by {translation_off:g} m)"
        )
