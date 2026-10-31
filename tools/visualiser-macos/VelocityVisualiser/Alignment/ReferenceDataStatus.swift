// ReferenceDataStatus.swift
// How current a site's stored Gerardus bundle is, and what Update reference
// data and Regenerate… tell the operator. Pure values, so tests need no server.
//
// Checked against Gerardus 0.2.0+a2c2ac10e649 (src/server.rs `prepare_site`,
// src/site.rs `Builder::prepare` and its `lacks_*` checks).

import Foundation
import SwiftProtobuf

enum ReferenceDataStatus {
    /// The layers a bundle built without `refresh` gains when the server can
    /// build them now, the ones GetServerInfo lists only when the builder is
    /// set up for them (`Service::layers`). KERBS, LANE_MARKINGS,
    /// STREET_FURNITURE and TREES are listed whether or not the builder can
    /// make them (they also carry what scans observed), so the list says
    /// nothing about a bundle lacking them; Update still adds them when it
    /// rebuilds for these.
    static let rebuildableLayers: [Gerardus_Align_V1alpha1_ReferenceLayer] = [
        .footprints, .cityStreets, .cityIntersections,
    ]

    /// Layers the server lists and builds that `bundle` has no summary for.
    static func missingLayers(
        bundle: Gerardus_Align_V1alpha1_Site, available: [Gerardus_Align_V1alpha1_ReferenceLayer]
    ) -> [Gerardus_Align_V1alpha1_ReferenceLayer] {
        let held = Set(bundle.layers.map(\.layer))
        return rebuildableLayers.filter { available.contains($0) && !held.contains($0) }
    }

    static func name(_ layer: Gerardus_Align_V1alpha1_ReferenceLayer) -> String {
        switch layer {
        case .aerialPoints: return "3DEP points"
        case .footprints: return "DataSF footprints"
        case .cityStreets: return "City streets"
        case .cityIntersections: return "City intersections"
        case .kerbs: return "Kerbs and islands"
        case .laneMarkings: return "Lane markings"
        case .streetFurniture: return "Street furniture"
        case .trees: return "Street trees"
        default: return "Layer \(layer.rawValue)"
        }
    }

    /// `2026-10-21`, in UTC.
    static func day(_ timestamp: Google_Protobuf_Timestamp) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withFullDate]
        formatter.timeZone = TimeZone(identifier: "UTC")
        return formatter.string(from: timestamp.date)
    }

    /// When the bundle was prepared.
    static func preparedLine(_ bundle: Gerardus_Align_V1alpha1_Site) -> String {
        bundle.hasPreparedAt
            ? "Bundle prepared \(day(bundle.preparedAt))" : "Bundle prepared (date not recorded)"
    }

    /// One line per source: its version and the date it was retrieved.
    static func sourceLines(_ bundle: Gerardus_Align_V1alpha1_Site) -> [String] {
        bundle.sources.filter { ![2, 4].contains($0.layer.rawValue) }.map { source in
            var parts: [String] = []
            if !source.version.isEmpty { parts.append(source.version) }
            if source.hasRetrievedAt { parts.append("retrieved \(day(source.retrievedAt))") }
            let detail = parts.isEmpty ? "no version recorded" : parts.joined(separator: ", ")
            return "\(name(source.layer)): \(detail)"
        }
    }

    /// The out-of-date flag.
    static func outOfDateLine(missing: [Gerardus_Align_V1alpha1_ReferenceLayer]) -> String {
        "Out of date: this Gerardus can build \(missing.map(name).joined(separator: ", ")), which the bundle lacks."
    }

    /// What an Update or Regenerate left, in a line for the pane.
    static func outcome(
        refresh: Bool, before: Set<Gerardus_Align_V1alpha1_ReferenceLayer>,
        after: Set<Gerardus_Align_V1alpha1_ReferenceLayer>
    ) -> String? {
        if refresh { return after.isEmpty ? nil : stillMissing(after, hint: "") }
        guard !before.isEmpty else { return nil }
        if after.isEmpty { return "Added \(list(before)) from the downloads Gerardus holds." }
        if after == before {
            return stillMissing(
                after,
                hint:
                    " Gerardus either returned the stored bundle as it stands (a server from before it rebuilds a bundle that lacks layers) or the site lies outside DataSF's coverage. Regenerate… builds from every source again."
            )
        }
        return "Added \(list(before.subtracting(after)))." + stillMissing(after, hint: "")
    }

    private static func stillMissing(
        _ layers: Set<Gerardus_Align_V1alpha1_ReferenceLayer>, hint: String
    ) -> String { "The bundle still lacks \(list(layers)).\(hint)" }

    private static func list(_ layers: Set<Gerardus_Align_V1alpha1_ReferenceLayer>) -> String {
        layers.sorted { $0.rawValue < $1.rawValue }.map(name).joined(separator: ", ")
    }

    /// One thing Regenerate does, as the confirmation states it.
    struct Consequence: Equatable, Identifiable {
        let title: String
        let detail: String
        var id: String { title }
    }

    /// The confirmation's items, each checked against Gerardus's source (see
    /// the header). The bundle id is a hash of the layers' content: it moves
    /// only when something downloaded or cut differs.
    static func regenerateConsequences(siteName: String) -> [Consequence] {
        [
            Consequence(
                title: "Downloads again",
                detail:
                    "The public sources for \(siteName)'s L13 cells and their neighbours that this Gerardus is set up for: DataSF building footprints, streets with the Street Names list, and kerbs and islands; SFMTA bike lanes, crosswalks, signs, meters and signals; and the Street Tree List. The DataSF and SFMTA sources cover San Francisco only. The cached downloads under <data>/{footprints,streets,kerbs,lane_markings,street_furniture,trees}/l13, and streets/names.json, are overwritten as each arrives, even if a later step fails. Neighbouring sites share those caches and read the newer downloads the next time they rebuild."
            ),
            Consequence(
                title: "Cuts 3DEP again",
                detail:
                    "The 3DEP points and the 1 m ground grid are cut again from the EPT index this Gerardus is set up with, which takes minutes. If the index is out of reach the call fails and the current bundle stays. A Gerardus with no 3DEP index builds a bundle without 3DEP points."
            ),
            Consequence(
                title: "Ids can change",
                detail:
                    "The bundle id is a hash of what was downloaded and cut, so it changes if any of it differs from before. When it does, the site-model id changes too, and so does the model id that exports record (scene-vectors.py writes it as placement.model_id). Features derived from scans are derived again for the new model; the earlier derivations stay in Gerardus's append-only log. A cached roofer reconstruction for the old bundle no longer applies: run gerardus roofer again where it was used."
            ),
            Consequence(
                title: "Kept",
                detail:
                    "Registration records and accepted poses, street-use edits, and the previous bundles under sites/<l16>/bundles/, which stay on disk; only the current link moves."
            ),
        ]
    }
}
