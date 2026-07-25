/**
 * Encode a UUID to a URL-safe base64 string.
 * Strips hyphens, encodes raw bytes, replaces +/- with _/-, removes padding.
 */
export function encodeId(id: string): string {
  // Remove hyphens and convert to base64
  const hex = id.replace(/-/g, '');
  const bytes = new Uint8Array(hex.match(/.{1,2}/g)!.map(b => parseInt(b, 16)));
  const base64 = btoa(String.fromCharCode(...bytes));
  // Make URL-safe: replace +/, remove padding
  return base64.replace(/\+/g, '_').replace(/\//g, '-').replace(/=+$/, '');
}

/**
 * Decode a URL-safe base64 string back to a UUID.
 */
export function decodeId(encoded: string): string {
  // Restore standard base64: replace _- with +/, add padding
  let base64 = encoded.replace(/_/g, '+').replace(/-/g, '/');
  while (base64.length % 4) base64 += '=';
  const binary = atob(base64);
  const hex = Array.from(binary, c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('');
  // Insert hyphens to form UUID
  return `${hex.slice(0,8)}-${hex.slice(8,12)}-${hex.slice(12,16)}-${hex.slice(16,20)}-${hex.slice(20)}`;
}
