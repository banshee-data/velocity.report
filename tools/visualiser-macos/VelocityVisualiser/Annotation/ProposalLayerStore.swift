// Immutable proposer output, bound to the pack digest. Its objects and masks
// use the sidecar's schema, but remain suggestions even after being reloaded.
import Darwin
import Foundation

struct ProposalLayer: Codable {
    let schemaVersion: Int
    let datasetID: String
    let packDigest: String
    let algorithm: String
    let algorithmVersion: String
    let objects: [AnnotationObject]
    let masks: [FrameMask]
    let descriptors: [Descriptor]

    struct Descriptor: Codable {
        let id: Int
        let kind: String
        let classGuess: String
        let travelled: Float
        let length: Float
        let height: Float

        enum CodingKeys: String, CodingKey {
            case id, kind, travelled, length, height
            case classGuess = "class_guess"
        }
    }

    enum CodingKeys: String, CodingKey {
        case objects, masks, descriptors, algorithm
        case schemaVersion = "schema_version"
        case datasetID = "dataset_id"
        case packDigest = "pack_digest"
        case algorithmVersion = "algorithm_version"
    }
}

struct ProposalLayerStore {
    let pack: AnnotationPack

    private var directory: URL { pack.directory.appendingPathComponent("proposals") }

    var hasLayers: Bool {
        let names = (try? FileManager.default.contentsOfDirectory(atPath: directory.path)) ?? []
        return names.contains { $0.hasPrefix("cluster_chain@1") && $0.hasSuffix(".json") }
            && names.contains { $0.hasPrefix("persistent_voxels@1") && $0.hasSuffix(".json") }
    }

    static func key(for proposal: ObjectProposal) -> String {
        "\(proposal.kind == .fixed ? "persistent_voxels" : "cluster_chain")@1/\(proposal.id)"
    }

    func load() throws -> [ObjectProposal] {
        guard FileManager.default.fileExists(atPath: directory.path) else { return [] }
        let files = try FileManager.default.contentsOfDirectory(
            at: directory, includingPropertiesForKeys: nil
        ).filter { $0.pathExtension == "json" }.sorted {
            $0.lastPathComponent < $1.lastPathComponent
        }
        var proposals: [ObjectProposal] = []
        let sampleIndex = Dictionary(
            uniqueKeysWithValues: pack.chronologicalSamples.enumerated().map {
                ($0.element.sampleID, $0.offset)
            })
        let pointCount = Dictionary(
            uniqueKeysWithValues: pack.samples.map { ($0.sampleID, $0.pointCount) })
        var ids = Set<Int>()
        for file in files {
            let layer = try JSONDecoder().decode(ProposalLayer.self, from: Data(contentsOf: file))
            guard layer.schemaVersion == 1, layer.packDigest == pack.manifest.packDigest,
                layer.datasetID == pack.manifest.datasetID, layer.algorithmVersion == "1",
                layer.algorithm == "cluster_chain" || layer.algorithm == "persistent_voxels",
                layer.objects.allSatisfy({ $0.status == .proposed }),
                layer.masks.allSatisfy({ $0.status == .proposed })
            else {
                throw AnnotationPackError.malformed(
                    "invalid proposal layer \(file.lastPathComponent)")
            }
            let objects = Set(layer.objects.map(\.objectID))
            for descriptor in layer.descriptors {
                guard ids.insert(descriptor.id).inserted else {
                    throw AnnotationPackError.malformed("duplicate proposal id \(descriptor.id)")
                }
                let objectID = "proposal_\(descriptor.id)"
                guard objects.contains(objectID) else {
                    throw AnnotationPackError.malformed("proposal \(descriptor.id) has no object")
                }
                var frames: [Int: [Int]] = [:]
                for mask in layer.masks where mask.objectID == objectID {
                    guard let index = sampleIndex[mask.sampleID],
                        let count = pointCount[mask.sampleID],
                        mask.pointIndices == Array(Set(mask.pointIndices)).sorted(),
                        mask.pointIndices.allSatisfy({ $0 >= 0 && $0 < count })
                    else { throw AnnotationPackError.malformed("invalid proposal mask") }
                    frames[index] = mask.pointIndices
                }
                guard !frames.isEmpty else { throw AnnotationPackError.malformed("empty proposal") }
                proposals.append(
                    ObjectProposal(
                        id: descriptor.id, kind: descriptor.kind == "fixed" ? .fixed : .moving,
                        classGuess: descriptor.classGuess, frames: frames,
                        travelled: descriptor.travelled, length: descriptor.length,
                        height: descriptor.height))
            }
        }
        return proposals
    }

    func save(_ proposals: [ObjectProposal], algorithm: String) throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let timestamp = SidecarStore.utcTimestamp()
        let provenance = Provenance(
            author: algorithm, createdUTC: timestamp, operation: "propose", algorithm: algorithm,
            algorithmVersion: "1")
        var objects: [AnnotationObject] = []
        var masks: [FrameMask] = []
        let samples = pack.chronologicalSamples
        for proposal in proposals {
            let objectID = "proposal_\(proposal.id)"
            objects.append(
                AnnotationObject(
                    objectID: objectID, objectClass: proposal.classGuess, subtype: nil,
                    confidence: 1, status: .proposed, provenance: provenance))
            for (index, indices) in proposal.frames where index < samples.count {
                masks.append(
                    FrameMask(
                        objectID: objectID, sampleID: samples[index].sampleID,
                        pointIndices: Array(Set(indices)).sorted(), completeness: .unreviewed,
                        visibility: .present, status: .proposed, provenance: provenance))
            }
        }
        let layer = ProposalLayer(
            schemaVersion: 1, datasetID: pack.manifest.datasetID,
            packDigest: pack.manifest.packDigest, algorithm: algorithm, algorithmVersion: "1",
            objects: objects, masks: masks,
            descriptors: proposals.map {
                ProposalLayer.Descriptor(
                    id: $0.id, kind: $0.kind == .fixed ? "fixed" : "moving",
                    classGuess: $0.classGuess, travelled: $0.travelled, length: $0.length,
                    height: $0.height)
            })
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(layer)
        let canonical = directory.appendingPathComponent("\(algorithm)@1.json")
        let path =
            FileManager.default.fileExists(atPath: canonical.path)
            ? directory.appendingPathComponent("\(algorithm)@1-\(UUID().uuidString).json")
            : canonical
        let fd = Darwin.open(path.path, O_WRONLY | O_CREAT | O_EXCL, 0o644)
        guard fd >= 0 else {
            throw AnnotationPackError.malformed("could not create proposal layer")
        }
        defer { Darwin.close(fd) }
        try data.withUnsafeBytes { raw in
            guard let base = raw.baseAddress else { return }
            var offset = 0
            while offset < raw.count {
                let written = Darwin.write(fd, base.advanced(by: offset), raw.count - offset)
                guard written > 0 else {
                    throw AnnotationPackError.malformed("could not write proposal layer")
                }
                offset += written
            }
        }
        guard Darwin.fsync(fd) == 0 else {
            throw AnnotationPackError.malformed("could not sync proposal layer")
        }
    }
}
