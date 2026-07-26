import { apiFetch } from './client';
import type { HallLayoutData, HallElement } from '$lib/types';

export async function getLayout(wid: string): Promise<HallLayoutData> {
	const res = await apiFetch(`/api/weddings/${wid}/layout`);
	if (!res.ok) throw new Error('Failed to load layout');
	return res.json();
}

export async function getPublicLayout(wid: string): Promise<HallLayoutData> {
	const res = await fetch(`/api/public/weddings/${wid}/layout`);
	if (!res.ok) throw new Error('Failed to load layout');
	return res.json();
}

export interface SaveLayoutPayload {
	hallWidth: number;
	hallHeight: number;
	tables: { id: string; x: number; y: number; degree: number }[];
	elements: Omit<HallElement, 'weddingId'>[];
}

export async function saveLayout(wid: string, data: SaveLayoutPayload): Promise<void> {
	// Strip extra fields AND temp IDs (new-* → "") that Fuego rejects
	const clean = {
		...data,
		elements: data.elements.map(({ id, type, x, y, degree, width, height, label, color, textColor, strokeColor, opacity, zIndex }) => ({
			id: id.startsWith('new-') ? '' : id,
			type, x, y, degree, width, height, label,
			color: color || '', textColor: textColor || '', strokeColor: strokeColor || '',
			opacity: opacity > 0 ? opacity : 1, zIndex
		}))
	};
	const res = await apiFetch(`/api/weddings/${wid}/layout`, {
		method: 'PATCH',
		body: JSON.stringify(clean)
	});
	if (!res.ok) throw new Error('Failed to save layout');
}
