import { apiFetch } from './client';
import { encodeId } from '$lib/utils/encode';

export interface Wedding {
	id: number;
	name: string;
	date: string;
	createdAt: string;
	updatedAt: string;
}

export interface WeddingCreateData {
	name: string;
	date: string;
}

export async function listWeddings(): Promise<Wedding[]> {
	const res = await apiFetch('/api/weddings');
	if (!res.ok) throw new Error('Failed to list weddings');
	return res.json();
}

export async function getWedding(id: number): Promise<Wedding> {
	const res = await apiFetch(`/api/weddings/${encodeId(id)}`);
	if (!res.ok) throw new Error('Failed to get wedding');
	return res.json();
}

export async function createWedding(data: WeddingCreateData): Promise<Wedding> {
	const res = await apiFetch('/api/weddings', {
		method: 'POST',
		body: JSON.stringify(data),
	});
	if (!res.ok) {
		const err = await res.json().catch(() => ({ title: 'Failed to create wedding' }));
		throw new Error(err.title || 'Failed to create wedding');
	}
	return res.json();
}

export async function updateWedding(id: number, data: WeddingCreateData): Promise<Wedding> {
	const res = await apiFetch(`/api/weddings/${encodeId(id)}`, {
		method: 'PUT',
		body: JSON.stringify(data),
	});
	if (!res.ok) throw new Error('Failed to update wedding');
	return res.json();
}

export async function deleteWedding(id: number): Promise<void> {
	const res = await apiFetch(`/api/weddings/${encodeId(id)}`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error('Failed to delete wedding');
}
