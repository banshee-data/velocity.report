import type { HeadwayDistribution, HeadwayMeasurement, SceneHeadway } from './api';
import {
	HEADWAY_METRICS,
	formatMeasurement,
	formatNanos,
	formatShare,
	headwayAvailabilityMessage,
	headwayBandRows,
	headwayEncounterRows,
	headwayExcludedRows,
	headwayStatusLabel,
	headwayStatusNote,
	headwaySummaryRows,
	versionKey,
	versionLabel
} from './headway';

const version = {
	estimate_stage: 'final' as const,
	estimator_id: 'cv_kf_v1',
	obs_model_id: 'medoid_v0',
	method_id: 'following_encounter_v1/abc',
	param_hash: 'sha256:x'
};

function interval(value: number, lower: number, upper: number, unit = 's'): HeadwayMeasurement {
	return {
		name: 'm',
		unit,
		value,
		suppressed: false,
		uncertainty: {
			kind: 'interval',
			lower,
			upper,
			coverage: 0.9,
			method: 'monte_carlo',
			samples: 2000
		}
	};
}

function suppressed(reason: string): HeadwayMeasurement {
	return { name: 'm', unit: 's', suppressed: true, reason };
}

function distribution(): HeadwayDistribution {
	return {
		schema: 'following_distribution_v1',
		version,
		events: 3,
		exposure_events: 2,
		accounting: {
			instants: 90,
			valid_nanos: 6_000_000_000,
			band_nanos: {},
			predicted_only_nanos: 500_000_000,
			record_gap_nanos: 200_000_000
		},
		accounted_nanos: 8_000_000_000,
		histograms: {},
		excluded: {
			not_observed: { instants: 5, nanos: 500_000_000, predicted_only_nanos: 500_000_000 },
			insufficient_observation: { instants: 9, nanos: 900_000_000, predicted_only_nanos: 0 },
			below_speed_floor: { instants: 6, nanos: 600_000_000, predicted_only_nanos: 0 }
		},
		bands: [
			{
				name: 'interaction.following_time_below_2000ms_s',
				threshold: 2,
				events: 2,
				instants: 40,
				nanos: 4_000_000_000,
				rate: {
					name: 'interaction.following_rate_below_2000ms_ratio',
					unit: 'ratio',
					value: 0.8,
					suppressed: false,
					opportunity_seconds: 5.1
				}
			},
			{
				name: 'interaction.following_time_below_1000ms_s',
				threshold: 1,
				events: 0,
				instants: 0,
				nanos: 0,
				rate: {
					name: 'interaction.following_rate_below_1000ms_ratio',
					unit: 'ratio',
					suppressed: true,
					reason: 'insufficient_observation'
				}
			}
		]
	};
}

function headway(overrides: Partial<SceneHeadway> = {}): SceneHeadway {
	return {
		scene_id: 'soma1',
		status: 'provisional',
		availability: 'available',
		start_unix_nanos: 1_000_000_000_000,
		end_unix_nanos: 1_010_000_000_000,
		source_id: 'source/v1/a',
		sources: [{ source_id: 'source/v1/a', events: 3, first_unix_nanos: 1, last_unix_nanos: 2 }],
		version,
		versions: [{ version, events: 3, status: 'provisional' }],
		distribution: distribution(),
		encounters: [],
		...overrides
	};
}

