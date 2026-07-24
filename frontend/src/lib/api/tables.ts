import { apiFetch } from './client';
import type { BanquetTable } from '$lib/types';

function wid(weddingId: string, path: string) {
	return `/api/weddings/${weddingId}${path}`;
}

export async function listTables(weddingId: string): Promise<BanquetTable[]> {
	const res = await apiFetch(wid(weddingId, '/tables'));
	if (!res.ok) throw new Error('Failed to load tables');
	return res.json();
}

export async function createTable(weddingId: string, data: Omit<BanquetTable, 'id' | 'x' | 'y'>): Promise<BanquetTable> {
	const res = await apiFetch(wid(weddingId, '/tables'), {
		method: 'POST',
		body: JSON.stringify(data)
	});
	if (!res.ok) throw new Error('Failed to create table');
	return res.json();
}

export async function updateTable(weddingId: string, id: string, data: Omit<BanquetTable, 'id' | 'x' | 'y'>): Promise<BanquetTable> {
	const res = await apiFetch(wid(weddingId, `/tables/${id}`), {
		method: 'PUT',
		body: JSON.stringify(data)
	});
	if (!res.ok) throw new Error('Failed to update table');
	return res.json();
}

export async function deleteTable(weddingId: string, id: string): Promise<void> {
	const res = await apiFetch(wid(weddingId, `/tables/${id}`), {
		method: 'DELETE'
	});
	if (!res.ok) throw new Error('Failed to delete table');
}

export interface RawOccupancy {
	TableID: string;
	Pax: number;
}

export async function getOccupancy(weddingId: string): Promise<RawOccupancy[]> {
	const res = await apiFetch(wid(weddingId, '/occupancy'));
	if (!res.ok) throw new Error('Failed to load occupancy');
	return res.json();
}
