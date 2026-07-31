import { apiFetch } from './client';
import type { DashboardStats, TableOccupancy, ActivityItem } from '$lib/types';

interface RawGuest {
  id: string;
  name: string;
  phone: string;
  email?: string;
  pax: number;
  rsvp: string;
  checkedInAt?: string | null;
  notes?: string;
  dietary?: string[];
  isVip?: boolean;
  angbaoAmt?: number | null;
  giftItem?: string | null;
  createdAt: string;
  updatedAt: string;
}

interface RawOccupancy {
  TableID: string;
  Pax: number;
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await apiFetch(path);
  if (!res.ok) throw new Error(`API ${res.status}`);
  return res.json();
}

export async function getDashboardStats(weddingId: string): Promise<DashboardStats> {
  const [guestData, occData] = await Promise.all([
    fetchJSON<{ guests: RawGuest[]; total: number }>(`/api/weddings/${weddingId}/guests?limit=500`),
    fetchJSON<RawOccupancy[]>(`/api/weddings/${weddingId}/occupancy`)
  ]);

  const guests = guestData.guests ?? [];
  const confirmed = guests.filter(g => g.rsvp === 'confirmed').length;
  const pending = guests.filter(g => g.rsvp === 'pending').length;
  const declined = guests.filter(g => g.rsvp === 'declined').length;
  const checkedIn = guests.filter(g => g.checkedInAt != null).length;
  const totalPax = guests.reduce((s, g) => s + (g.pax ?? 0), 0);

  return {
    totalGuests: guests.length,
    confirmedGuests: confirmed,
    pendingRsvp: pending,
    declined,
    checkedIn,
    totalPax,
    totalTables: (occData ?? []).length,
    occupiedTables: (occData ?? []).filter(o => o.Pax > 0).length,
    averageOccupancy: 0
  };
}

export async function getOccupancy(weddingId: string): Promise<TableOccupancy[]> {
  const [occData, tableData] = await Promise.all([
    fetchJSON<RawOccupancy[]>(`/api/weddings/${weddingId}/occupancy`),
    fetchJSON<{ id: string; name: string; capacity: number }[]>(`/api/weddings/${weddingId}/tables`)
  ]);

  const tableMap = new Map((tableData ?? []).map(t => [t.id, { name: t.name, capacity: t.capacity }]));
  return (occData ?? []).map(o => {
    const t = tableMap.get(o.TableID);
    const capacity = t?.capacity ?? 0;
    return {
      tableId: o.TableID,
      tableName: t?.name || `Table`,
      occupied: o.Pax,
      capacity,
      percentage: capacity > 0 ? Math.round((o.Pax / capacity) * 100) : 0
    };
  });
}

export async function getDashboardData(weddingId: string): Promise<{ stats: DashboardStats; occupancy: TableOccupancy[]; activity: ActivityItem[] }> {
  const [guestData, occData, tableData] = await Promise.all([
    fetchJSON<{ guests: RawGuest[]; total: number }>(`/api/weddings/${weddingId}/guests?limit=500`),
    fetchJSON<RawOccupancy[]>(`/api/weddings/${weddingId}/occupancy`),
    fetchJSON<{ id: string; name: string; capacity: number }[]>(`/api/weddings/${weddingId}/tables`)
  ]);

  const guests = guestData.guests ?? [];

  const stats: DashboardStats = {
    totalGuests: guests.length,
    confirmedGuests: guests.filter(g => g.rsvp === 'confirmed').length,
    pendingRsvp: guests.filter(g => g.rsvp === 'pending').length,
    declined: guests.filter(g => g.rsvp === 'declined').length,
    checkedIn: guests.filter(g => g.checkedInAt != null).length,
    totalPax: guests.reduce((s, g) => s + (g.pax ?? 0), 0),
    totalTables: (occData ?? []).length,
    occupiedTables: (occData ?? []).filter(o => o.Pax > 0).length,
    averageOccupancy: 0
  };

  const tableMap = new Map((tableData ?? []).map(t => [t.id, { name: t.name, capacity: t.capacity }]));
  const occupancy: TableOccupancy[] = (occData ?? []).map(o => {
    const t = tableMap.get(o.TableID);
    const capacity = t?.capacity ?? 0;
    return {
      tableId: o.TableID,
      tableName: t?.name || 'Table',
      occupied: o.Pax,
      capacity,
      percentage: capacity > 0 ? Math.round((o.Pax / capacity) * 100) : 0
    };
  });

  const activity: ActivityItem[] = [...guests]
    .sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime())
    .slice(0, 8)
    .map(g => ({
      id: String(g.id),
      type: g.checkedInAt ? 'check_in' as const : 'rsvp_update' as const,
      message: g.checkedInAt ? 'checked in' : `RSVP: ${g.rsvp}`,
      guestName: g.name,
      timestamp: new Date(g.updatedAt),
      icon: g.checkedInAt ? 'check-circle' : 'user-check'
    }));

  return { stats, occupancy, activity };
}
