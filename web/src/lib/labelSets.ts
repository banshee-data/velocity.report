/**
 * Read-only descriptions of annotation packs and the label sets that hold
 * their labels, for the Segments page's pack inventory.
 *
 * A label set cites a capture's returns by packet, so it applies to every run
 * of the capture and every file holding its packets. Its verified captures are
 * the files `velocity lidar labels bind` has checked, by SHA-256.
 */

/** One capture file a label set has verified holds its packets. */
export type LabelSetCapture = {
	name: string;
	sha256: string;
	packets_found: number;
	packets_missing: number;
	windows_complete: number;
	windows_partial: number;
	windows_missing: number;
};

/** One entry of `GET /api/lidar/annotations/packs`. */
export type AnnotationPack = {
	pack_dir: string;
	segment_id?: string;
	status: string;
	reviewed_objects: number;
	reviewed_masks: number;
	proposal_layers: number;
	label_set_id?: string;
	label_set_name?: string;
	label_set_revision?: number;
	label_set_packets?: number;
	label_set_windows?: number;
	label_set_captures?: LabelSetCapture[];
	error?: string;
};

/** The last path component, which is how an operator names a pack. */
export function baseName(path: string): string {
	const parts = path.split('/').filter(Boolean);
	return parts[parts.length - 1] ?? path;
}

/** The first eight hex digits of a `sha256:` digest. */
export function shortSHA(digest: string): string {
	return digest.replace(/^sha256:/, '').slice(0, 8);
}

/** Whether the pack is linked to a label set, which holds its labels. */
export function hasLabelSet(pack: AnnotationPack): boolean {
	return Boolean(pack.label_set_id);
}

/** Where the pack's labels are, in one line. */
export function labelsText(pack: AnnotationPack): string {
	if (hasLabelSet(pack)) {
		const name = pack.label_set_name || pack.label_set_id;
		return `Label set ${name}, revision ${pack.label_set_revision ?? 0}`;
	}
	return 'No labels yet';
}

/** One verified capture: name, short digest, packets and frames covered. */
export function captureText(capture: LabelSetCapture, pack: AnnotationPack): string {
	const packets = pack.label_set_packets ?? capture.packets_found + capture.packets_missing;
	const windows =
		pack.label_set_windows ??
		capture.windows_complete + capture.windows_partial + capture.windows_missing;
	return (
		`${capture.name} (${shortSHA(capture.sha256)}): ` +
		`${capture.packets_found.toLocaleString('en-GB')} of ${packets.toLocaleString('en-GB')} packets, ` +
		`${capture.windows_complete.toLocaleString('en-GB')} of ${windows.toLocaleString('en-GB')} frames`
	);
}

/** The verified-captures lines, or why there are none. */
export function capturesText(pack: AnnotationPack): string[] {
	if (!hasLabelSet(pack)) return [];
	const captures = pack.label_set_captures ?? [];
	if (captures.length === 0) {
		return ['Not verified against any capture file yet (velocity lidar labels bind)'];
	}
	return captures.map((capture) => captureText(capture, pack));
}

/** Reviewed work in one line. */
export function reviewText(pack: AnnotationPack): string {
	return (
		`${pack.reviewed_objects} reviewed objects · ${pack.reviewed_masks} reviewed masks · ` +
		`${pack.proposal_layers} proposal layers`
	);
}

/**
 * Packs in the order an operator looks for them: those with labels first,
 * then the rest, each by name.
 */
export function inventoryOrder(packs: AnnotationPack[]): AnnotationPack[] {
	const rank = (pack: AnnotationPack) => (hasLabelSet(pack) ? 0 : 1);
	return [...packs].sort(
		(a, b) => rank(a) - rank(b) || baseName(a.pack_dir).localeCompare(baseName(b.pack_dir))
	);
}
