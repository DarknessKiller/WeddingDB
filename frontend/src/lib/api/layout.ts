import { apiFetch } from './client';
import type { HallLayoutData, HallElement } from '#lib/types/index.ts';

export async function getLayout(wid: string): Promise<HallLayoutData> {
	const res = await apiFetch(`/api/weddings/${wid}/layout`);
	if (!res.ok) throw new Error('Failed to load layout');
	return res.json();
}

export async function getPublicLayout(wid: string, opts: { signal?: AbortSignal } = {}): Promise<HallLayoutData> {
	const res = await fetch(`/api/public/weddings/${wid}/layout`, { signal: opts.signal });
	if (!res.ok) throw new Error('Failed to load layout');
	return res.json();
}

// Another device saved the layout after this one loaded it (HTTP 409).
export class LayoutConflictError extends Error {
	constructor() {
		super('Another device changed the hall layout. We reloaded it; make your changes again.');
		this.name = 'LayoutConflictError';
	}
}

export interface SaveLayoutPayload {
	version: number; // from the getLayout this edit started from
	hallWidth: number;
	hallHeight: number;
	tables: { id: string; x: number; y: number; degree: number; capacity: number; isVip: boolean }[];
	elements: Omit<HallElement, 'weddingId'>[];
}

export async function saveLayout(wid: string, data: SaveLayoutPayload): Promise<void> {
	// Strip extra fields AND temp IDs (new-* → "") that Fuego rejects
	const clean = {
		...data,
		elements: data.elements.map(({ id, type, x, y, degree, width, height, name, color, textColor, strokeColor, opacity, zIndex }) => ({
			id: id.startsWith('new-') ? '' : id,
			type, x, y, degree, width, height, name,
			color: color || '', textColor: textColor || '', strokeColor: strokeColor || '',
			opacity: opacity > 0 ? opacity : 1, zIndex
		}))
	};
	const res = await apiFetch(`/api/weddings/${wid}/layout`, {
		method: 'PATCH',
		body: JSON.stringify(clean)
	});
	if (res.status === 409) throw new LayoutConflictError();
	if (!res.ok) throw new Error('Failed to save layout');
}

// saveLayoutKeepMine retries a conflicted save against the version currently
// on the server, after the user explicitly chose to overwrite it.
export async function saveLayoutKeepMine(wid: string, data: SaveLayoutPayload): Promise<void> {
	const latest = await getLayout(wid);
	await saveLayout(wid, { ...data, version: latest.version ?? 0 });
}
