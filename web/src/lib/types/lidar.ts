// src/lib/types/lidar.ts
// TypeScript type definitions for LiDAR tracking system

/**
 * Track represents a tracked object in the LiDAR system.
 */
export interface Track {
	/** Unique identifier for the track */
	track_id: string;
	/** Sensor that detected the track */
	sensor_id: string;
	/** Current track state: tentative (unconfirmed), confirmed, or deleted */
	state: 'tentative' | 'confirmed' | 'deleted';
	/** Current position in world frame (meters) */
	position: { x: number; y: number; z: number };
	/** Current velocity (meters/second) */
	velocity: { vx: number; vy: number };
	/** Current speed (meters/second) */
	speed_mps: number;
	/** Current heading (radians, world frame) */
	heading_rad: number;
	/** Classified object type (optional) */
	object_class?: 'pedestrian' | 'car' | 'bird' | 'other';
	/** Confidence in object classification (0-1, optional) */
	object_confidence?: number;
	/** Number of observations for this track */
	observation_count: number;
	/**
	 * Age of track in seconds (computed as last_seen - first_seen on the backend).
	 * This represents the duration the track has existed.
	 */
	age_seconds: number;
	/** Average speed over track lifetime (meters/second) */
	avg_speed_mps: number;
	/** Peak speed observed (meters/second) */
	peak_speed_mps: number;
	/** Average bounding box dimensions (meters) */
	bounding_box: {
		/** Average length (meters) */
		length_avg: number;
		/** Average width (meters) */
		width_avg: number;
		/** Average height (meters) */
		height_avg: number;
	};
	/** ISO 8601 timestamp when track was first seen */
	first_seen: string;
	/** ISO 8601 timestamp when track was last seen */
	last_seen: string;
	/** Historical positions for trail visualization (optional) */
	history?: {
		x: number;
		y: number;
		timestamp: string;
	}[];
}

/**
 * TrackObservation represents a single observation of a tracked object.
 */
export interface TrackObservation {
	/** Track identifier this observation belongs to */
	track_id: string;
	/** ISO 8601 timestamp of this observation */
	timestamp: string;
	/** Position in world frame at this observation (meters) */
	position: { x: number; y: number; z: number };
	/** Velocity at this observation (meters/second) */
	velocity: { vx: number; vy: number };
	/** Speed at this observation (meters/second) */
	speed_mps: number;
	/** Heading at this observation (radians, world frame) */
	heading_rad: number;
	/** Bounding box dimensions at this observation (meters) */
	bounding_box: {
		length: number;
		width: number;
		height: number;
	};
}

/**
 * BackgroundGrid represents the learned background model from the LiDAR sensor.
 * Used for visualizing the static environment structure.
 */
export interface BackgroundGrid {
	/** Sensor identifier */
	sensor_id: string;
	/** ISO 8601 timestamp of grid snapshot */
	timestamp: string;
	/** Number of elevation rings in the sensor */
	rings: number;
	/** Number of azimuth bins in the grid */
	azimuth_bins: number;
	/** Background cell data */
	cells: BackgroundCell[];
}

/**
 * BackgroundCell represents a single cell in the background grid.
 * Contains learned statistics about the static environment.
 */
export interface BackgroundCell {
	/** Cartesian X position (meters) */
	x: number;
	/** Cartesian Y position (meters) */
	y: number;
	/** Variability in range measurements (meters) - stability indicator */
	range_spread_meters: number;
	/** Number of times this cell has been observed */
	times_seen: number;
	// Legacy fields (optional, for backward compatibility)
	ring?: number;
	azimuth_deg?: number;
	average_range_meters?: number;
}

export interface TrackListResponse {
	tracks: Track[];
	count: number;
	timestamp: string;
}

export interface TrackHistoryResponse {
	tracks: Track[];
	observations: Record<string, TrackObservation[]>;
}

export interface ObservationListResponse {
	observations: TrackObservation[];
	count: number;
	timestamp: string;
}

export interface ClusterResponse {
	cluster_id: number;
	sensor_id: string;
	timestamp: string;
	centroid: { x: number; y: number; z: number };
	bounding_box: {
		length: number;
		width: number;
		height: number;
	};
	points_count: number;
	height_p95: number;
	intensity_mean: number;
}

export interface TrackSummaryResponse {
	by_class: Record<string, TrackClassSummary>;
	by_state: Record<string, TrackStateSummary>;
	total_tracks: number;
	timestamp: string;
}

