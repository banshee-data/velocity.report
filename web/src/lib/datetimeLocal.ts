export function fromDatetimeLocalToUnixSeconds(value: string): number | null {
	if (!value) {
		return null;
	}

	// Parse YYYY-MM-DDTHH:mm manually to avoid browser-dependent
	// interpretation of timezone-less ISO strings.
	const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/);
	if (!match) {
		return null;
	}

	const [, year, month, day, hours, minutes, seconds] = match;
	const d = new Date(
		Number(year),
		Number(month) - 1,
		Number(day),
		Number(hours),
		Number(minutes),
		Number(seconds ?? 0)
	);

	if (Number.isNaN(d.getTime())) {
		return null;
	}

	return Math.floor(d.getTime() / 1000);
}

export function toDatetimeLocalValue(unixSeconds: number): string {
	const d = new Date(unixSeconds * 1000);
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
