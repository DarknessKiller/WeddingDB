import type { BanquetTable, HallLayout } from '$lib/types';

export const HALL_LAYOUT: HallLayout = {
  width: 860,
  height: 1000,
  stageTop: 0,
  stageHeight: 60,
  entranceBottom: 1000,
  aisleWidth: 40
};

export const TABLE_DEFINITIONS: BanquetTable[] = [
  // VIP tables (near stage) — 3 tables, 30% apart
  { id: 1,  name: 'Table 1',  capacity: 10, x: 20, y: 18, isVip: true,  zone: 'front' },
  { id: 2,  name: 'Table 2',  capacity: 10, x: 50, y: 18, isVip: true,  zone: 'front' },
  { id: 3,  name: 'Table 3',  capacity: 10, x: 80, y: 18, isVip: true,  zone: 'front' },
  // Middle-upper row — 4 tables, ~22% apart
  { id: 4,  name: 'Table 4',  capacity: 10, x: 14, y: 34, isVip: false, zone: 'middle' },
  { id: 5,  name: 'Table 5',  capacity: 10, x: 38, y: 34, isVip: false, zone: 'middle' },
  { id: 6,  name: 'Table 6',  capacity: 10, x: 62, y: 34, isVip: false, zone: 'middle' },
  { id: 7,  name: 'Table 7',  capacity: 10, x: 86, y: 34, isVip: false, zone: 'middle' },
  // Middle row — 4 tables
  { id: 8,  name: 'Table 8',  capacity: 10, x: 14, y: 52, isVip: false, zone: 'middle' },
  { id: 9,  name: 'Table 9',  capacity: 10, x: 38, y: 52, isVip: false, zone: 'middle' },
  { id: 10, name: 'Table 10', capacity: 10, x: 62, y: 52, isVip: false, zone: 'middle' },
  { id: 11, name: 'Table 11', capacity: 10, x: 86, y: 52, isVip: false, zone: 'middle' },
  // Back-upper row — 4 tables
  { id: 12, name: 'Table 12', capacity: 10, x: 14, y: 70, isVip: false, zone: 'back' },
  { id: 13, name: 'Table 13', capacity: 8,  x: 38, y: 70, isVip: false, zone: 'back' },
  { id: 14, name: 'Table 14', capacity: 8,  x: 62, y: 70, isVip: false, zone: 'back' },
  { id: 15, name: 'Table 15', capacity: 8,  x: 86, y: 70, isVip: false, zone: 'back' },
  // Back row — 3 tables
  { id: 16, name: 'Table 16', capacity: 8,  x: 20, y: 86, isVip: false, zone: 'back' },
  { id: 17, name: 'Table 17', capacity: 8,  x: 50, y: 86, isVip: false, zone: 'back' },
  { id: 18, name: 'Table 18', capacity: 8,  x: 80, y: 86, isVip: false, zone: 'back' },
  // Extra back row — above entrance
  { id: 19, name: 'Table 19', capacity: 8,  x: 35, y: 92, isVip: false, zone: 'back' },
  { id: 20, name: 'Table 20', capacity: 8,  x: 65, y: 92, isVip: false, zone: 'back' },
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
