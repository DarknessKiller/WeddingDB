import { apiFetch } from './client';
import { weddingId } from '#lib/stores/weddingId.ts';
import { get } from 'svelte/store';
import type { BanquetTable } from '#lib/types/index.ts';

export async function listTables(): Promise<BanquetTable[]> {
	const wid = get(weddingId);
	const res = await apiFetch(`/api/weddings/${wid}/tables`);
	if (!res.ok) throw new Error('Failed to list tables');
	return res.json();
}
