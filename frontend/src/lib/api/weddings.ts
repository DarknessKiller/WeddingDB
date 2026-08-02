import { apiFetch } from './client';

export interface Wedding {
	id: string;
	name: string;
	date: string;
	showSeatNumbers: boolean;
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

export async function getWedding(id: string): Promise<Wedding> {
	const res = await apiFetch(`/api/weddings/${id}`);
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

export async function updateWedding(id: string, data: WeddingCreateData): Promise<Wedding> {
	const res = await apiFetch(`/api/weddings/${id}`, {
		method: 'PUT',
		body: JSON.stringify(data),
	});
	if (!res.ok) throw new Error('Failed to update wedding');
	return res.json();
}

export async function deleteWedding(id: string): Promise<void> {
	const res = await apiFetch(`/api/weddings/${id}`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error('Failed to delete wedding');
}

export interface KioskSettings {
	venueName: string;
	venueAddress: string;
	kioskDescription: string;
	kioskLogoUrl: string;
	kioskBackgroundUrl: string;
	kioskBackgroundBlur: number;
	kioskBackgroundSize: string;
	kioskBackgroundPosX: string;
	kioskBackgroundPosY: string;
	kioskLogoSize: string;
	kioskLogoPosX: string;
	kioskLogoPosY: string;
	showSeatNumbers: boolean;
}

export async function updateKioskSettings(id: string, settings: KioskSettings): Promise<Wedding> {
	const res = await apiFetch(`/api/weddings/${id}/kiosk`, {
		method: 'PUT',
		body: JSON.stringify(settings),
	});
	if (!res.ok) throw new Error('Failed to update kiosk settings');
	return res.json();
}
