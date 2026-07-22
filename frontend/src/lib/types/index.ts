export type RSVPStatus = 'confirmed' | 'pending' | 'declined' | 'no_response';

export interface Guest {
  id: string;
  name: string;
  phone: string;
  email?: string;
  rsvp: RSVPStatus;
  pax: number;
  tableId: number | null;
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
  id: number;
  name: string;
  capacity: number;
  x: number; // percentage position in hall
  y: number; // percentage position in hall
  isVip: boolean;
  zone: 'front' | 'middle' | 'back' | 'center';
}

export interface Seat {
  tableId: number;
  seatNumber: number;
  guest: Guest | null;
  isSelected: boolean;
  isHighlighted: boolean;
}

export interface TableOccupancy {
  tableId: number;
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

export interface HallLayout {
  width: number;
  height: number;
  stageTop: number;
  stageHeight: number;
  entranceBottom: number;
  aisleWidth: number;
}

export type ViewMode = 'seating' | 'reception' | 'kiosk';
