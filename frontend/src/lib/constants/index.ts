import type { BanquetTable, HallLayout } from '$lib/types';

export const HALL_LAYOUT: HallLayout = {
	width: 860,
	height: 1000,
	stageTop: 0,
	stageHeight: 60,
	entranceBottom: 1000,
	aisleWidth: 40
};

// Default table definitions — static fallback, API fetches from backend
export const DEFAULT_TABLES: BanquetTable[] = [
	{ id: '1', name: 'Table 1', capacity: 10, x: 20, y: 18, row: 1, col: 1, isVip: true },
	{ id: '2', name: 'Table 2', capacity: 10, x: 50, y: 18, row: 1, col: 2, isVip: true },
	{ id: '3', name: 'Table 3', capacity: 10, x: 80, y: 18, row: 1, col: 3, isVip: true },
	{ id: '4', name: 'Table 4', capacity: 10, x: 15, y: 34, row: 2, col: 1, isVip: false },
	{ id: '5', name: 'Table 5', capacity: 10, x: 38, y: 34, row: 2, col: 2, isVip: false },
	{ id: '6', name: 'Table 6', capacity: 10, x: 62, y: 34, row: 2, col: 3, isVip: false },
	{ id: '7', name: 'Table 7', capacity: 10, x: 85, y: 34, row: 2, col: 4, isVip: false },
	{ id: '8', name: 'Table 8', capacity: 10, x: 15, y: 52, row: 3, col: 1, isVip: false },
	{ id: '9', name: 'Table 9', capacity: 10, x: 38, y: 52, row: 3, col: 2, isVip: false },
	{ id: '10', name: 'Table 10', capacity: 10, x: 62, y: 52, row: 3, col: 3, isVip: false },
	{ id: '11', name: 'Table 11', capacity: 10, x: 85, y: 52, row: 3, col: 4, isVip: false },
	{ id: '12', name: 'Table 12', capacity: 10, x: 15, y: 70, row: 4, col: 1, isVip: false },
	{ id: '13', name: 'Table 13', capacity: 8, x: 38, y: 70, row: 4, col: 2, isVip: false },
	{ id: '14', name: 'Table 14', capacity: 8, x: 62, y: 70, row: 4, col: 3, isVip: false },
	{ id: '15', name: 'Table 15', capacity: 8, x: 85, y: 70, row: 4, col: 4, isVip: false },
	{ id: '16', name: 'Table 16', capacity: 8, x: 20, y: 86, row: 5, col: 1, isVip: false },
	{ id: '17', name: 'Table 17', capacity: 8, x: 50, y: 86, row: 5, col: 2, isVip: false },
	{ id: '18', name: 'Table 18', capacity: 8, x: 80, y: 86, row: 5, col: 3, isVip: false },
	{ id: '19', name: 'Table 19', capacity: 8, x: 35, y: 92, row: 6, col: 1, isVip: false },
	{ id: '20', name: 'Table 20', capacity: 8, x: 65, y: 92, row: 6, col: 2, isVip: false }
];

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
