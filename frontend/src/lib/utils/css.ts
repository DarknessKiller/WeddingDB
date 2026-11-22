/** True for an empty value, a same-origin absolute path (/uploads/x.png), and
 * absolute http(s) URLs. Protocol-relative //host is rejected. */
export function isSafeImageUrl(value: string): boolean {
	const v = value.trim();
	if (!v) return true;
	// Uploads are stored as same-origin paths; //host would load another origin.
	if (v.startsWith('/')) return !v.startsWith('//');
	try {
		const { protocol } = new URL(v);
		return protocol === 'http:' || protocol === 'https:';
	} catch {
		return false;
	}
}

/** CSS url() for an accepted image URL, `none` otherwise. Quotes and
 * backslashes are escaped so a value cannot break out of the url(). */
export function cssImageUrl(value: string): string {
	if (!value.trim() || !isSafeImageUrl(value)) return 'none';
	const escaped = value.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
	return `url("${escaped}")`;
}
