import type { HallLayout } from '$lib/types';

export const HALL_LAYOUT: HallLayout = {
	width: 860,
	height: 1000,
	stageTop: 0,
	stageHeight: 60,
	entranceBottom: 1000,
	aisleWidth: 40
};

export const DIETARY_OPTIONS = [
	'Vegetarian',
	'Vegan',
	'Halal',
	'Gluten-free',
	'Nut allergy',
	'Seafood allergy',
	'Dairy-free',
	'No spicy food'
] as const;

export const RSVP_STATUS_LABELS: Record<string, string> = {
	confirmed: 'Confirmed',
	pending: 'Pending',
	declined: 'Declined',
	no_response: 'No Response'
};

export const RSVP_STATUS_COLORS: Record<string, string> = {
	confirmed: 'bg-emerald-50 text-emerald-700 border-emerald-200',
	pending: 'bg-amber-50 text-amber-700 border-amber-200',
	declined: 'bg-red-50 text-red-700 border-red-200',
	no_response: 'bg-gray-50 text-gray-500 border-gray-200'
};
