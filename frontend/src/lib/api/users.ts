import { apiFetch } from './client';

export interface User {
	id: string;
	email: string;
	name: string;
	role: string;
	createdAt: string;
	updatedAt: string;
}

export interface UserCreateData {
	email: string;
	password: string;
	name: string;
	role: string;
	weddings?: string[];
}

export async function listUsers(): Promise<User[]> {
	const res = await apiFetch('/api/users');
	if (!res.ok) throw new Error('Failed to list users');
	return res.json();
}

export async function createUser(data: UserCreateData): Promise<User> {
	const res = await apiFetch('/api/users', {
		method: 'POST',
		body: JSON.stringify(data),
	});
	if (!res.ok) {
		const err = await res.json().catch(() => ({ title: 'Failed to create user' }));
		throw new Error(err.title || 'Failed to create user');
	}
	return res.json();
}

export async function deleteUser(id: string): Promise<void> {
	const res = await apiFetch(`/api/users/${id}`, {
		method: 'DELETE',
	});
	if (!res.ok) throw new Error('Failed to delete user');
}

export async function assignWeddings(userId: string, weddingIds: string[]): Promise<User> {
	const res = await apiFetch(`/api/users/${userId}/weddings`, {
		method: 'PUT',
		body: JSON.stringify({ weddings: weddingIds }),
	});
	if (!res.ok) throw new Error('Failed to assign weddings');
	return res.json();
}

export async function getUserWeddings(userId: string): Promise<{ id: string; name: string; date: string }[]> {
	const res = await apiFetch(`/api/users/${userId}/weddings`);
	if (!res.ok) throw new Error('Failed to get user weddings');
	return res.json();
}

export async function resetPassword(userId: string, password: string): Promise<void> {
	const res = await apiFetch(`/api/users/${userId}/reset-password`, {
		method: 'POST',
		body: JSON.stringify({ password }),
	});
	if (!res.ok) throw new Error('Failed to reset password');
}

export async function updateRole(userId: string, role: string): Promise<User> {
	const res = await apiFetch(`/api/users/${userId}/role`, {
		method: 'PUT',
		body: JSON.stringify({ role }),
	});
	if (!res.ok) throw new Error('Failed to update role');
	return res.json();
}
