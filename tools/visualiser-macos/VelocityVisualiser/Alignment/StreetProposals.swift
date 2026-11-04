// StreetProposals.swift
// Gerardus's lane proposals in the street painter (lane painter plan, Item
// 2): reading them with their profiles, and recording a rejection or a
// reopening with ReviewLaneProposal. Approving is the painter's: it makes a
// mark (StreetPainterSession.approveProposal).
//
// A review follows the painter's Save discipline: its request id is kept
// until the outcome is known, so a retry after a lost answer is the same
// request; a stale row (ABORTED) reads the proposals again; a definite
// refusal drops it.

import Combine
import Foundation
import GRPCCore
import simd

@available(macOS 15.0, *) @MainActor final class StreetProposals: ObservableObject {
    @Published var visible = true { didSet { onChange?() } }
    @Published private(set) var proposals: [LaneProposalView] = []
    @Published var selectedID: String? { didSet { if selectedID != oldValue { onChange?() } } }
    @Published private(set) var isLoading = false
    @Published private(set) var isReviewing = false
    @Published private(set) var errorMessage: String?
    @Published private(set) var modelID = ""
    /// A review whose outcome is not known yet: retried unchanged.
    private(set) var pending: Gerardus_Align_V1alpha1_ReviewLaneProposalRequest?

    private let alignment: AlignmentSession
    var onChange: (() -> Void)?

    init(alignment: AlignmentSession) { self.alignment = alignment }

    var serverSupports: Bool { alignment.serverInfo?.supportsLaneFields == true }
    var selected: LaneProposalView? { proposals.first { $0.featureID == selectedID } }

    /// Proposals by bound (direction), high before medium, then by street.
    var grouped: [(bound: String, proposals: [LaneProposalView])] {
        let groups = Dictionary(grouping: proposals, by: \.bound)
        return groups.keys.sorted().map { bound in
            (
                bound,
                groups[bound]!.sorted {
                    ($0.isHigh ? 0 : 1, $0.street, $0.featureID) < (
                        $1.isHigh ? 0 : 1, $1.street, $1.featureID
                    )
                }
            )
        }
    }

    func clear() {
        proposals = []
        selectedID = nil
        pending = nil
        errorMessage = nil
        onChange?()
    }

    func load() async {
        guard serverSupports, let client = alignment.gerardusClient,
            let token = alignment.site?.l16Token
        else {
            proposals = []
            return
        }
        isLoading = true
        do {
            let r = try await client.listLaneProposals(l16Token: token)
            proposals = r.proposals.map(LaneProposalView.init)
            modelID = r.modelID
            if let id = selectedID, !proposals.contains(where: { $0.featureID == id }) {
                selectedID = nil
            }
            errorMessage = nil
        } catch { errorMessage = "Proposals: " + GerardusClientError.describe(error) }
        isLoading = false
        onChange?()
    }

    /// Rejects or reopens one proposal against the row as listed.
    func review(
        _ featureID: String, _ decision: Gerardus_Align_V1alpha1_LaneProposalDecision,
        author: String, note: String = ""
    ) async {
        guard let client = alignment.gerardusClient, let token = alignment.site?.l16Token,
            let proposal = proposals.first(where: { $0.featureID == featureID })
        else { return }
        let name = author.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else {
            errorMessage = "Give an author in Settings ▸ Servers: Gerardus records who reviewed it."
            return
        }
        var request = Gerardus_Align_V1alpha1_ReviewLaneProposalRequest()
        request.l16Token = token
        request.featureID = featureID
        request.decision = decision
        request.author = name
        request.note = note
        request.expectedRowID = proposal.rowID
        if var held = pending {
            // Only the identical request may reuse the id.
            let id = held.requestID
            held.requestID = ""
            if held == request {
                request.requestID = id
            } else {
                errorMessage =
                    "The previous review has an unknown outcome: retry it unchanged, or read the proposals again."
                return
            }
        } else {
            request.requestID = UUID().uuidString
        }
        pending = request
        isReviewing = true
        do {
            let r = try await client.reviewLaneProposal(request)
            pending = nil
            if let i = proposals.firstIndex(where: { $0.featureID == featureID }) {
                // The answer's proposal carries no profiles: keep the listed ones.
                var updated = LaneProposalView(r.proposal)
                updated.profiles = proposals[i].profiles
                proposals[i] = updated
            }
            errorMessage = nil
        } catch {
            errorMessage = GerardusClientError.describe(error)
            if let rpc = error as? RPCError, rpc.code == .aborted {
                pending = nil
                await load()
                errorMessage =
                    "The proposal changed since it was listed (aborted); it has been read again: review it once more."
            } else if GerardusClientError.isDefiniteRejection(error) {
                pending = nil
            }
        }
        isReviewing = false
        onChange?()
    }

    /// Drops a review whose outcome is unknown and reads the proposals again.
    func discardPending() async {
        pending = nil
        await load()
    }
}
