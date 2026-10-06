// Package jobs is the contract between the analysis hub, its workers, the
// dashboard and the CLI: what a job is, what makes two results comparable,
// what a worker may be asked to run, and which state a job may move to next.
//
// It does no I/O beyond hashing a file. The hub's store, the worker's loop and
// the HTTP surfaces are separate packages that import this one, so that a
// change to what a job means fails here, in one place, before it fails on a
// host that is switched off.
//
// The design is docs/plans/lidar-worker-pool-and-results-hub-plan.md; the map
// from that design to code is docs/plans/lidar-job-queue-implementation-plan.md.
package jobs
