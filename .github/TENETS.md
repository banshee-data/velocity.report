# velocity.report Tenets

These are the non-negotiable principles that govern every decision in this project. When tenets conflict, earlier tenets take precedence.

## 1. Privacy Above All

No cameras. No licence plates. No PII. Velocity measurements only.
Data stays local — no cloud transmission, no external analytics, no tracking.
User data ownership is absolute. If PII reaches a log, a response body, or an export, the system has failed.

## 2. Protect The Vulnerable

This project exists to make streets safer for people who walk, cycle, and play.
A child on a scooter has no crumple zone. Design decisions must weigh safety proportionally. Those with the least protection deserve the most attention.

## 3. Evidence Over Opinion

Decisions are grounded in measured data, not anecdote. Statistical claims require sample sizes, confidence intervals, and reproducible methodology. The record governs.

## 4. Local-First, Offline-Capable

The system runs on a Raspberry Pi in a neighbourhood. It works without internet connectivity. SQLite is the single database. No clustering, no replication, no cloud dependencies.

## 5. Simplicity And Durability

Prefer the smallest change that solves the problem durably. Avoid speculative abstractions. Build for the constraints that exist, not the ones that might.

## 6. British English

All code, comments, documentation, symbols, filenames, and commit messages use British English spelling. Exception: external dependencies or rigid standards that require American spelling.

## 7. DRY

Every project fact has one canonical source. Reference it; do not copy it. When functionality changes, update all relevant documentation.