describe('headway presentation', () => {
	it('labels every status as provisional', () => {
		expect(headwayStatusLabel('provisional')).toBe('PROVISIONAL');
		expect(headwayStatusLabel('synthetic_oracle')).toBe('PROVISIONAL · SYNTHETIC ORACLE');
		expect(headwayStatusNote('synthetic_oracle')).toContain('not sensor data');
		expect(headwayStatusNote('provisional')).toContain('promotion gates');
	});

	it('says why there is no distribution, for every availability', () => {
		expect(headwayAvailabilityMessage(headway())).toBe('');
		expect(headwayAvailabilityMessage(headway({ availability: 'no_capture_window' }))).toContain(
			'no capture window'
		);
		expect(headwayAvailabilityMessage(headway({ availability: 'no_encounters' }))).toContain(
			'No following encounters'
		);
		const twin = headway({
			availability: 'source_ambiguous',
			sources: [
				{ source_id: 'a', events: 1, first_unix_nanos: 1, last_unix_nanos: 2 },
				{ source_id: 'b', events: 1, first_unix_nanos: 1, last_unix_nanos: 2 }
			]
		});
		expect(headwayAvailabilityMessage(twin)).toBe(
			'2 analyses cover this capture window. Choose one to see its distribution.'
		);
		expect(
			headwayAvailabilityMessage(headway({ availability: 'no_matching_version' }), 'final')
		).toContain('Choose one of the versions');
		expect(
			headwayAvailabilityMessage(headway({ availability: 'no_matching_version', versions: [] }))
		).toBe('No final analysis exists for this scene.');
	});

	it('formats time, shares and measurements without inventing zeros', () => {
		expect(formatNanos(4_900_000_000)).toBe('4.9 s');
		expect(formatShare(1, 8)).toBe('13%');
		expect(formatShare(0.5, 9.1)).toBe('5.5%');
		expect(formatShare(0, 8)).toBe('0%');
		expect(formatShare(1, 0)).toBe('–');
		expect(formatMeasurement(interval(0.775, 0.7, 0.85))).toBe('0.78 s [0.70, 0.85] 90%');
		expect(formatMeasurement(suppressed('estimate_not_final'))).toBe(
			'suppressed: estimate_not_final'
		);
		expect(formatMeasurement(undefined)).toBe('–');
		// Valid time declares no uncertainty: the value alone, never an invented interval.
		expect(
			formatMeasurement(
				{ name: 'm', unit: 's', value: 4.9, suppressed: false, uncertainty: { kind: 'none' } },
				1
			)
		).toBe('4.9 s');
	});

	it('summarises where the accounted time went', () => {
		const rows = headwaySummaryRows(distribution());
		const byLabel = Object.fromEntries(rows.map((r) => [r.label, r]));
		expect(byLabel['Encounters'].value).toBe('3 (2 in the distribution)');
		expect(byLabel['Accounted time'].value).toBe('8.0 s');
		expect(byLabel['In the distribution'].value).toBe('6.0 s');
		expect(byLabel['Beside it, by reason']).toEqual({
			label: 'Beside it, by reason',
			value: '2.0 s',
			share: '25%'
		});
		expect(byLabel['Predicted-only'].share).toBe('6.3%');
		expect(byLabel['Record gaps, not counted'].value).toBe('0.2 s');
	});

	it('lists band exposure by registry id, keeping a suppressed rate suppressed', () => {
		const rows = headwayBandRows(distribution());
		expect(rows[0]).toEqual({
			name: 'interaction.following_time_below_2000ms_s',
			threshold: '2.0 s',
			below: '4.0 s',
			rate: '80%',
			rateName: 'interaction.following_rate_below_2000ms_ratio',
			events: 2
		});
		expect(rows[1].rate).toBe('suppressed: insufficient_observation');
	});

	it('lists excluded time by reason token, most first', () => {
		const rows = headwayExcludedRows(distribution());
		expect(rows.map((r) => r.reason)).toEqual([
			'insufficient_observation',
			'below_speed_floor',
			'not_observed'
		]);
		expect(rows[2]).toEqual({
			reason: 'not_observed',
			instants: 5,
			time: '0.5 s',
			predictedOnly: '0.5 s',
			share: '6.3%'
		});
	});

	it('reads encounter values from the block the stage allows', () => {
		const encounter = {
			event_id: 'interaction/v1/e1',
			primary_track_id: 'trk_follower',
			secondary_track_id: 'trk_leader',
			start_unix_nanos: 1_002_500_000_000,
			end_unix_nanos: 1_007_400_000_000,
			geometry_id: 'path/v1/g',
			worst_support: 'observed',
			accounting: {
				instants: 50,
				valid_nanos: 4_900_000_000,
				band_nanos: {},
				predicted_only_nanos: 0,
				record_gap_nanos: 0
			},
			measurements: {
				[HEADWAY_METRICS.netTimeGapMin]: suppressed('estimate_not_final'),
				[HEADWAY_METRICS.netTimeGapP50]: suppressed('estimate_not_final')
			},
			provisional: {
				[HEADWAY_METRICS.netTimeGapMin]: interval(0.775, 0.7, 0.85),
				[HEADWAY_METRICS.netTimeGapP50]: interval(1.3875, 1.3, 1.45),
				[HEADWAY_METRICS.spatialGapMin]: interval(7.75, 7.3, 8.2, 'm')
			}
		};
		const fixedLag = { ...version, estimate_stage: 'fixed_lag' as const };
		const [row] = headwayEncounterRows(headway({ version: fixedLag, encounters: [encounter] }));
		expect(row).toEqual({
			eventId: 'interaction/v1/e1',
			follower: 'trk_follower',
			leader: 'trk_leader',
			start: '+2.5 s',
			validTime: '4.9 s',
			minNetTimeGap: '0.78 s [0.70, 0.85] 90%',
			medianNetTimeGap: '1.39 s [1.30, 1.45] 90%',
			minSpatialGap: '7.8 m [7.3, 8.2] 90%',
			block: 'provisional'
		});
		const [finalRow] = headwayEncounterRows(headway({ encounters: [encounter] }));
		expect(finalRow.block).toBe('measurements');
		expect(finalRow.minNetTimeGap).toBe('suppressed: estimate_not_final');
	});

	it('keys and labels versions', () => {
		expect(versionKey(version)).toBe(
			'final|cv_kf_v1|medoid_v0|following_encounter_v1/abc|sha256:x'
		);
		expect(versionLabel({ version, events: 3, status: 'provisional' })).toBe(
			'final · cv_kf_v1 · following_encounter_v1/abc · 3 encounters'
		);
	});
});
