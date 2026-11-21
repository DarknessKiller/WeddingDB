import type { TableOccupancy } from '#lib/types/index.ts';
import { getOccupancy as getRawOccupancy, listTables } from './tables';

export async function getOccupancy(weddingId: string): Promise<TableOccupancy[]> {
  const [occData, tableData] = await Promise.all([getRawOccupancy(weddingId), listTables(weddingId)]);

  const tableMap = new Map((tableData ?? []).map(t => [t.id, { name: t.name, capacity: t.capacity, isVip: t.isVip ?? false }]));
  const tableNameCollator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
  return occData.map(o => {
    const t = tableMap.get(o.TableID);
    const capacity = t?.capacity ?? 0;
    return {
      tableId: o.TableID,
      tableName: t?.name || `Table`,
      occupied: o.Pax,
      capacity,
      percentage: capacity > 0 ? Math.round((o.Pax / capacity) * 100) : 0,
      isVip: t?.isVip ?? false
    };
  }).sort((a, b) => a.isVip !== b.isVip ? (a.isVip ? -1 : 1) : tableNameCollator.compare(a.tableName, b.tableName));
}
