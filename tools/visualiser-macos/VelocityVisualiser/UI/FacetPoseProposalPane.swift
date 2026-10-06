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
                        Text(
                            result.constraint == "normal_position_given_prior_yaw"
                                ? "Edge normal constrained · tangential position retained from prior"
                                : "Planar position constrained given prior yaw"
                        ).font(.caption.bold())
                        Text(
                            "Centre \(result.centre.xM, specifier: "%.2f"), \(result.centre.yM, specifier: "%.2f") ± \(result.centre.boundM, specifier: "%.2f") m"
                        ).font(.caption.monospacedDigit())
                        Text(
                            "Yaw remains prior-only; height is not measured. Orange footprint and hidden ends are predictions from the fixed source body."
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
                }.disabled(features.busy)
            }.onChange(of: inputKey) { _, _ in features.clearPoseProposal() }.onChange(
                of: session.currentSample?.sampleID
            ) { _, _ in confirmed = false }.onChange(of: features.activeID) { _, _ in
                confirmed = false
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

    private func propose() {
        guard ready, let x, let y, let yawDegrees, let positionBound, let yawBoundDegrees,
            let returnBound, let sample = session.currentSample, let feature = features.active,
            let state = features.state, let point = features.registrationPoint
        else { return }
        let request = FacetPoseRequest(
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
