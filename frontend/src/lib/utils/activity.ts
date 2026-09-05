import type { Guest } from '$lib/types';

export function getActivityTimestamp(guest: Pick<Guest, 'createdAt' | 'checkedInAt' | 'updatedAt'>): Date {
	return guest.checkedInAt ?? guest.updatedAt ?? guest.createdAt;
}
