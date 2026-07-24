import { apiFetch } from './client';

export interface AdminUser {
	id: string;
	email: string;
	name: string;
	role: string;
	createdAt: string;
	updatedAt: string;
}

export interface AdminCreateData {
	email: string;
	password: string;
	name: string;
	role: string;
	weddings?: string[];
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

export async function deleteAdmin(id: string): Promise<void> {
	const res = await apiFetch(`/api/admins/${id}`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error('Failed to delete admin');
}

export async function assignWeddings(adminId: string, weddingIds: string[]): Promise<AdminUser> {
	const res = await apiFetch(`/api/admins/${adminId}/weddings`, {
		method: 'PUT',
		body: JSON.stringify({ weddings: weddingIds }),
	});
	if (!res.ok) throw new Error('Failed to assign weddings');
	return res.json();
}

export async function getUserWeddings(userId: string): Promise<{ id: string; name: string; date: string }[]> {
	const res = await apiFetch(`/api/admins/${userId}/weddings`);
	if (!res.ok) throw new Error('Failed to get user weddings');
	return res.json();
}
