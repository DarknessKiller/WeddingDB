export type RSVPStatus = 'confirmed' | 'pending' | 'declined' | 'no_response';

export interface Guest {
  id: string;
  name: string;
  pinyin?: string;
  email?: string;
  rsvp: RSVPStatus;
  pax: number;
  tableId: string | null;
  seatNumber: number | null;
  checkedIn: boolean;
  checkedInAt?: Date;
  notes: string;
  dietaryRequirements: string[];
  isVip: boolean;
  angbaoAmount?: number;
  giftItem?: string;
  createdAt: Date;
  updatedAt?: Date;
}

export interface BanquetTable {
  id: string;
  name: string;
  capacity: number;
  x: number;
  y: number;
  degree: number;
  isVip: boolean;
}

export interface TableOccupancy {
  tableId: string;
  tableName: string;
  occupied: number;
  capacity: number;
  percentage: number;
  // Highest occupied seat end; the capacity floor the API enforces.
  highestSeat?: number;
  isVip?: boolean;
}

export interface ActivityItem {
  id: string;
  type: 'check_in' | 'rsvp_update' | 'table_assignment' | 'reservation';
  message: string;
  guestName?: string;
  timestamp: Date;
  icon: string;
}

export type ElementType = 'stage' | 'dj_counter' | 'entrance' | 'tv' | 'walkway' | 'box';

export interface HallElement {
	id: string;
	type: ElementType;
	x: number;
	y: number;
	degree: number;
	width: number;
	height: number;
	name: string;
	color: string;
	textColor: string;
	strokeColor: string;
	opacity: number;
	zIndex: number;
}

export interface HallLayoutData {
  hallWidth: number;
  hallHeight: number;
  tables: BanquetTable[];
  elements: HallElement[];
}
