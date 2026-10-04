// A report seed borrows a stated physical body, never a medoid or visible OBB.
// It is an assisted sketch. Statistical sigmas do not become conservative
// reference bounds, and no imported component claims observed evidence.
import Foundation

struct PhysicalTrackerSeed: Equatable {
    var objectID: String
    var sampleID: Int
    var source: String
    var prediction: ReportPrediction
    var body: PhysicalBody
    var keyframe: PhysicalKeyframe

    /// A report prediction that has passed every rule a seed must meet, and
    /// the parts the seed is built from. Nothing is minted: no record IDs, no
    /// timestamps. Checking is cheap enough for a view to ask on every
    /// render, and `make` builds from this same answer, so what the pane
    /// offers and what the import does cannot disagree.
    struct Eligible: Equatable {
        var sampleID: Int
        var prediction: ReportPrediction
        var heading: ReportPrediction.Heading
        var length: ReportPrediction.Extent
        var width: ReportPrediction.Extent
        var estimator: String
    }

    static func check(
        instant: ReportInstant, identity: ReportArmIdentity, reportDigest: String,
        reportPackDigest: String, packDigest: String, objectID: String, sample: AnnotationSample,
        author: String
    ) throws -> Eligible {
        guard reportPackDigest == packDigest, instant.objectID == objectID,
            instant.sampleID == sample.sampleID, instant.timestampNs == sample.timestampNs,
            let p = instant.prediction, p.timestampNs == sample.timestampNs
        else {
            throw FeatureError.message(
                "The estimate does not name this pack, object and exact frame.")
        }
        guard let match = instant.match, match.trackKey == p.trackKey, match.offsetNs == 0,
            match.candidates == 1
        else {
            throw FeatureError.message(
                "The report's object-to-track association is not unique at the exact frame. Inspect and place the physical anchor manually."
            )
        }
        guard p.physical, p.reference == "body_centre" else {
            throw FeatureError.message(
                "This estimate locates a \(instant.prediction?.reference ?? "missing point"), not a stated physical body centre. Place the physical anchor manually."
            )
        }
        let digest = reportDigest.dropFirst("sha256:".count)
        guard reportDigest.hasPrefix("sha256:"), digest.count == 64,
            digest.allSatisfy({ $0.isHexDigit }), !p.trackKey.isEmpty,
            let estimator = identity.estimatorID, !estimator.isEmpty, !identity.stage.isEmpty,
            !author.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else {
            throw FeatureError.message(
                "The seed needs a pinned report, named estimator/stage and author.")
        }
        guard p.x.isFinite, p.y.isFinite, p.centreX == nil || p.centreX == p.x,
            p.centreY == nil || p.centreY == p.y
        else {
            throw FeatureError.message(
                "The stated body centre is invalid or contradicts the footprint centre.")
        }
        guard let heading = p.heading, heading.rad.isFinite, let length = p.length,
            let width = p.width, length.metres.isFinite, length.metres > 0, width.metres.isFinite,
            width.metres > 0, p.height == nil || (p.height!.metres.isFinite && p.height!.metres > 0)
        else {
            throw FeatureError.message(
                "A body seed needs finite positive length/width and a finite heading.")
        }
        return Eligible(
            sampleID: sample.sampleID, prediction: p, heading: heading, length: length,
            width: width, estimator: estimator)
    }

