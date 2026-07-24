/**
 * Encode a numeric ID to base64url (raw, no padding) — matches Go's base64.RawURLEncoding.
 */
export function encodeId(id: number | string): string {
	return btoa(String(id)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
