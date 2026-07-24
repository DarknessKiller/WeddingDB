/**
 * UUIDs are URL-safe strings — no encoding needed.
 * Kept as passthrough for backward compat with existing imports.
 */
export function encodeId(id: number | string): string {
	return String(id);
}
