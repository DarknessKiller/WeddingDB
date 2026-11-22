# 003. Per-column last-write-wins for guest edits

Status: Accepted. Supersedes the per-field rejection in [001](001-offline-sync.md).

## Context

Offline edits replay after reconnect. Online edits arrive as they happen. Both
write to the same `guest_records` row. Two devices can edit one guest while
apart, for example one sets `pax` and the other sets `notes`.

Today the two paths resolve conflicts differently:

- Offline `Sync` compares the mutation's `clientUpdatedAt` with the row's
  `UpdatedAt` and skips the whole mutation when it is older.
- Online `Update` has no timestamp check, so the last request to reach the
  database wins the whole row.

A whole-row rule drops an unrelated edit when a newer write touches a different
column. Example: one device edits `dietary`, another moves the table a second
later. The row-level rule keeps the table move and discards the dietary edit.

## Decision

Every guest column keeps its own write time. A write changes only the columns
it carries. It wins a column when its time is not older than that column's
stored time. Online and offline writes use this same rule.

- Storage: `guest_records.field_times`, a JSON text map from column group to
  RFC 3339 time, using the same text-JSON pattern as `StringSlice`. Added by
  `AutoMigrate`, no backfill. A column with no entry falls back to `updated_at`,
  so a row that predates the column seeds every group from `updated_at` on its
  first merged write.
- Presence: a write changes only the groups it carries. An absent field keeps
  its stored value, an explicit `null` clears a nullable field, and a sparse
  client payload never touches the rest of the row. Patches built in Go rather
  than decoded carry every group.
- Online writes stamp server time on each column they change. Offline writes
  stamp `clientUpdatedAt`, clamped to server time when it is in the future.
- Column groups: `name`, `email`, `pax`, `tableId`+`seatNum`, `rsvp`,
  `checkedInAt`, `notes`, `dietary`, `isVip`, `angbaoAmt`, `giftItem`.
  - `tableId` and `seatNum` form one group with one time. A seat only means
    something with its table, so one device's table move never pairs with
    another device's seat.
- A create stamps every group with its operation time.
- Delete stays row-level. A delete applies only when newer than every column
  write on the row.
- Seat overlap and the guest cap are checked on the merged result, not per
  mutation.
- Ties count as newer, matching current offline code.

## Alternatives

- **Row-level LWW (keep the current rule).** Simplest, but one newer write
  discards unrelated edits on other columns. Rejected: it loses data the
  couple typed.
- **Field-level CRDT.** Overkill for one night and a small door team. Rejected
  in ADR 001 and still rejected.

## Consequences

- `Sync` and `Update` share one merge function. Tests cover that function once.
- A mutation can apply some columns and skip others. Status `skipped` now means
  no column won.
- A sparse online put keeps the fields it omits. Only an explicit `null` clears
  a nullable field.
- Rows that predate the column fall back to `updated_at` until their first merged
  write, which seeds every group with that `updated_at`. After the seed, a sparse
  write moves only the groups it carries.
- A losing mutation is safe, not silent: the response returns `serverRecord`, and
  the client reverts.
- Behaviour rules live in
  [CODING_STANDARDS.md#offline-sync](../../CODING_STANDARDS.md#offline-sync).
