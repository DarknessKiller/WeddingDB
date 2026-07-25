import type { Guest, BanquetTable, DashboardStats, ActivityItem, TableOccupancy } from '$lib/types';
import { DEFAULT_TABLES } from '$lib/constants';

const FIRST_NAMES = [
  'Ah Kow', 'Siew Ping', 'Mei Ling', 'Chong Huat', 'Wei Jie', 'Ah Mui', 'Beng Kiat',
  'Shu Fen', 'Boon Chen', 'Li Kuan', 'Ah Hoy', 'Bee Kiang', 'Beng San', 'Pheck Choy',
  'Soo Leng', 'Bee Keng', 'Choon Seng', 'Kah Hoe', 'Soo Khoon', 'Bee Eng', 'Kim Seng',
  'Mei Hua', 'Bee Leng', 'Kah Wei', 'Choon Wat', 'Siew Lee', 'Ai Ling', 'Wee Kiat',
  'Mei Ying', 'Beng Choo', 'Ah Huat', 'Siew Leng', 'Chin Huat', 'Yee Ling', 'Mun Fai',
  'Peng Cheng', 'Siew Kuan', 'Bee Lian', 'Choon Kiat', 'Ah Hock', 'Siew Ching', 'Wai Keat',
  'Bee Yong', 'Kah Seng', 'Ah Ban', 'Siew Hua', 'Chun Wai', 'Ming Jian', 'Peng Seng',
  'Bee Huay', 'Yee Kuan', 'Ah Teck', 'Siew Wah', 'Keat Mun', 'Bee Gaik', 'Hock Seng',
  'Ah Chieng', 'Siew Yin', 'Choon Hong', 'Bee Kiat', 'Wai Yee', 'Mun Yee', 'Peng Lim',
  'Bee Hwee', 'Yee Ching', 'Ah Kuen', 'Siew Boon', 'Keat Lee', 'Bee Leng', 'Hock Leong',
  'Ah Moy', 'Siew Gek', 'Choon Leng', 'Bee Hoon', 'Wai San', 'Mun Kuen', 'Peng Choon',
  'Bee Koon', 'Yee Siew', 'Ah Guat', 'Siew Tiang', 'Keat Heng', 'Bee Lian', 'Hock Chye',
  'Ah Eng', 'Siew Keng', 'Choon Huay', 'Bee Gek', 'Wai Kuan', 'Mun Leong', 'Peng Huat',
  'Bee Leng', 'Yee Keng', 'Ah Hua', 'Siew Cheng', 'Keat Koon', 'Bee Choo', 'Hock Bee',
  'Ah Kim', 'Siew Poh', 'Choon Bee', 'Bee Hwee', 'Wai Lee', 'Mun Seng', 'Peng Kiat',
  'Bee Gek', 'Yee Boon', 'Ah Lian', 'Siew Huay', 'Keat Seng', 'Bee Keng', 'Hock Koon',
  'Ah Choo', 'Siew Huat', 'Choon Seng', 'Bee Hiong', 'Wai Keng', 'Mun Heng', 'Peng Siew',
  'Bee Kiat', 'Yee Bee', 'Ah Hoon', 'Siew Fong', 'Keat Hwee', 'Beng Choo', 'Hock Guat',
  'Ah Gek', 'Siew Lian', 'Choon Kiat', 'Bee Eng', 'Wai Siew', 'Mun Heng', 'Peng Boon',
  'Bee Huay', 'Yee Ching'
];

const SURNAMES = [
  'Tan', 'Lim', 'Wong', 'Ng', 'Lee', 'Chan', 'Goh', 'Yap', 'Khoo', 'Ong',
  'Chong', 'Sim', 'Teo', 'Foo', 'Koh', 'Ang', 'Phua', 'Loh', 'Seah', 'Yeo',
  'Cheah', 'Chua', 'Tay', 'Neo', 'Wee', 'Heng', 'Teh', 'Tong', 'Lau', 'Fong',
  'Choong', 'Hong', 'Kong', 'Pang', 'Sia', 'Bong', 'Hee', 'Lam', 'Gan', 'Chin'
];

const DIETARY_POOL = [
  'Vegetarian', 'Vegan', 'Halal', 'Gluten-free', 'Nut allergy',
  'Seafood allergy', 'Dairy-free', 'No spicy food', ''
];

