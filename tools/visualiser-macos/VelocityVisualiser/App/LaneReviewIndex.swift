// LaneReviewIndex.swift
// The Lanes to review list's counts (lidar-lane-field-batch-plan Item 6):
// each placed site's lane proposals, as ListLaneProposals answers them,
// counted by grade and by review, and the sites sorted by what is left to
// review. Nothing here talks to a server; LaneReviewSession does.
//
// A proposal is Gerardus's row and changes under review, so the counts live
// nowhere static: `city/sites.json` carries none. They are keyed by L16
// cell, the token ListLaneProposals takes, so two index sites at one cell
// share one call and one count.

import Foundation

/// One Gerardus site's live proposals, counted.
///
/// `high` and `medium` count every proposal of that grade, adopted or
/// rejected or not. `adopted`, `rejected` and `outstanding` count by review:
/// outstanding is neither adopted nor rejected, what an operator has left
/// to look at.
struct LaneReviewCounts: Equatable, Sendable {
    var total = 0
    var high = 0
    var medium = 0
    var adopted = 0
    var rejected = 0
    var outstanding = 0

    init() {}

    /// Read as the painter reads them (LaneProposalView): the grade is the
    /// `confidence` tag, adopted is a non-empty `adopted_by`, rejected is a
    /// latest review that rejects.
    init(_ proposals: [LaneProposalView]) {
        total = proposals.count
        for p in proposals {
            switch p.grade {
            case "high": high += 1
            case "medium": medium += 1
            default: break
            }
            if p.isAdopted { adopted += 1 }
            if p.rejected { rejected += 1 }
            if !p.isAdopted && !p.rejected { outstanding += 1 }
        }
    }

    init(_ response: Gerardus_Align_V1alpha1_ListLaneProposalsResponse) {
        self.init(response.proposals.map(LaneProposalView.init))
    }

    /// The row's help: every count in one line.
    var summary: String {
        guard total > 0 else { return "No proposals" }
        let noun = total == 1 ? "proposal" : "proposals"
        return "\(total) \(noun): \(high) high, \(medium) medium; \(adopted) adopted, "
            + "\(rejected) rejected, \(outstanding) outstanding"
    }
}

/// What is known of one cell's proposals.
enum LaneReviewEntry: Equatable, Sendable {
    /// ListLaneProposals is on the wire for it.
    case reading
    case counted(LaneReviewCounts)
    /// The call failed; the reason, as the pane shows a gRPC status.
    case failed(String)

    var counts: LaneReviewCounts? {
        if case .counted(let c) = self { return c }
        return nil
    }
}

/// One row of the list: a placed site and what is known of its cell.
struct LaneReviewRow: Identifiable, Equatable, Sendable {
    var site: CitySite
    /// Nil until the cell is asked for.
    var entry: LaneReviewEntry?

    var id: String { site.id }
    var counts: LaneReviewCounts? { entry?.counts }
}

/// The counts the list holds for the session, by L16 cell.
struct LaneReviewIndex: Equatable, Sendable {
    var entries: [String: LaneReviewEntry] = [:]

    /// The sites the list counts: placed, with a cell to ask Gerardus about.
    static func reviewable(_ sites: [CitySite]) -> [CitySite] {
        sites.filter { $0.isPlaced && !$0.cells.l16.isEmpty }
    }

    /// Each reviewable site's cell once, in the index's order.
    static func cells(_ sites: [CitySite]) -> [String] {
        var seen = Set<String>()
        return reviewable(sites).map(\.cells.l16).filter { seen.insert($0).inserted }
    }

    /// The cells still to ask: never asked, or failed last time. A cell
    /// counted is kept for the session until a save or review there drops it.
    func cellsToRead(_ sites: [CitySite]) -> [String] {
        Self.cells(sites).filter { cell in
            switch entries[cell] {
            case nil, .failed: return true
            case .reading, .counted: return false
            }
        }
    }

    func entry(for site: CitySite) -> LaneReviewEntry? {
        site.cells.l16.isEmpty ? nil : entries[site.cells.l16]
    }

    /// The outstanding count a site's marker carries; nil unless counted.
    func outstanding(for site: CitySite) -> Int? { entry(for: site)?.counts?.outstanding }

    /// The list: most outstanding first. Sites still being read follow the
    /// counted ones, and sites whose call failed come last; ties go by
    /// title, then id, so the order does not move between redraws.
    func rows(_ sites: [CitySite]) -> [LaneReviewRow] {
        func rank(_ row: LaneReviewRow) -> (Int, Int) {
            switch row.entry {
            case .counted(let c): return (0, -c.outstanding)
            case .reading, nil: return (1, 0)
            case .failed: return (2, 0)
            }
        }
        return Self.reviewable(sites).map { LaneReviewRow(site: $0, entry: entry(for: $0)) }.sorted
        { a, b in
            let (ra, rb) = (rank(a), rank(b))
            if ra != rb { return ra < rb }
            if a.site.title != b.site.title { return a.site.title < b.site.title }
            return a.site.id < b.site.id
        }
    }

    /// Why the list shows no counts at all, or nil when it can: no Gerardus
    /// has answered, or the one that has serves no lane proposals. Counts
    /// are never shown as zeros for a server that could not give them.
    static func reason(
        availability: AlignmentSession.Availability,
        serverInfo: Gerardus_Align_V1alpha1_GetServerInfoResponse?, probeFailure: String?,
        sites: [CitySite]
    ) -> String? {
        switch availability {
        case .unknown: return "No Gerardus server has answered yet. The proposals are Gerardus's."
        case .unavailable:
            let why = probeFailure.map { " (\($0))" } ?? ""
            return "Gerardus cannot be reached\(why). The proposals are Gerardus's."
        case .available(let version):
            guard serverInfo?.supportsLaneFields == true else {
                let named = version.isEmpty ? "This Gerardus" : "This Gerardus (\(version))"
                return "\(named) serves no lane proposals: it reports supports_lane_fields false."
            }
        }
        if reviewable(sites).isEmpty { return "No placed site in the site index." }
        return nil
    }
}
