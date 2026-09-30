//
//  FreezeSplitSheetTests.swift
//  VelocityVisualiserTests
//
//  Freezing from the window: the draft goes through untouched, the preview
//  gates the freeze, a changed draft needs a new preview, the author comes
//  from the session, and the service's refusals are shown as they are.
//

import Foundation
import Testing

@testable import VelocityVisualiser

/// A stand-in for the split endpoints.
final class FakeSplitService: @unchecked Sendable {
    let lock = NSLock()
    var requests: [(path: String, body: [String: Any])] = []
    var wouldFreeze = true
    var existing: [[String: Any]] = []
    var refuse: [String: (Int, String, String)] = [:]

    func handle(_ request: URLRequest) throws -> (HTTPURLResponse, Data) {
        lock.lock()
        defer { lock.unlock() }
        let path = request.url!.path
        var body: [String: Any] = [:]
        if let stream = request.httpBodyStream {
            stream.open()
            var data = Data()
            var buffer = [UInt8](repeating: 0, count: 4096)
            while stream.hasBytesAvailable {
                let n = stream.read(&buffer, maxLength: buffer.count)
                if n <= 0 { break }
                data.append(buffer, count: n)
            }
            stream.close()
            body = (try? JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
        }
        requests.append((path, body))
        func respond(_ status: Int, _ json: Any) throws -> (HTTPURLResponse, Data) {
            (
                HTTPURLResponse(
                    url: request.url!, statusCode: status, httpVersion: nil, headerFields: nil)!,
                try JSONSerialization.data(withJSONObject: json)
            )
        }
        if let (status, code, message) = refuse[path] {
            return try respond(status, ["error": message, "code": code])
        }
        switch path {
        case "/api/annotations/split/preview":
            return try respond(
                200,
                [
                    "packs": [
                        [
                            "pack_digest": "sha256:p", "dataset_id": "ds", "sidecar_revision": 3,
                            "sidecar_sha256": "sha256:s",
                            "objects": [["object_id": "obj_a", "partition": "tune"]],
                            "physical": [
                                "revision": 2, "sha256": "sha256:x", "content_sha256": "sha256:c",
                                "objects": [
                                    [
                                        "object_id": "obj_a", "body": "reviewed",
                                        "body_independent": true, "keyframes": 3,
                                        "reviewed_keyframes": 2, "proposed_keyframes": 1,
                                        "tracker_assisted_keyframes": 0,
                                    ]
                                ], "coverage": ["front": 1],
                            ],
                        ]
                    ], "membership_problems": wouldFreeze ? [] : ["object obj_b is proposed"],
                    "physical_problems": [], "would_freeze": wouldFreeze,
                    "split_digest": "sha256:split",
                ])
        case "/api/annotations/split/freeze":
            existing.append([
                "name": body["output"] ?? "", "revision": 1, "split_digest": "sha256:split",
                "frozen_utc": "t", "author": body["author"] ?? "", "packs": 1,
            ])
            return try respond(
                200,
                [
                    "name": body["output"] ?? "", "path": "/x/splits/\(body["output"] ?? "")",
                    "split_digest": "sha256:split", "revision": 1,
                ])
        case "/api/annotations/splits":
            return try respond(200, ["splits": existing, "count": existing.count])
        default: return try respond(404, ["error": "no route", "code": "not_found"])
        }
    }

    func last(_ path: String) -> [String: Any]? {
        lock.lock()
        defer { lock.unlock() }
        return requests.last { $0.path == path }?.body
    }
}

@MainActor private func makeModel() -> (FreezeSplitModel, FakeSplitService, URL) {
    let (session, baseURL, register) = AnnotationMockURLProtocol.makeSession()
    let fake = FakeSplitService()
    register { try fake.handle($0) }
    let model = FreezeSplitModel(client: SplitFreezeAPIClient(baseURL: baseURL, session: session))
    let draft = FileManager.default.temporaryDirectory.appendingPathComponent(
        "draft-\(UUID().uuidString).json")
    try! Data(
        #"{"schema":"velocity.report/split-draft","schema_version":1,"packs":[{"dir":"run-a/pack"}]}"#
            .utf8
    ).write(to: draft)
    return (model, fake, draft)
}

@MainActor struct FreezeSplitSheetTests {
    @Test func theDraftIsSentUntouchedAndThePreviewGatesTheFreeze() async throws {
        let (model, fake, draft) = makeModel()
        #expect(!model.canFreeze)
        model.chooseDraft(draft)
        #expect(model.output.hasSuffix("-frozen.json") && model.draft != nil)
        #expect(!model.canFreeze, "froze without a preview")
        await model.runPreview()
        let preview = try #require(model.preview)
        #expect(preview.wouldFreeze && preview.splitDigest == "sha256:split")
        #expect(
            preview.packs[0].physical?.revision == 2
                && preview.packs[0].physical?.objects[0].reviewedKeyframes == 2)
        #expect(preview.packs[0].physical?.coverage == ["front": 1])
        let sent = try #require(fake.last("/api/annotations/split/preview"))
        let sentDraft = sent["draft"] as? [String: Any]
        #expect((sentDraft?["packs"] as? [[String: Any]])?.first?["dir"] as? String == "run-a/pack")
        #expect(model.canFreeze)