const GIFT_ITEMS = [
  'Gold bracelet', 'Gold ring', 'Jade pendant', 'Red packet', 'Cash gift',
  'Crystal vase', 'Wine set', 'Dinnerware set', 'Artwork', 'Perfume set',
  'Watch', 'Diamond earrings', 'Silver photo frame', 'Luxury hamper', ''
];

const NOTES_POOL = [
  'Brings children', 'Needs wheelchair access', 'VIP guest',
  'Performs speech', 'Photographer contact', 'Band member',
  'Family representative', 'Decorations coordinator',
  '', '', '', '', ''
];

function generateGuests(): Guest[] {
  const guests: Guest[] = [];
  const usedNames = new Set<string>();
  const tableOccupancy = new Map<string, number>();

  for (let i = 0; i < 150; i++) {
    let name: string;
    do {
      const surname = SURNAMES[Math.floor(Math.random() * SURNAMES.length)];
      const first = FIRST_NAMES[Math.floor(Math.random() * FIRST_NAMES.length)];
      name = `${surname} ${first}`;
    } while (usedNames.has(name));
    usedNames.add(name);

    const rsvpRoll = Math.random();
    const rsvp: Guest['rsvp'] =
      rsvpRoll < 0.65 ? 'confirmed' :
      rsvpRoll < 0.80 ? 'pending' :
      rsvpRoll < 0.88 ? 'declined' : 'no_response';

    const pax = Math.floor(Math.random() * 4) + 1;
    let tableId: string | null = null;
    let seatNumber: number | null = null;

    if ((rsvp === 'confirmed' || rsvp === 'pending') && Math.random() < 0.45) {
      const available = DEFAULT_TABLES.filter(t => {
        const current = tableOccupancy.get(t.id) ?? 0;
        if (current === 0 && Math.random() < 0.2) return false;
        return current + pax <= t.capacity;
      });
      if (available.length > 0) {
        const picked = available[Math.floor(Math.random() * available.length)];
        tableId = picked.id;
        const current = tableOccupancy.get(picked.id) ?? 0;
        seatNumber = current + 1;
        tableOccupancy.set(picked.id, current + pax);
      }
    }

    const checkedIn = rsvp === 'confirmed' && Math.random() < 0.45;

    let angbaoAmount: number | undefined;
    let giftItem: string | undefined;
    if (checkedIn) {
      if (Math.random() < 0.8) {
        const amounts = [80, 100, 120, 168, 200, 288, 300, 388, 500, 800, 1000];
        angbaoAmount = amounts[Math.floor(Math.random() * amounts.length)];
      }
      if (Math.random() < 0.3) {
        giftItem = GIFT_ITEMS[Math.floor(Math.random() * (GIFT_ITEMS.length - 1))];
      }
    }

    const dietary: string[] = [];
    if (Math.random() < 0.2) {
      dietary.push(DIETARY_POOL[Math.floor(Math.random() * (DIETARY_POOL.length - 1))]);
    }

    guests.push({
      id: `guest-${String(i + 1).padStart(3, '0')}`,
      name,
      phone: `+60 ${Math.floor(Math.random() * 10 + 1)}${String(Math.floor(Math.random() * 100)).padStart(2, '0')}-${String(Math.floor(Math.random() * 1000)).padStart(3, '0')} ${String(Math.floor(Math.random() * 10000)).padStart(4, '0')}`,
      rsvp,
      pax,
      tableId,
      seatNumber,
      checkedIn,
      checkedInAt: checkedIn ? new Date(Date.now() - Math.random() * 86400000) : undefined,
      notes: NOTES_POOL[Math.floor(Math.random() * NOTES_POOL.length)],
      dietaryRequirements: dietary,
      isVip: Math.random() < 0.05,
      angbaoAmount,
      giftItem,
      createdAt: new Date(Date.now() - Math.random() * 30 * 86400000)
    });
  }
  return guests;
}

export const guests: Guest[] = generateGuests();

export const tables: BanquetTable[] = DEFAULT_TABLES;

