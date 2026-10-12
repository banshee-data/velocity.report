import {
	baseName,
	captureText,
	capturesText,
	inventoryOrder,
	labelsText,
	reviewText,
	shortSHA,
	type AnnotationPack
} from './labelSets';

function pack(extra: Partial<AnnotationPack> = {}): AnnotationPack {
	return {
		pack_dir: '/packs/ad8b9438-v2',
		status: 'reviewed',
		reviewed_objects: 30,
		reviewed_masks: 4102,
		proposal_layers: 23,
		...extra
	};
}

const kirk0 = {
	name: 'kirk0.pcapng',
	sha256: 'sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a',
	packets_found: 149741,
	packets_missing: 0,
	windows_complete: 832,
	windows_partial: 0,
	windows_missing: 0
};

describe('labelSets', () => {
	it('names a pack by its last path component', () => {
		expect(baseName('/a/b/pack-v2/')).toBe('pack-v2');
		expect(baseName('pack')).toBe('pack');
	});

	it('shortens a digest to eight hex digits', () => {
		expect(shortSHA(kirk0.sha256)).toBe('2864ebde');
	});

	it('says where a pack keeps its labels', () => {
		expect(
			labelsText(
				pack({ label_set_id: 'id', label_set_name: 'kirk0-reference', label_set_revision: 2232 })
			)
		).toBe('Label set kirk0-reference, revision 2232');
		expect(labelsText(pack({ migrated_to: '/packs/ad8b9438-v2' }))).toBe(
			'Version 1, read only: labels moved to ad8b9438-v2'
		);
		expect(labelsText(pack())).toBe('Version 1, read only');
	});

	it('describes a verified capture against the label set it belongs to', () => {
		const p = pack({ label_set_id: 'id', label_set_packets: 149741, label_set_windows: 832 });
		expect(captureText(kirk0, p)).toBe(
			'kirk0.pcapng (2864ebde): 149,741 of 149,741 packets, 832 of 832 frames'
		);
		const clip = {
			...kirk0,
			name: 'clip.pcapng',
			packets_found: 40000,
			packets_missing: 109741,
			windows_complete: 200,
			windows_missing: 632
		};
		expect(captureText(clip, p)).toBe(
			'clip.pcapng (2864ebde): 40,000 of 149,741 packets, 200 of 832 frames'
		);
	});

	it('lists captures only for label sets, and says when none are verified', () => {
		expect(capturesText(pack())).toEqual([]);
		expect(capturesText(pack({ label_set_id: 'id' }))).toEqual([
			'Not verified against any capture file yet (velocity lidar labels bind)'
		]);
		expect(
			capturesText(pack({ label_set_id: 'id', label_set_captures: [kirk0, kirk0] }))
		).toHaveLength(2);
	});

	it('summarises reviewed work', () => {
		expect(reviewText(pack())).toBe(
			'30 reviewed objects · 4102 reviewed masks · 23 proposal layers'
		);
	});

	it('orders label sets first and migrated originals last', () => {
		const ordered = inventoryOrder([
			pack({ pack_dir: '/p/old', migrated_to: '/p/old-v2' }),
			pack({ pack_dir: '/p/zeta-v2', label_set_id: 'z' }),
			pack({ pack_dir: '/p/plain' }),
			pack({ pack_dir: '/p/alpha-v2', label_set_id: 'a' })
		]).map((p) => baseName(p.pack_dir));
		expect(ordered).toEqual(['alpha-v2', 'zeta-v2', 'plain', 'old']);
	});
});
