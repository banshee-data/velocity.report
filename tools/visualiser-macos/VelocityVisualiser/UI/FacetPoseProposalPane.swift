import SwiftUI

/// Explicit priors only. No defaults are borrowed from target reference truth.
struct FacetPoseProposalPane: View {
    @ObservedObject var session: AnnotationSession
    @ObservedObject var features: FeatureAuthoring
    @State private var x: Double?
    @State private var y: Double?
    @State private var yawDegrees: Double?
    @State private var positionBound: Double?
    @State private var yawBoundDegrees: Double?
    @State private var returnBound: Double?
    @State private var priorSource = ""
    @State private var identity = ""
    @State private var confirmed = false

    var body: some View {
        if features.active?.hasAnchor == true {
            DisclosureGroup("Offline pose from matched facet · experiment") {
                VStack(alignment: .leading, spacing: 6) {
                    Text(
                        "Use this frame's saved definite facet support. Confirm a physical spot, or two widely separated direction returns on a straight edge, using the return pickers above."
                    ).font(.caption2).foregroundStyle(.secondary)
                    Text("Explicit prior · target reference is not an input").font(.caption.bold())
                    HStack {
                        TextField("Prior X m", value: $x, format: .number)
                        TextField("Prior Y m", value: $y, format: .number)
                    }
                    TextField("Prior yaw °", value: $yawDegrees, format: .number)
                    TextField("Prior position ± m", value: $positionBound, format: .number)
                    TextField("Prior yaw ± °", value: $yawBoundDegrees, format: .number)
                    TextField(
                        "Name prior source and its uncertainty assumptions", text: $priorSource)
                    TextField(
                        "Matched-return uncertainty ± m", value: $returnBound, format: .number)
                    TextField("Same physical spot/edge and bound assumptions", text: $identity)
                    Toggle("I confirm these returns match the saved facet", isOn: $confirmed).font(
                        .caption)
                    Button("Propose body pose · read-only assisted") { propose() }.disabled(
                        !ready || features.poseProposalBusy)
                    if features.poseProposalBusy { ProgressView().controlSize(.small) }
                    if let result = session.currentFacetPoseProposal {
                        let measuredYaw =
                            result.constraint == FacetPoseProposal.normalPositionAndYaw
                        let yawSource: String =
                            measuredYaw ? "measured from the edge normals" : "prior only"
                        let yawDegrees: Double = result.yaw.rad * 180 / Double.pi
                        let yawBoundDegrees: Double = result.yaw.boundRad * 180 / Double.pi
                        Text(
                            measuredYaw
                                ? "Edge normal and yaw measured · tangential position retained from prior"
                                : "Planar position constrained given prior yaw"
                        ).font(.caption.bold())
                        Text(
                            "Centre \(result.centre.xM, specifier: "%.2f"), \(result.centre.yM, specifier: "%.2f") ± \(result.centre.boundM, specifier: "%.2f") m"
                        ).font(.caption.monospacedDigit())
                        Text(
                            "Yaw \(yawDegrees, specifier: "%.1f")° ± \(yawBoundDegrees, specifier: "%.1f")° · \(yawSource)"
                        ).font(.caption2.monospacedDigit())
                        Text(
                            measuredYaw
                                ? "Height is not measured. Orange footprint and hidden ends are predictions from the fixed source body."
                                : "Yaw remains prior-only; height is not measured. Orange footprint and hidden ends are predictions from the fixed source body."
                        ).font(.caption2).foregroundStyle(.orange)
                        if let normal = result.normal, let bound = result.normalBoundM,
                            let tangent = result.tangentBoundM
                        {
                            Text(
                                "Pack X/Y normal (\(normal[0], specifier: "%.2f"), \(normal[1], specifier: "%.2f")) ± \(bound, specifier: "%.2f") m · tangent retains prior ± \(tangent, specifier: "%.2f") m"
                            ).font(.caption2.monospacedDigit())
                        }
                        if let box = result.box {
                            Text(
                                "Fixed source body \(result.body.bodyID) · \(box.lengthM, specifier: "%.2f") × \(box.widthM, specifier: "%.2f") m"
                            ).font(.caption2.monospacedDigit())
                        }
                        if let reason = result.boxUnavailable {
                            Text(reason).font(.caption2).foregroundStyle(.orange)
                        }
                        Text(
                            "Source physical revision \(result.sourcePhysicalRevision) · facet revision \(result.request.featureRevision) · membership \(result.request.membershipRevision)"
                        ).font(.caption2).textSelection(.enabled)
                        Text(
                            "Inspect/correct the explicit inputs and recompute, or discard. This experiment has no Accept-to-reference or proposal-save operation."
                        ).font(.caption2).foregroundStyle(.secondary)
                        Button("Discard pose proposal") { features.clearPoseProposal() }
                    }
                    jointControls
                }.disabled(features.busy)
            }.onChange(of: inputKey) { _, _ in features.clearPoseProposal() }.onChange(
                of: session.currentSample?.sampleID
            ) { _, _ in
                confirmed = false
                features.clearJointSelection()
            }.onChange(of: features.activeID) { _, _ in
                confirmed = false
                identity = ""
            }.onChange(of: session.operatorName) { _, _ in
                confirmed = false
                features.clearJointSelection()
            }
        }
    }

