export type RSVPStatus = 'confirmed' | 'pending' | 'declined' | 'no_response';

export interface Guest {
  id: string;
  name: string;
  phone: string;
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

export interface Seat {
  tableId: string;
  seatNumber: number;
  guest: Guest | null;
  isSelected: boolean;
  isHighlighted: boolean;
}

export interface TableOccupancy {
  tableId: string;
  tableName: string;
  occupied: number;
  capacity: number;
  percentage: number;
}

export interface DashboardStats {
  totalGuests: number;
  confirmedGuests: number;
  pendingRsvp: number;
  declined: number;
  checkedIn: number;
  totalPax: number;
  totalTables: number;
  occupiedTables: number;
  averageOccupancy: number;
}

export interface ActivityItem {
  id: string;
  type: 'check_in' | 'rsvp_update' | 'table_assignment' | 'reservation';
  message: string;
  guestName?: string;
  timestamp: Date;
  icon: string;
}

export interface ReservationFormData {
  guestName: string;
  phone: string;
  email?: string;
  numberOfGuests: number;
  attendance: 'attending' | 'not_attending';
  specialRequests: string;
  dietaryRequirements: string[];
}

export interface SearchResult {
  guest: Guest;
  matchScore: number;
  matchType: 'name' | 'phone' | 'email';
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
  label: string;
  zIndex: number;
}

export interface HallLayoutData {
  hallWidth: number;
  hallHeight: number;
  tables: BanquetTable[];
  elements: HallElement[];
}

export type ViewMode = 'seating' | 'reception' | 'kiosk';
