/**
 * Types for the capture index: what is on a capture volume, what changed since
 * the last look, and which files form a continuous session.
 *
 * These mirror the Go structures served by /api/lidar/capture/*. Times arrive
 * as nanoseconds since the epoch because that is what the capture clock is
 * recorded in; convert with nsToDate rather than by dividing inline.
 */

/** A configured capture volume. */
export interface CaptureRoot {
	root_id: string;
	path: string;
	label?: string;
	enabled: boolean;
	last_scan_at_ns?: number;
	/** never | ok | unreachable | error */
	last_scan_state: string;
	last_scan_error?: string;
	created_at_ns: number;
	updated_at_ns: number;
}

/** Scan states a root can report. */
export const SCAN_NEVER = 'never';
export const SCAN_OK = 'ok';
export const SCAN_UNREACHABLE = 'unreachable';
export const SCAN_ERROR = 'error';

/** Probe states a capture file can be in. */
export const PROBE_PENDING = 'pending';
export const PROBE_OK = 'ok';
export const PROBE_FAILED = 'failed';

/** One capture file as the index records it. */
export interface CaptureFile {
	capture_file_id: string;
	root_id: string;
	rel_path: string;
	size_bytes: number;
	modified_at_ns: number;
	content_tag?: string;
	first_packet_ns?: number;
	last_packet_ns?: number;
	packet_count?: number;
	udp_port?: number;
	probe_state: string;
	probe_error?: string;
	probed_at_ns?: number;
	present: boolean;
	first_seen_at_ns: number;
	last_seen_at_ns: number;
	session_id?: string;
}

/** A derived run of contiguous capture files. */
export interface CaptureSession {
	session_id: string;
	root_id: string;
	label?: string;
	sensor_id?: string;
	file_count: number;
	start_ns: number;
	end_ns: number;
	covered_ns: number;
	lost_ns: number;
	/** seamless | acceptable | broken | overlap — never worse than acceptable. */
	worst_seam: string;
	size_bytes: number;
	derived_at_ns: number;
}

/** One stretch of a session sharing a motion/static classification. */
export interface MotionPeriod {
	period_id: string;
	session_id: string;
	ordinal: number;
	/** motion | static */
	type: string;
	label: string;
	start_ns: number;
	end_ns: number;
	duration_ns: number;
	start_secs: number;
	end_secs: number;
	start_frame?: number;
	end_frame?: number;
	created_at_ns: number;
}

/** Background work over the capture index. */
export interface CaptureJob {
	job_id: string;
	kind: string;
	session_id?: string;
	root_id?: string;
	/** queued | running | completed | failed | cancelled */
	state: string;
	progress_current: number;
	progress_total: number;
	detail?: string;
	error?: string;
	queued_at_ns: number;
	started_at_ns?: number;
	finished_at_ns?: number;
}

/** Job states. */
export const JOB_QUEUED = 'queued';
export const JOB_RUNNING = 'running';
export const JOB_COMPLETED = 'completed';
export const JOB_FAILED = 'failed';
export const JOB_CANCELLED = 'cancelled';

/** What one root's scan found. */
export interface ScanRootResult {
	root_id: string;
	path: string;
	state: string;
	error?: string;
	drift: string;
	added: number;
	missing: number;
	changed: number;
	probed: number;
	probe_failed: number;
	sessions?: CaptureSession[];
}

export interface ScanResponse {
	roots: ScanRootResult[];
	count: number;
}

export interface PeriodsResponse {
	session_id: string;
	periods: MotionPeriod[];
	count: number;
	static_seconds: number;
	motion_seconds: number;
}

/**
 * Geographic identity, per docs/lidar/architecture/geographic-indexing.md.
 *
 * The canonical tokens are the identifiers — what is stored, indexed and
 * exchanged. The family displays are derived presentation: they carry one
 * hyphen at the family boundary, may appear in UI and logs, and must never be
 * sent back as a key or used to address anything.
 */
export interface CaseLocation {
	/**
	 * The case's own sensor pose: where the car was for this one visit. Free
	 * to differ from the site's canonical pose, and from another case's visit
	 * to the same site.
	 */
	origin_lat: number;
	origin_lon: number;
	/**
	 * Canonical S2 tokens. L16 is the site — a junction and its approaches,
	 * about 180 m across. L13 is the neighbourhood around it and L10 the
	 * district, which is the archive-scale roll-up.
	 */
	s2_l10_token: string;
	s2_l13_token: string;
	s2_l16_token: string;
	/** Family displays, derived by the server on read. Presentation only. */
	s2_l10_display: string;
	s2_l13_display: string;
	s2_l16_display: string;
	/** The site this case is at — equal to s2_l16_token. */
	site_id: string;
	/** surveyed | operator | fix */
	geographic_source?: string;
	/** located | unavailable */
	geographic_status: string;
}

export const GEO_LOCATED = 'located';
export const GEO_UNAVAILABLE = 'unavailable';

/** How a case's sensor pose was established. */
export const GEO_SOURCES = ['surveyed', 'operator', 'fix'] as const;
export type GeoSource = (typeof GEO_SOURCES)[number];

/** How a site's canonical pose was established — never a fix: a canonical
 * pose is by definition not one sensor's reading on one visit. */
export const SITE_SOURCES = ['surveyed', 'operator'] as const;
export type SiteSource = (typeof SITE_SOURCES)[number];

/** One replay case as a site lists it — its own sensor pose for that visit. */
export interface SiteCase {
	replay_case_id: string;
	description?: string;
	sensor_id?: string;
	origin_lat: number;
	origin_lon: number;
	created_at_ns: number;
}

/**
 * A site is one L16 cell: a junction and its approaches, about 180 m across.
 * It exists once any case has been located there, and its own canonical pose
 * — a fixed point such as the midpoint of the intersection — is separate from
 * any one case's sensor pose, and unset until someone (a surveyor or an
 * operator) records it deliberately.
 */
export interface LidarSite {
	s2_l16_token: string;
	s2_l13_token: string;
	s2_l10_token: string;
	label?: string;
	canonical_lat?: number;
	canonical_lon?: number;
	/** surveyed | operator — absent when no canonical pose has been set. */
	canonical_source?: string;
	created_at_ns: number;
	updated_at_ns?: number;
	cases: SiteCase[];
}

/**
 * One area captures were taken in. The grouping is the L10 cell, which is
 * district-scale, so an entry typically holds several distinct sites.
 */
export interface SceneArea {
	s2_l10_token: string;
	s2_l10_display: string;
	centre_lat: number;
	centre_lon: number;
	sw_lat: number;
	sw_lon: number;
	ne_lat: number;
	ne_lon: number;
	case_count: number;
	/** Distinct L13 cells within the area. */
	neighbourhood_count: number;
	sites: LidarSite[];
}

export interface SceneMapResponse {
	areas: SceneArea[];
	area_count: number;
	site_count: number;
	case_count: number;
	coarse_level: number;
	fine_level: number;
	precise_level: number;
}
