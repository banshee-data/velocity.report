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