    private var inputKey: [String] {
        [x, y, yawDegrees, positionBound, yawBoundDegrees, returnBound].map {
            $0.map(String.init(describing:)) ?? ""
        } + [
            priorSource, identity, String(confirmed),
            String(describing: features.registrationPoint),
            String(describing: features.registrationEndPoint),
        ]
    }

    private var ready: Bool {
        guard confirmed, let x, let y, let yawDegrees, let positionBound, let yawBoundDegrees,
            let returnBound,
            [x, y, yawDegrees, positionBound, yawBoundDegrees, returnBound].allSatisfy(\.isFinite),
            positionBound >= 0, yawBoundDegrees >= 0, yawBoundDegrees <= 180, returnBound > 0,
            !priorSource.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            !identity.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            !session.operatorName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            let sample = session.currentSample, !session.dirtySamples.contains(sample.sampleID),
            !session.physical.isDirty, !features.isDirty, !features.busy,
            features.state?.document.knownEditingContract == true,
            features.state?.digest.isEmpty == false,
            features.active?.anchor.sourceSample != UInt32(exactly: sample.sampleID),
            features.active?.objectID == session.activeObjectID,
            let support = session.featureOverlay, support.decision == .acceptedProposal,
            session.facetSupportSummary(observation: support).fraction != nil,
            features.registrationPoint != nil
        else { return false }
        return features.active?.anchor.hasLine != true || features.registrationEndPoint != nil
    }

    private var currentRequest: FacetPoseRequest? {
        guard ready, let x, let y, let yawDegrees, let positionBound, let yawBoundDegrees,
            let returnBound, let sample = session.currentSample, let feature = features.active,
            let state = features.state, let point = features.registrationPoint
        else { return nil }
        return FacetPoseRequest(
            pack: PhysicalReferenceAPIClient.handle(for: session.pack.directory),
            packDigest: session.pack.manifest.packDigest, featureID: feature.featureID,
            featureRevision: state.document.revision, featureDigest: state.digest,
            membershipRevision: session.sidecar.revision,
            membershipDigest: session.membershipDigest, sampleID: sample.sampleID,
            timestampNs: sample.timestampNs, pointIndex: point,
            endIndex: feature.anchor.hasLine ? features.registrationEndPoint : nil,
            returnBoundM: returnBound, confirmed: confirmed, author: session.operatorName,
            identityNote: identity,
            prior: FacetPosePrior(
                xM: x, yM: y, yawRad: yawDegrees * .pi / 180, positionBoundM: positionBound,
                yawBoundRad: yawBoundDegrees * .pi / 180, source: priorSource))
    }

