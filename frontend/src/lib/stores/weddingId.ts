import { writable } from 'svelte/store';

function getInitialWeddingId(): string {
	if (typeof window !== 'undefined') {
		return localStorage.getItem('weddingdb_wedding_id') ?? '';
	}
	return '';
}

export const weddingId = writable<string>(getInitialWeddingId());

export function setWeddingId(id: string) {
	weddingId.set(id);
	if (typeof window !== 'undefined') {
		if (id) {
			localStorage.setItem('weddingdb_wedding_id', id);
		} else {
			localStorage.removeItem('weddingdb_wedding_id');
		}
	}
}
