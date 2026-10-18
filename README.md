# WeddingDB

![Theme](https://img.shields.io/badge/theme-red%20%26%20gold-%23A11217) ![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue) ![Stack: SvelteKit + Go](https://img.shields.io/badge/stack-SvelteKit%20%2B%20Go-blue)

> This project is written with AI agents. If you're against agentic coding, walk away now. No hard feelings.

A Chinese wedding reception runs on a printed guest list. One copy, static, shared by everyone at the door. WeddingDB puts that list on every phone. A small box serves it, and anyone at the door checks guests in from their own device.

Open source under AGPL-3.0. Run it on your own hardware, host it for a venue, change anything you want. No lock-in, no per-guest fees.

## The door is where weddings lose time

Two hundred guests arrive in under two hours. Each one needs a welcome, a table number, and their red packet recorded. A paper list means one queue at one desk, one pen, and a gift ledger transcribed by hand after everyone leaves.

WeddingDB changes the door:

- **Check-in takes seconds.** The receptionist types part of a name, or the pinyin of a Chinese name, and the guest pops up. One tap marks them arrived.
- **The whole team checks in from their phone.** Anyone with the app finds a guest by name and taps check-in, so the reception desk is not one person with a clipboard.
- **Angpao is keyed separately, per guest.** Check-in stays one tap. The angpao amount and gift item are recorded from the guest list, so a declined guest who still sent a gift gets an entry too. CSV or Excel export happens after the night, with no transcription.
- **Seats update live.** Cross off a guest at the door and every screen shows it. A no-show frees a table for a last-minute guest.

## What you get

| You are | WeddingDB gives you |
|---|---|
| Couple or host | Drag-and-drop banquet hall map, table and seat assignment, live occupancy, updates on every screen |
| Reception team | Guest check-in at the door, table and seat lookup, CSV or XLSX angpao reports after the night |
| Guests | Self-service kiosk. Type a name, see the table and seat. No app install, no asking around |

## The guest kiosk

The kiosk removes the queue at the door. A tablet runs WeddingDB in kiosk mode: guests type their name, see their table and seat, and walk in.

Kiosk settings are per wedding: logo, background, welcome message, and hall colors.

## What it does

- Interactive hall map (Konva) to draw the real venue: tables, stage, entrances, obstacles
- Bulk import of up to 1,000 guests from CSV, counted per person per wedding
- Angpao reports as CSV or Excel, ready for thank-you notes
- Multi-wedding support in one deployment, for planners and venues
- JWT auth with refresh-token rotation, token revocation, role-based admin, rate-limited login, and kiosk settings that never touch admin access
- Opt-in tracing with OpenTelemetry and Jaeger, covering requests and DB queries end to end
- Self-hosting with Docker Compose, with PostgreSQL and DragonflyDB/Redis

## Under the hood

The Go API serves the SvelteKit app and talks to PostgreSQL and Redis (DragonflyDB). Docker Compose runs the stack, and the data stays on your machines.

Full setup, environment variables, and the API reference are in the [developer documentation](docs/dev/README.md).