    private func propose() {
        guard let request = currentRequest, let feature = features.active,
            let sample = session.currentSample
        else { return }
        Task {
            if await features.proposePose(request), let result = session.currentFacetPoseProposal {
                let source =
                    "Offline facet pose proposal: \(feature.featureID), facet revision \(request.featureRevision), source physical \(result.sourcePhysicalRevision), prior \(priorSource)"
                features.recordPoseProposalExposure(
                    objectID: feature.objectID, sampleID: sample.sampleID, source: source)
                session.physical.expose(objectIDs: [feature.objectID], source: source)
            }
        }
    }
}

extension AnnotationSession {
    var currentFacetPoseProposal: FacetPoseProposal? {
        guard let result = features.poseProposal, let sample = currentSample,
            result.request.sampleID == sample.sampleID,
            result.request.timestampNs == sample.timestampNs,
            result.request.membershipDigest == membershipDigest,
            result.request.membershipRevision == sidecar.revision,
            result.request.featureDigest == features.state?.digest,
            result.request.featureID == features.activeID, result.objectID == activeObjectID,
            !dirtySamples.contains(sample.sampleID), !physical.isDirty
        else { return nil }
        return result
    }
}

extension FacetPoseProposalPane {
    private var jointControls: some View {
        VStack(alignment: .leading, spacing: 6) {
            Divider()
            Text("Joint pose · confirmed facets").font(.caption.bold())
            Text(
                "Use the same return pickers and confirmation above for each saved facet. Add two to four complementary matches at this frame."
            ).font(.caption2).foregroundStyle(.secondary)
            Button("Add or replace this match") { stageMatch() }.disabled(
                !ready || features.poseProposalBusy)
            ForEach(features.jointMatches, id: \.featureID) { match in
                HStack {
                    let name =
                        features.state?.document.features.first(where: {
                            $0.featureID == match.featureID
                        })?.name ?? "Facet"
                    Text("\(name) · \(match.featureID.prefix(8))").font(.caption2)
                    Spacer()
                    Button("Remove") { features.removeJointMatch(featureID: match.featureID) }.font(
                        .caption2)
                }
            }
            HStack {
                Button("Propose joint body pose") { proposeJoint() }.disabled(!jointReady)
                Button("Clear matches") { features.clearJointSelection() }.disabled(
                    features.jointMatches.isEmpty)
            }
            if let result = session.currentJointFacetPoseProposal {
                Text(
                    "\(result.confidence.state.capitalized) geometric support · rank \(result.confidence.dataRank)/3"
                ).font(.caption.bold()).foregroundStyle(.orange)
                Text(
                    "Position: \(result.confidence.position.replacingOccurrences(of: "_", with: " ")) · heading: \(result.confidence.yaw.replacingOccurrences(of: "_", with: " "))"
                ).font(.caption2)
                DisclosureGroup("Evidence versions") {
                    Text(
                        "Fixed source body \(result.body.bodyID) · physical revision \(result.contributors.first?.sourcePhysicalRevision ?? 0)"
                    ).font(.caption2).textSelection(.enabled)
                    Text(
                        "Facet revision \(result.request.featureRevision) · membership \(result.request.membershipRevision)"
                    ).font(.caption2)
                }.font(.caption2)
                Text(
                    "Centre \(result.centre.xM, specifier: "%.2f"), \(result.centre.yM, specifier: "%.2f") ± \(result.centre.boundM, specifier: "%.2f") m"
                ).font(.caption.monospacedDigit())
                Text(
                    "Yaw \(result.yaw.rad * 180 / .pi, specifier: "%.1f")° ± \(result.yaw.boundRad * 180 / .pi, specifier: "%.1f")°"
                ).font(.caption2.monospacedDigit())
                ForEach(Array(result.confidence.reasons.enumerated()), id: \.offset) { _, reason in
                    Text(reason).font(.caption2).foregroundStyle(.secondary)
                }
                ForEach(Array(result.unresolved.enumerated()), id: \.offset) { _, reason in
                    Text(unresolvedTitle(reason)).font(.caption2).foregroundStyle(.orange).help(
                        reason)
                }
                if let reason = result.boxUnavailable {
                    Text(reason).font(.caption2).foregroundStyle(.orange)
                }
            }
            Text(
                "Support states are not calibrated match probabilities. Hidden endpoints are predictions from the pinned body. This read-only experiment does not save a reference or update the tracker. Planar surface registration is not implemented."
            ).font(.caption2).foregroundStyle(.secondary)
        }
    }