        #expect(!(await model.freeze(author: "  ")))
        #expect(model.error?.contains("Labelled by") == true)
        #expect(await model.freeze(author: "op"))
        let frozen = try #require(fake.last("/api/annotations/split/freeze"))
        #expect(frozen["author"] as? String == "op" && frozen["output"] as? String == model.output)
        #expect(frozen["supersedes"] == nil)
        #expect(model.result?.revision == 1)
        #expect(model.existing.count == 1)
    }

    @Test func aRefusedPreviewNeverFreezesAndARefusalIsShown() async throws {
        let (model, fake, draft) = makeModel()
        fake.wouldFreeze = false
        model.chooseDraft(draft)
        await model.runPreview()
        #expect(
            model.preview?.wouldFreeze == false
                && model.preview?.membershipProblems == ["object obj_b is proposed"])
        #expect(!model.canFreeze)
        #expect(!(await model.freeze(author: "op")))
        #expect(fake.last("/api/annotations/split/freeze") == nil)

        fake.wouldFreeze = true
        await model.runPreview()
        fake.refuse["/api/annotations/split/freeze"] = (409, "conflict", "split exists")
        #expect(!(await model.freeze(author: "op")))
        #expect(model.error?.contains("split exists") == true)
    }

    @Test func aChangedDraftNeedsANewPreviewAndSupersedesIsSent() async throws {
        let (model, fake, draft) = makeModel()
        model.chooseDraft(draft)
        await model.runPreview()
        #expect(model.canFreeze)
        let other = FileManager.default.temporaryDirectory.appendingPathComponent(
            "draft-\(UUID().uuidString).json")
        try Data(
            #"{"schema":"velocity.report/split-draft","schema_version":1,"packs":[{"dir":"run-b/pack"}]}"#
                .utf8
        ).write(to: other)
        model.chooseDraft(other)
        #expect(!model.canFreeze && model.preview == nil, "a stale preview gated a new draft")
        model.supersedes = "pilot.json"
        await model.runPreview()
        #expect(
            fake.last("/api/annotations/split/preview")?["supersedes"] as? String == "pilot.json")
        #expect(await model.freeze(author: "op"))
        #expect(
            fake.last("/api/annotations/split/freeze")?["supersedes"] as? String == "pilot.json")
    }

    @Test func anUnreadableDraftIsRefusedAndListingsDecode() async throws {
        let (model, fake, _) = makeModel()
        let bad = FileManager.default.temporaryDirectory.appendingPathComponent(
            "bad-\(UUID().uuidString).json")
        try Data("[1,2]".utf8).write(to: bad)
        model.chooseDraft(bad)
        #expect(model.draft == nil && model.error?.contains("draft") == true)
        model.chooseDraft(bad.appendingPathExtension("missing"))
        #expect(model.draft == nil)
        fake.existing = [
            [
                "name": "a.json", "revision": 2, "split_digest": "sha256:a", "frozen_utc": "t",
                "author": "op", "packs": 1,
            ],
            [
                "name": "broken.json", "revision": 0, "split_digest": "", "frozen_utc": "",
                "author": "", "packs": 0, "error": "parse",
            ],
        ]
        await model.loadExisting()
        #expect(
            model.existing.map(\.name) == ["a.json", "broken.json"]
                && model.existing[1].error == "parse")
        fake.refuse["/api/annotations/splits"] = (501, "not_configured", "no dir")
        await model.loadExisting()
        #expect(model.error?.contains("no dir") == true)
    }

    @Test func theWindowOffersFreezeAndTheSheetTakesTheSessionsAuthor() throws {
        let window = try String(
            contentsOf: URL(fileURLWithPath: #filePath).deletingLastPathComponent()
                .deletingLastPathComponent().appendingPathComponent(
                    "VelocityVisualiser/UI/AnnotationWindow.swift"), encoding: .utf8
        ).filter { !$0.isWhitespace }
        #expect(window.contains(#"Button("FreezeSplit…"){showFreezeSheet=true}"#))
        #expect(window.contains("FreezeSplitSheet(session:session)"))
        let sheet = try String(
            contentsOf: URL(fileURLWithPath: #filePath).deletingLastPathComponent()
                .deletingLastPathComponent().appendingPathComponent(
                    "VelocityVisualiser/UI/FreezeSplitSheet.swift"), encoding: .utf8
        ).filter { !$0.isWhitespace }
        #expect(sheet.contains("model.freeze(author:session.operatorName)"))
    }
}
