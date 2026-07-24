import { apiFetch } from './client';
import { encodeId } from '$lib/utils/encode';

export interface AdminUser {
	id: number;
	email: string;
	name: string;
	role: string;
	weddingId: number | null;
	createdAt: string;
	updatedAt: string;
}

export interface AdminCreateData {
	email: string;
	password: string;
	name: string;
	role: string;
	weddingId?: number | null;
}

export async function listAdmins(): Promise<AdminUser[]> {
	const res = await apiFetch('/api/admins');
	if (!res.ok) throw new Error('Failed to list admins');
	return res.json();
}

export async function createAdmin(data: AdminCreateData): Promise<AdminUser> {
	const res = await apiFetch('/api/admins', {
		method: 'POST',
		body: JSON.stringify(data),
	});
	if (!res.ok) {
		const err = await res.json().catch(() => ({ title: 'Failed to create admin' }));
		throw new Error(err.title || 'Failed to create admin');
	}
	return res.json();
}

export async function deleteAdmin(id: number): Promise<void> {
	const res = await apiFetch(`/api/admins/${encodeId(id)}`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error('Failed to delete admin');
}

export async function assignWedding(adminId: number, weddingId: number | null): Promise<AdminUser> {
	const res = await apiFetch(`/api/admins/${encodeId(adminId)}/wedding`, {
		method: 'PUT',
		body: JSON.stringify({ weddingId }),
	});
	if (!res.ok) throw new Error('Failed to assign wedding');
	return res.json();
}