    private func unresolvedTitle(_ reason: String) -> String {
        switch reason {
        case "height_not_measured": return "Height is not measured."
        case "yaw_from_prior_not_measured": return "Heading remains from the prior."
        case "tangent_from_prior_not_measured":
            return "Position along the edge remains from the prior."
        default: return "An additional pose component remains unresolved."
        }
    }

    private var jointContext: FacetJointContext? {
        guard let sample = session.currentSample, let state = features.state,
            let objectID = session.activeObjectID
        else { return nil }
        return FacetJointContext(
            objectID: objectID, sampleID: sample.sampleID, timestampNs: sample.timestampNs,
            featureRevision: state.document.revision, featureDigest: state.digest,
            membershipRevision: session.sidecar.revision, membershipDigest: session.membershipDigest
        )
    }

    private var jointReady: Bool {
        guard let context = jointContext, context == features.jointContext,
            (2...4).contains(features.jointMatches.count), !features.poseProposalBusy,
            !features.busy, !features.isDirty, features.canEdit, !session.physical.isDirty,
            !session.dirtySamples.contains(context.sampleID), let x, let y, let yawDegrees,
            let positionBound, let yawBoundDegrees,
            [x, y, yawDegrees, positionBound, yawBoundDegrees].allSatisfy(\.isFinite),
            positionBound >= 0, yawBoundDegrees >= 0, yawBoundDegrees <= 180,
            !priorSource.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
            !session.operatorName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else { return false }
        return true
    }

    private func stageMatch() {
        guard let request = currentRequest, let context = jointContext else { return }
        _ = features.stageJointMatch(
            FacetJointMatch(
                featureID: request.featureID, pointIndex: request.pointIndex,
                endIndex: request.endIndex, returnBoundM: request.returnBoundM,
                confirmed: request.confirmed, identityNote: request.identityNote), context: context)
    }

    private func proposeJoint() {
        guard jointReady, let context = jointContext, let x, let y, let yawDegrees,
            let positionBound, let yawBoundDegrees
        else { return }
        let request = FacetJointPoseRequest(
            pack: PhysicalReferenceAPIClient.handle(for: session.pack.directory),
            packDigest: session.pack.manifest.packDigest, featureRevision: context.featureRevision,
            featureDigest: context.featureDigest, membershipRevision: context.membershipRevision,
            membershipDigest: context.membershipDigest, sampleID: context.sampleID,
            timestampNs: context.timestampNs, author: session.operatorName,
            prior: FacetPosePrior(
                xM: x, yM: y, yawRad: yawDegrees * .pi / 180, positionBoundM: positionBound,
                yawBoundRad: yawBoundDegrees * .pi / 180, source: priorSource),
            matches: features.jointMatches)
        Task {
            if await features.proposeJointPose(request),
                let result = session.currentJointFacetPoseProposal
            {
                let source =
                    "Offline joint facet pose proposal: \(request.matches.map(\.featureID).joined(separator: ", ")), facet revision \(request.featureRevision), prior \(request.prior.source)"
                features.recordPoseProposalExposure(
                    objectID: result.objectID, sampleID: context.sampleID, source: source)
                session.physical.expose(objectIDs: [result.objectID], source: source)
            }
        }
    }
}
