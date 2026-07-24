import { writable } from 'svelte/store';

// ponytail: hardcoded wedding ID for now; reads from localStorage or defaults to base64("1") = "MQ"
function getInitialWeddingId(): string {
	if (typeof window !== 'undefined') {
		return localStorage.getItem('weddingdb_wedding_id') ?? 'MQ';
	}
	return 'MQ';
}

export const weddingId = writable<string>(getInitialWeddingId());

export function setWeddingId(id: string) {
	weddingId.set(id);
	if (typeof window !== 'undefined') {
		localStorage.setItem('weddingdb_wedding_id', id);
	}
}
