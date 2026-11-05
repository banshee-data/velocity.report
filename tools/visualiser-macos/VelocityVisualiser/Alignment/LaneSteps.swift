// LaneSteps.swift
// The street painter's steps from a recording to lanes velocity.report
// reports by: what is done, what is next and why, in the order the work goes
// (docs/ui/visualiser/gerardus-alignment.md, "From traffic to lanes").
//
// Pure: built from the painter's state, so the wording and the order are
// tested without a server.

import Foundation

/// One step: its kind, whether it is done, and a line saying what it holds or
/// what to do.
struct LaneStep: Equatable, Identifiable, Sendable {
    enum Kind: String, CaseIterable, Sendable {
        case pose
        case run
        case field
        case sent
        case proposals
        case saved
        case snapshot
    }

    enum State: Equatable, Sendable {
        case done
        /// The first step not done: what to do now.
        case next
        /// Waits on a step before it.
        case waiting
    }

    var kind: Kind
    var state: State
    var title: String
    var detail: String

    var id: String { kind.rawValue }
}

/// What the steps are read from.
struct LaneStepsInput: Equatable, Sendable {
    /// Gerardus takes lane fields (GetServerInfo supports_lane_fields).
    var serverSupportsFields = true
    var poseAccepted = false
    var runID: String?
    /// The run whose recording the server is replaying, when no run is open.
    var replayRunID: String?
    /// The open run's field: its distinct tracks, or nil when there is none.
    var runFieldTracks: Int?
    /// Captures summed in the site's field.
    var siteCaptures = 0
    /// The most distinct vehicle tracks in any one cell of the site's field.
    var siteBusiestVehicleCell = 0
    var proposalsHigh = 0
    var proposalsMedium = 0
    /// Travel lanes among the saved marks.
    var savedLanes = 0
    var unsavedChanges = false
    var snapshotUnsent = false
}

enum LaneSteps {
    /// Gerardus's grades (LANE-PROPOSALS §4.7): distinct vehicle tracks at a
    /// share of the lane's stations.
    static let mediumTracks = 10
    static let highTracks = 30

    static func steps(_ s: LaneStepsInput) -> [LaneStep] {
        var out: [LaneStep] = []
        out.append(
            s.poseAccepted
                ? step(.pose, true, "Pose accepted", "The field and the lanes are placed by it.")
                : step(
                    .pose, false, "Accept a pose",
                    "Leave Paint streets, Register, then press Accept in Decision. The field and the lanes are placed by the accepted pose."
                ))
        if let runID = s.runID {
            out.append(step(.run, true, "Run open", "Run \(short(runID))."))
        } else if let replay = s.replayRunID {
            out.append(
                step(
                    .run, false, "Open this replay as a run",
                    "The server is replaying run \(short(replay))'s recording, but no run is open here. Open it to read its field; the replay starts again."
                ))
        } else {
            out.append(
                step(
                    .run, false, "Open a run",
                    "Pick one in Runs. A field belongs to a run; Live is counted on screen and never sent."
                ))
        }
        if s.runID == nil {
            out.append(step(.field, false, "This run's field", "Needs a run."))
        } else if let tracks = s.runFieldTracks {
            out.append(
                step(.field, true, "This run's field", "\(tracks) track\(tracks == 1 ? "" : "s")."))
        } else {
            out.append(
                step(
                    .field, false, "This run's field",
                    "velocity.report has none for this run: press ↻ to read it again, and see the error below if it fails."
                ))
        }
        guard s.serverSupportsFields else { return ordered(out + savedSteps(s)) }
        out.append(
            s.siteCaptures > 0
                ? step(
                    .sent, true, "Fields at this site",
                    "Gerardus sums \(s.siteCaptures) capture\(s.siteCaptures == 1 ? "" : "s") here. Send this run too if it is not among them: a field sent twice is stored once."
                )
                : step(
                    .sent, false, "Send lane field",
                    "Gerardus holds no field for this site yet. Send sends this run's, placed by the accepted pose."
                ))
        let proposals = s.proposalsHigh + s.proposalsMedium
        if proposals > 0 {
            out.append(
                step(
                    .proposals, true, "Proposals",
                    "\(s.proposalsHigh) high, \(s.proposalsMedium) medium. Approve, edit or reject each below."
                ))
        } else if s.siteCaptures == 0 {
            out.append(
                step(
                    .proposals, false, "Proposals",
                    "Gerardus proposes lanes from the site's summed field: send one first."))
        } else {
            out.append(
                step(
                    .proposals, false, "Proposals",
                    "None yet. The busiest vehicle cell here has \(s.siteBusiestVehicleCell) track\(s.siteBusiestVehicleCell == 1 ? "" : "s"); a medium lane needs \(mediumTracks) distinct vehicle tracks along 60% of its length, a high one \(highTracks) along 80%. Send more recordings of this site: the fields add up."
                ))
        }
        return ordered(out + savedSteps(s))
    }