    static func make(
        instant: ReportInstant, identity: ReportArmIdentity, reportDigest: String,
        reportPackDigest: String, packDigest: String, objectID: String, sample: AnnotationSample,
        author: String, session: String
    ) throws -> Self {
        let checked = try check(
            instant: instant, identity: identity, reportDigest: reportDigest,
            reportPackDigest: reportPackDigest, packDigest: packDigest, objectID: objectID,
            sample: sample, author: author)
        let p = checked.prediction
        let source =
            identity.source + " · " + reportDigest + " · track " + p.trackKey
            + " · sample \(sample.sampleID) · \(sample.timestampNs) ns"
        var review = PhysicalDraft.review(author: author, session: session)
        review.origin = .trackerAssisted
        review.trackerSource = source
        review.method = "tracker_report_seed_v1"
        review.provenance.operation = "tracker_seed"
        review.provenance.algorithm = checked.estimator
        review.provenance.algorithmVersion = identity.stage
        let support = PhysicalSupport(frames: [sample.sampleID], external: source)
        func dimension(_ extent: ReportPrediction.Extent?) -> PhysicalDimension {
            guard let extent else { return PhysicalDimension() }
            return PhysicalDimension(
                status: .inferred, span: .full, valueM: extent.metres, support: support)
        }
        let body = PhysicalBody(
            bodyID: PhysicalDraft.newID("body"), length: dimension(checked.length),
            width: dimension(checked.width), height: dimension(p.height), review: review)
        let pose = PhysicalKeyframe(
            keyframeID: PhysicalDraft.newID("kf"), sampleID: sample.sampleID,
            timestampNs: sample.timestampNs,
            position: PhysicalPosition(status: .inferred, xM: p.x, yM: p.y, support: support),
            yaw: PhysicalYaw(
                status: .inferred, axis: checked.heading.resolved ? .resolved : .frontRearAmbiguous,
                yawRad: checked.heading.rad, support: support), review: review)
        return Self(
            objectID: objectID, sampleID: sample.sampleID, source: source, prediction: p,
            body: body, keyframe: pose)
    }
}

extension AnnotationSession {
    /// What a seed at this object and frame would be made from: the one
    /// report row, and the context it must agree with.
    private struct TrackerSeedRow {
        var instant: ReportInstant
        var identity: ReportArmIdentity
        var reportDigest: String
        var reportPackDigest: String
        var objectID: String
        var sample: AnnotationSample
    }

    /// One explicitly matched prediction from the report already opened in Compare.
    /// Refuse duplicate object rows rather than guessing which track supplied it.
    private func trackerSeedRow() throws -> TrackerSeedRow {
        guard comparisonAllowed, let objectID = activeObjectID, let sample = currentSample,
            let report = reportInspector.report, let digest = reportInspector.reportDigest,
            let identity = reportInspector.armIdentity
        else {
            throw FeatureError.message(
                "Choose an object and open a tuning report in Compare first.")
        }
        let rows = report.instants(arm: reportInspector.arm, sampleID: sample.sampleID).filter {
            $0.objectID == objectID && $0.prediction != nil
        }
        guard rows.count == 1 else {
            throw FeatureError.message(
                "The report has no unique matched estimate for this object at this frame.")
        }
        return TrackerSeedRow(
            instant: rows[0], identity: identity, reportDigest: digest,
            reportPackDigest: report.reference.packDigest, objectID: objectID, sample: sample)
    }

    /// Whether the report can seed this object and frame, and from what,
    /// without building the seed. For the pane, which asks on every render.
    func trackerSeedEligibility() throws -> PhysicalTrackerSeed.Eligible {
        let row = try trackerSeedRow()
        return try PhysicalTrackerSeed.check(
            instant: row.instant, identity: row.identity, reportDigest: row.reportDigest,
            reportPackDigest: row.reportPackDigest, packDigest: pack.manifest.packDigest,
            objectID: row.objectID, sample: row.sample, author: operatorName)
    }

    func trackerSeedFromReport() throws -> PhysicalTrackerSeed {
        let row = try trackerSeedRow()
        return try PhysicalTrackerSeed.make(
            instant: row.instant, identity: row.identity, reportDigest: row.reportDigest,
            reportPackDigest: row.reportPackDigest, packDigest: pack.manifest.packDigest,
            objectID: row.objectID, sample: row.sample, author: operatorName, session: sessionID)
    }

    func seedPhysicalFromReport() {
        guard workMode == .physical, physical.canEdit, !physical.needsReload else { return }
        guard dirtySamples.isEmpty, !physical.isDirty, !features.isDirty else {
            physical.refuse(
                "Save or discard outstanding point, pose and facet edits before importing a seed.")
            return
        }
        do {
            let seed = try trackerSeedFromReport()
            guard physical.object(seed.objectID)?.keyframe(sampleID: seed.sampleID) == nil else {
                physical.refuse(
                    "This frame already has a pose. Import a seed at an unmarked frame; existing poses are retained."
                )
                return
            }
            physical.expose(objectIDs: [seed.objectID], source: seed.source)
            physical.edit { objects in
                let i = PhysicalDraft.index(of: seed.objectID, in: &objects)
                if objects[i].body == nil { objects[i].body = seed.body }
                objects[i].keyframes.append(seed.keyframe)
            }
            physical.seedGhost = seed
        } catch { physical.refuse(error.localizedDescription) }
    }
}