export interface TrackClassSummary {
	count: number;
	avg_speed_mps: number;
	max_speed_mps: number;
	avg_duration_seconds: number;
}

export interface TrackStateSummary {
	count: number;
	avg_age_seconds: number;
}

// Color scheme for track visualization
export const TRACK_COLORS = {
	pedestrian: '#4CAF50', // Green
	car: '#2196F3', // Blue
	bird: '#FFC107', // Amber
	other: '#9E9E9E', // Grey
	tentative: '#FF9800', // Orange (unconfirmed)
	deleted: '#F44336' // Red (just deleted)
} as const;

/** Classification labels for track identity (single-select: what is the object?) */
export type DetectionLabel = 'car' | 'ped' | 'noise';

/** Quality flags for track attributes (multi-select: properties of the track) */
export type QualityLabel =
	| 'good'
	| 'noisy'
	| 'jitter_velocity'
	| 'merge'
	| 'split'
	| 'truncated'
	| 'disconnected';

/** Scene represents a PCAP-based evaluation environment */
export interface LidarScene {
	scene_id: string;
	sensor_id: string;
	pcap_file: string;
	pcap_start_secs?: number;
	pcap_duration_secs?: number;
	description?: string;
	reference_run_id?: string;
	optimal_params_json?: string;
	created_at_ns: number;
	updated_at_ns?: number;
}

/** Analysis run with track snapshots */
export interface AnalysisRun {
	run_id: string;
	created_at: string;
	source_type: string;
	source_path?: string;
	sensor_id: string;
	params_json?: Record<string, unknown>;
	duration_secs?: number;
	total_frames?: number;
	total_clusters?: number;
	total_tracks: number;
	confirmed_tracks: number;
	processing_time_ms?: number;
	status: string;
	error_message?: string;
	parent_run_id?: string;
	notes?: string;
}

/** Run track with labelling fields */
export interface RunTrack {
	run_id: string;
	track_id: string;
	sensor_id: string;
	track_state: string;
	start_unix_nanos: number;
	end_unix_nanos: number;
	observation_count: number;
	avg_speed_mps: number;
	peak_speed_mps: number;
	p50_speed_mps: number;
	p85_speed_mps: number;
	p95_speed_mps: number;
	bounding_box_length_avg: number;
	bounding_box_width_avg: number;
	bounding_box_height_avg: number;
	object_class?: string;
	object_confidence?: number;
	user_label?: string;
	quality_label?: string;
	label_confidence?: number;
	labeler_id?: string;
	labeled_at?: number;
	is_split_candidate?: boolean;
	is_merge_candidate?: boolean;
	linked_track_ids?: string[];
}

/** Labelling progress statistics */
export interface LabellingProgress {
	total: number;
	labelled: number;
	progress_pct: number;
	by_class: Record<string, number>;
}

/** Missed region: area where an object should have been tracked but wasn't */
export interface MissedRegion {
	region_id: string;
	run_id: string;
	center_x: number;
	center_y: number;
	radius_m: number;
	time_start_ns: number;
	time_end_ns: number;
	expected_label: string;
	labeler_id?: string;
	labeled_at?: number;
	notes?: string;
}

/** Lightweight sweep record for list views (no large JSON blobs). */
export interface SweepSummary {
	id: number;
	sweep_id: string;
	sensor_id: string;
	mode: string;
	status: string;
	error?: string;
	started_at: string;
	completed_at?: string;
}

/** Full sweep record including results, recommendation, charts, and round results. */
export interface SweepRecord {
	id: number;
	sweep_id: string;
	sensor_id: string;
	mode: string;
	status: string;
	request?: Record<string, unknown>;
	results?: Record<string, unknown>[];
	charts?: Record<string, unknown>[];
	recommendation?: Record<string, unknown>;
	round_results?: Record<string, unknown>[];
	error?: string;
	started_at: string;
	completed_at?: string;
	score_components?: ScoreComponents | string;
}

/** Score components for sweep scoring explanation */
export interface ScoreComponents {
	detection_rate: number;
	fragmentation: number;
	false_positives: number;
	velocity_coverage: number;
	quality_premium: number;
	truncation_rate: number;
	velocity_noise_rate: number;
	stopped_recovery: number;
	composite_score: number;
	weights_used: Record<string, number>;
	top_contributors?: string[];
	label_coverage_confidence?: number;
}
