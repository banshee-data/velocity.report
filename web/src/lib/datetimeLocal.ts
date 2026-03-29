export function fromDatetimeLocalToUnixSeconds(value: string): number | null {
	if (!value) {
		return null;
	}

	const parsed = new Date(value).getTime();
	if (Number.isNaN(parsed)) {
		return null;
	}

	return Math.floor(parsed / 1000);
}

export function toDatetimeLocalValue(unixSeconds: number): string {
	const d = new Date(unixSeconds * 1000);
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
