# WeddingDB

![Theme](https://img.shields.io/badge/theme-red%20%26%20gold-%23A11217) ![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue) ![Stack: SvelteKit + Go](https://img.shields.io/badge/stack-SvelteKit%20%2B%20Go-blue)

The night of a Chinese wedding reception runs on a printed spreadsheet. One copy, static, shared by everyone at the door. WeddingDB is that list, made live. A small box runs it, and the people who used to pass around a clipboard check guests in from their phones.

Open source under AGPL-3.0: run it on your own hardware, host it for a venue, change anything you want. No lock-in, no per-guest fees.

## The door is where weddings lose time

Two hundred guests arrive in under two hours. Each one needs a welcome, a table number, and their red packet recorded. With a paper list that means one queue at one desk, one pen, and a gift ledger transcribed by hand after everyone leaves.

WeddingDB changes the door:

- **Check-in takes seconds, not a line.** The receptionist types part of a name, or the pinyin of a Chinese name, and the guest pops up. One tap checks them in and records the angpao at the same time. No flipping pages, no squinting at handwriting.
- **The whole team checks in from their pocket.** Anyone with the app finds a guest by name and taps check-in. The reception desk stops being one person with a clipboard.
- **Angpao is recorded at the same tap.** Name, amount, table. It exports to CSV or Excel after the night, no transcription, no deciphering handwriting.
- **Seats update live.** Cross off a guest at the door and every screen shows it. A no-show frees a table and a last-minute guest takes it.

## What You Get

| You're the... | WeddingDB handles |
|---|---|
| **Couple / host** | Drag-and-drop banquet hall map, table and seat assignment, live occupancy, real-time updates across all screens |
| **Reception team** | Guest lookup and check-in at the door, instant table and seat find, CSV/XLSX angpao reports at the end of the night |
| **Guests** | Self-service kiosk: type your name, see your table and seat, no app install, no asking around |

## The Guest Kiosk

The kiosk removes the door queue entirely. A tablet runs WeddingDB in kiosk mode: guests type their name, get their table and seat, and walk in. No desk, no queue, no "which table am I?"

It's customizable per wedding: your own logo and background, a welcome message, hall colors.

## Built for One Night, Engineered to Last

- **Interactive hall map** (Konva-based) to draw your actual venue: tables, stage, entrances, obstacles
- **Bulk import** of up to 1,000 guests from CSV
- **Angpao reports** as CSV or Excel, ready for thank-you notes
- **Multi-wedding support** in one deployment, useful for planners and venues
- **Secure by default** with JWT auth, refresh-token rotation, token revocation, role-based admin, rate-limited login, and per-wedding kiosk settings that never touch admin access
- **Self-hostable** with Docker Compose; data stays yours (PostgreSQL + DragonflyDB/Redis)

## Under the Hood

Self-hostable, so the data never leaves your machines: Docker Compose runs the whole stack (PostgreSQL + DragonflyDB/Redis) in minutes. Multi-wedding, so one deployment covers planners and venues running many events.

Developers: full setup, environment variables, and the complete API reference live in the [Developer Documentation](docs/dev/README.md).

---

*The day should be about the couple, not the paper list.*
