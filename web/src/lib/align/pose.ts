/**
 * A sensor's place on the map, and the arithmetic that puts its scan there.
 *
 * The sensor frame is the LiDAR's own: metres, +Y the azimuth zero, +X 90°
 * clockwise from +Y seen from above, +Z up. A pose is the sensor's latitude and
 * longitude and its heading, the bearing of +Y in degrees clockwise from north.
 *
 * Around a site the map is treated as flat: a local east/north frame in metres
 * about an origin, by the equirectangular approximation. Over the 200 m an
 * alignment spans, that is good to millimetres.
 */

export interface Pose {
	lat: number;
	lon: number;
	/** Bearing of the sensor's +Y, degrees clockwise from north. */
	headingDeg: number;
}

export interface EN {
	east: number;
	north: number;
}

const METRES_PER_DEGREE = 111_320;

/** Normalises an angle to [0, 360). */
export function wrapDegrees(deg: number): number {
	const r = deg % 360;
	return r < 0 ? r + 360 : r;
}

/** Metres east and north of origin. */
export function toLocal(origin: { lat: number; lon: number }, lat: number, lon: number): EN {
	const cos = Math.cos((origin.lat * Math.PI) / 180);
	return {
		east: (lon - origin.lon) * METRES_PER_DEGREE * cos,
		north: (lat - origin.lat) * METRES_PER_DEGREE
	};
}

/** The latitude and longitude of a point east and north of origin. */
export function fromLocal(
	origin: { lat: number; lon: number },
	p: EN
): { lat: number; lon: number } {
	const cos = Math.cos((origin.lat * Math.PI) / 180);
	return {
		lat: origin.lat + p.north / METRES_PER_DEGREE,
		lon: origin.lon + p.east / (METRES_PER_DEGREE * cos)
	};
}

/** A sensor-frame point in east/north metres about the sensor, for a heading. */
export function sensorToEN(x: number, y: number, headingDeg: number): EN {
	const t = (headingDeg * Math.PI) / 180;
	const c = Math.cos(t);
	const s = Math.sin(t);
	return { east: x * c + y * s, north: -x * s + y * c };
}

/**
 * The north azimuth site-index.json records, from a heading. It is the scene
 * viewer's measurement, which Gerardus found to put +Y at 180° − N; see
 * docs/ui/visualiser/gerardus-alignment.md. The conversion is its own inverse.
 */
export function northAzimuthFromHeading(headingDeg: number): number {
	return wrapDegrees(180 - headingDeg);
}

/** The heading a recorded north azimuth implies. */
export function headingFromNorthAzimuth(northAzimuthDeg: number): number {
	return wrapDegrees(180 - northAzimuthDeg);
}

/** Applies a small rigid move in the local frame: rotate about the sensor, then shift. */
export function movePose(pose: Pose, dEast: number, dNorth: number, dHeadingDeg: number): Pose {
	const moved = fromLocal(pose, { east: dEast, north: dNorth });
	return { lat: moved.lat, lon: moved.lon, headingDeg: wrapDegrees(pose.headingDeg + dHeadingDeg) };
}