export function getDashboardStats(): DashboardStats {
  const confirmed = guests.filter(g => g.rsvp === 'confirmed');
  const checkedIn = guests.filter(g => g.checkedIn);
  const totalPax = confirmed.reduce((sum, g) => sum + g.pax, 0);

  const occupiedTableIds = new Set(confirmed.filter(g => g.tableId).map(g => g.tableId));

  return {
    totalGuests: guests.length,
    confirmedGuests: confirmed.length,
    pendingRsvp: guests.filter(g => g.rsvp === 'pending').length,
    declined: guests.filter(g => g.rsvp === 'declined').length,
    checkedIn: checkedIn.length,
    totalPax,
    totalTables: tables.length,
    occupiedTables: occupiedTableIds.size,
    averageOccupancy: Math.round((confirmed.length / (tables.length * 10)) * 100)
  };
}

export function getTableOccupancy(tableId?: string): TableOccupancy | TableOccupancy[] {
  const result = tables.map(t => {
    const tableGuests = guests.filter(g => g.tableId === t.id);
    const occupied = tableGuests.reduce((sum, g) => sum + g.pax, 0);
    return {
      tableId: t.id,
      tableName: t.name,
      occupied,
      capacity: t.capacity,
      percentage: Math.round((occupied / t.capacity) * 100)
    };
  });
  if (tableId !== undefined) {
    return result.find(r => r.tableId === tableId) ?? { tableId, tableName: '', occupied: 0, capacity: 0, percentage: 0 };
  }
  return result;
}

export function getSeatGuest(tableId: string, seatNum: number): Guest | undefined {
  return guests.find(g =>
    g.tableId === tableId &&
    g.seatNumber !== null &&
    seatNum >= g.seatNumber &&
    seatNum < g.seatNumber + g.pax
  );
}

export function isSeatOccupied(tableId: string, seatNum: number): boolean {
  return getSeatGuest(tableId, seatNum) !== undefined;
}

export function getRecentActivity(): ActivityItem[] {
  const activities: ActivityItem[] = [
    { id: 'a1', type: 'check_in', message: 'checked in', guestName: 'Tan Ah Kow', timestamp: new Date(Date.now() - 300000), icon: 'check-circle' },
    { id: 'a2', type: 'rsvp_update', message: 'confirmed attendance', guestName: 'Lim Siew Ping', timestamp: new Date(Date.now() - 600000), icon: 'user-check' },
    { id: 'a3', type: 'reservation', message: 'submitted RSVP', guestName: 'Wong Mei Ling', timestamp: new Date(Date.now() - 900000), icon: 'mail' },
    { id: 'a4', type: 'check_in', message: 'checked in', guestName: 'Lee Wei Jie', timestamp: new Date(Date.now() - 1200000), icon: 'check-circle' },
    { id: 'a5', type: 'table_assignment', message: 'moved to Table 5', guestName: 'Chan Ah Mui', timestamp: new Date(Date.now() - 1500000), icon: 'arrow-right-circle' },
    { id: 'a6', type: 'check_in', message: 'checked in', guestName: 'Yap Shu Fen', timestamp: new Date(Date.now() - 1800000), icon: 'check-circle' },
    { id: 'a7', type: 'rsvp_update', message: 'declined invitation', guestName: 'Goh Beng Kiat', timestamp: new Date(Date.now() - 2100000), icon: 'user-x' },
    { id: 'a8', type: 'check_in', message: 'checked in', guestName: 'Ong Li Kuan', timestamp: new Date(Date.now() - 2400000), icon: 'check-circle' },
  ];
  return activities;
}

export function searchGuests(query: string): Guest[] {
  if (!query.trim()) return [];
  const q = query.toLowerCase();
  return guests
    .filter(g =>
      g.name.toLowerCase().includes(q) ||
      g.phone.includes(q) ||
      (g.email?.toLowerCase().includes(q) ?? false)
    )
    .slice(0, 20);
}

export function getGuestById(id: string): Guest | undefined {
  return guests.find(g => g.id === id);
}

export function getGuestsByTable(tableId: string): Guest[] {
  const table = DEFAULT_TABLES.find(t => t.id === tableId);
  if (!table) return [];
  const tableGuests = guests.filter(g => g.tableId === tableId);
  let totalPax = 0;
  const result: Guest[] = [];
  for (const g of tableGuests) {
    if (totalPax + g.pax > table.capacity) break;
    result.push(g);
    totalPax += g.pax;
  }
  return result;
}

export function addGuest(data: Omit<Guest, 'id' | 'createdAt'>): Guest {
  const guest: Guest = {
    ...data,
    id: `guest-${String(guests.length + 1).padStart(3, '0')}`,
    createdAt: new Date(),
  };
  guests.push(guest);
  return guest;
}
