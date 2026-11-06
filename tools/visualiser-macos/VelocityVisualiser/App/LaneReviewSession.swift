// LaneReviewSession.swift
// The Lanes to review list's reads (lidar-lane-field-batch-plan Item 6):
// ListLaneProposals for each placed site's cell, asked when city mode
// opens and kept for the session, and the row's open: switch to the site
// and open the painter with Proposals on.
//
// One call is on the wire at a time: the cells are read one after another
// by a single task, and a refresh asked for while it runs is taken up when
// it ends, as the site switch takes a site chosen during another. The list
// does not take the alignment session's wire (`claimWire`): Place from
// hint, which the row's open needs, refuses to start while that is held.
//
// A save or a review in the painter drops its cell's count, which is read
// again the next time city mode opens. A different Gerardus drops them all.

import Combine
import Foundation

@available(macOS 15.0, *) @MainActor final class LaneReviewSession: ObservableObject {
    @Published private(set) var index = LaneReviewIndex()
    /// True while the cells are being read.
    @Published private(set) var isReading = false
    /// The site a row is opening, while it switches, places and paints.
    @Published private(set) var opening: String?
    /// Why the last row chosen did not open the painter.
    @Published private(set) var openError: String?

    let alignment: AlignmentSession
    /// Switches to a site and opens paint mode with Proposals on; returns
    /// why it could not, or nil. AppState's `openLaneReview`.
    var opener: ((CitySite) async -> String?)?

    /// City mode is open: only then are cells read.
    private(set) var isActive = false
    private var sites: [CitySite] = []
    private var reading: Task<Void, Never>?
    /// A refresh asked for while a read ran.
    private var again = false
    /// The server the counts are from; a read answered by another is dropped.
    private var serverKey: String?
    /// Bumped per cell when its count is dropped, so an answer already on
    /// the wire for it is not taken as the new count.
    private var cellGeneration: [String: Int] = [:]
    private var subscriptions = Set<AnyCancellable>()

    init(alignment: AlignmentSession) {
        self.alignment = alignment
        serverKey = Self.key(alignment.serverInfo, alignment.serviceAddress)
        // @Published emits before the value is stored: the emitted info is
        // the new one, and the refresh waits a turn for the session to hold it.
        alignment.$serverInfo.sink { [weak self] info in self?.serverChanged(info) }.store(
            in: &subscriptions)
    }

    /// The task reading the cells, for tests to await.
    var pendingRead: Task<Void, Never>? { reading }

    /// The sites in the list now.
    var rows: [LaneReviewRow] { index.rows(sites) }

    /// Why the list shows no counts, or nil when it can.
    var reason: String? {
        LaneReviewIndex.reason(
            availability: alignment.availability, serverInfo: alignment.serverInfo,
            probeFailure: alignment.probeFailure, sites: sites)
    }

    /// The outstanding count a marker carries; nil while there is none to
    /// show (not counted, or the list has a reason instead).
    func outstanding(siteID: String) -> Int? {
        guard reason == nil, let site = sites.first(where: { $0.id == siteID }),
            let n = index.outstanding(for: site), n > 0
        else { return nil }
        return n
    }

    /// City mode opened or closed. Opening reads what is not yet counted.
    func setActive(_ on: Bool, sites next: [CitySite]) {
        isActive = on
        sites = next
        if on { refresh() }
    }

    /// The site index was read again.
    func sitesChanged(_ next: [CitySite]) {
        guard next != sites else { return }
        sites = next
        refresh()
    }

    /// Reads every cell not yet counted, one call at a time.
    func refresh() {
        guard isActive else { return }
        if reading != nil {
            again = true
            return
        }
        reading = Task { [weak self] in await self?.readAll() }
    }

    /// A save or review at `cell` changed its proposals: its count is
    /// dropped and read again when city mode next opens (at once if open).
    func invalidate(cell: String) {
        guard !cell.isEmpty else { return }
        cellGeneration[cell, default: 0] += 1
        index.entries[cell] = nil
        refresh()
    }

    /// The row's open: switch, place, paint.
    func open(_ site: CitySite) async {
        guard opening == nil, let opener else { return }
        opening = site.id
        openError = nil
        openError = await opener(site)
        opening = nil
    }

    func dismissOpenError() { openError = nil }

    // MARK: - Reading

    /// Written without a `defer` or an early return inside `do`, as
    /// StreetPainterSession.load() is: in that shape the test host aborted
    /// in swift_task_dealloc on the Xcode 26 toolchain.
    private func readAll() async {
        isReading = true
        var more = true
        while more {
            again = false
            for cell in index.cellsToRead(sites) {
                guard isActive, alignment.serverInfo?.supportsLaneFields == true,
                    let client = alignment.gerardusClient
                else { break }
                let key = serverKey
                let generation = cellGeneration[cell, default: 0]
                index.entries[cell] = .reading
                var entry = LaneReviewEntry.reading
                do {
                    entry = .counted(
                        LaneReviewCounts(try await client.listLaneProposals(l16Token: cell)))
                } catch { entry = .failed(GerardusClientError.describe(error)) }
                // Another server, or the count was dropped while asked: the
                // answer is stale, and `again` reads the cell once more.
                if key == serverKey, generation == cellGeneration[cell, default: 0] {
                    index.entries[cell] = entry
                }
            }
            more = again
        }
        isReading = false
        reading = nil
    }

    private func serverChanged(_ info: Gerardus_Align_V1alpha1_GetServerInfoResponse?) {
        let key = Self.key(info, alignment.serviceAddress)
        guard key != serverKey else { return }
        serverKey = key
        index = LaneReviewIndex()
        guard isActive else { return }
        Task { @MainActor [weak self] in self?.refresh() }
    }

    /// The server a count is from: its address and version; nil for none.
    nonisolated static func key(
        _ info: Gerardus_Align_V1alpha1_GetServerInfoResponse?, _ address: GerardusAddress
    ) -> String? { info.map { "\(address.description) \($0.serverVersion)" } }
}

@available(macOS 15.0, *) extension AppState {
    /// A Lanes to review row: switches to `site` with the city view's own
    /// switch (unless it is the site open now), places the sensor from its
    /// hint, then leaves city mode for paint mode with Proposals on. Returns
    /// why the painter did not open, or nil when it did. A site that cannot
    /// be placed leaves city mode as it was, so the list can say why.
    func openLaneReview(_ site: CitySite) async -> String? {
        if siteContext.site?.id != site.id {
            await city.select(site)
            if let error = city.switchError { return error }
            guard siteContext.site?.id == site.id else {
                return "Another site was being opened: choose \(site.title) again."
            }
        }
        if alignment.pose == nil || alignment.reference == nil {
            if let busy = alignment.busy { return "\(site.title): Gerardus is busy (\(busy))." }
            await alignment.placeFromHint()
        }
        // The painter reads the site's marks once the reference is placed;
        // a server without street use says so only in that answer.
        await streets.pendingLoad?.value
        streets.proposals.visible = true
        if let blocker = streets.paintBlocker {
            return "\(site.title): \(alignment.errorMessage ?? blocker)"
        }
        city.setCityMode(false)
        showAlignmentPane = alignment.isAvailable
        streets.setPainting(true)
        guard streets.isPainting else {
            return "\(site.title): \(streets.paintBlocker ?? "paint mode did not open.")"
        }
        return nil
    }
}
