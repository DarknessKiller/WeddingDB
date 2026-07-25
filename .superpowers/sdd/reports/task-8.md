# Task 8: Guest Service

## Status: DONE

## Created
- `backend/internal/services/guest_service.go`

## Verified
- `go build ./internal/services/` — compiles clean

## Commit
- `3d6f2ca` feat(services): guest service with seat validation and check-in/out

## Summary
Thin service layer over GuestRepo and TableRepo. Handles CRUD, search, seat assignment with capacity validation, check-in/out, and table occupancy query.
