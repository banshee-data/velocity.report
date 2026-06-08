```
████████ ███████ ███    ██ ███████ ████████ ███████
   ██    ██      ████   ██ ██         ██    ██
   ██    █████   ██ ██  ██ █████      ██    ███████
   ██    ██      ██  ██ ██ ██         ██         ██
   ██    ███████ ██   ████ ███████    ██    ███████
```

These are the non-negotiable principles that govern every decision in this project.
When tenets conflict, earlier tenets take precedence.

## 1. Privacy above all

No cameras. No licence plates. No PII. Velocity measurements only.
Measurement data stays local: no cloud transmission of traffic observations,
no external analytics, no tracking.
Remote web requests are explicit, optional, and disableable. Examples include
operator-enabled OpenStreetMap tile or Overpass downloads and opt-in geometry
prior fetches. These requests must not send PII, vehicle data, or raw sensor
captures, and the system must still work without them.
User data ownership is absolute.
If PII reaches a log, a response body, or an export, the system has failed.

## 2. Protect the vulnerable

This project exists to make streets safer for people who walk, cycle, and play.
A child on a scooter has no crumple zone. Design decisions must weigh safety proportionally.
Those with the least protection deserve the most attention.

## 3. Evidence over opinion

Decisions are grounded in measured data, not anecdote.
Statistical claims require sample sizes, confidence intervals, and reproducible methodology.
The record governs.

## 4. Local-first, offline-capable

The system runs on a Raspberry Pi in a neighbourhood. It works without internet connectivity.
SQLite is the single database. No clustering, no replication, no cloud dependencies.
Network-enriched features are additive only: OSM tile download, Overpass lookup,
and remote prior fetches are optional operator choices, not required runtime
dependencies.

## 5. Simplicity and durability

Prefer the smallest change that solves the problem durably. Avoid speculative abstractions.
Build for the constraints that exist, not the ones that might.

## 6. DRY

Every project fact has one canonical source. Reference it; do not copy it.
When functionality changes, update all relevant documentation.