    private static func savedSteps(_ s: LaneStepsInput) -> [LaneStep] {
        var out: [LaneStep] = []
        if s.unsavedChanges {
            out.append(step(.saved, false, "Save", "Unsaved changes: Save sends them to Gerardus."))
        } else if s.savedLanes > 0 {
            out.append(
                step(
                    .saved, true, "Lanes saved",
                    "\(s.savedLanes) travel lane\(s.savedLanes == 1 ? "" : "s") in Gerardus."))
        } else {
            out.append(
                step(
                    .saved, false, "Approve and save",
                    "Approve proposals, or draw lanes with the Lane tool, then Save."))
        }
        if s.snapshotUnsent {
            out.append(
                step(
                    .snapshot, false, "Send lanes to velocity.report",
                    "Saved but not sent: velocity.report's reports still use the previous lanes."))
        } else if s.savedLanes > 0 && !s.unsavedChanges {
            out.append(
                step(
                    .snapshot, true, "Lanes to velocity.report",
                    "Nothing saved in this session is waiting to be sent."))
        } else {
            out.append(
                step(
                    .snapshot, false, "Send lanes to velocity.report",
                    "After a save, so its reports split speeds by lane and direction."))
        }
        return out
    }

    /// The first step not done is next; the others not done wait on it.
    private static func ordered(_ steps: [LaneStep]) -> [LaneStep] {
        var seenNext = false
        return steps.map { s in
            guard s.state != .done else { return s }
            var s = s
            s.state = seenNext ? .waiting : .next
            seenNext = true
            return s
        }
    }

    private static func step(
        _ kind: LaneStep.Kind, _ done: Bool, _ title: String, _ detail: String
    ) -> LaneStep {
        LaneStep(kind: kind, state: done ? .done : .waiting, title: title, detail: detail)
    }

    private static func short(_ id: String) -> String { String(id.prefix(8)) }

    /// The most distinct tracks in one vehicle cell of a field.
    static func busiestVehicleCell(_ grid: LaneFieldGrid?) -> Int {
        Int(grid?.groups[.vehicle]?.tracks.max() ?? 0)
    }

    /// The run whose recording a replay plays: the run whose stored path is
    /// the one being replayed, else the one run whose recording has the same
    /// directory name (the server opens the canonical path, which may differ
    /// from the stored one by a symlink). Nil when none, or more than one,
    /// matches.
    static func replayRun(
        vrlogPath: String?, runs: [(runID: String, vrlogPath: String?)]
    ) -> String? {
        guard let path = vrlogPath?.trimmingCharacters(in: .whitespaces), !path.isEmpty else {
            return nil
        }
        let trimmed = trimSlash(path)
        if let exact = runs.first(where: { $0.vrlogPath.map(trimSlash) == trimmed }) {
            return exact.runID
        }
        let name = (trimmed as NSString).lastPathComponent
        let byName = runs.filter {
            guard let p = $0.vrlogPath, !p.isEmpty else { return false }
            return (trimSlash(p) as NSString).lastPathComponent == name
        }
        return byName.count == 1 ? byName[0].runID : nil
    }

    private static func trimSlash(_ p: String) -> String {
        p.count > 1 && p.hasSuffix("/") ? String(p.dropLast()) : p
    }
}
