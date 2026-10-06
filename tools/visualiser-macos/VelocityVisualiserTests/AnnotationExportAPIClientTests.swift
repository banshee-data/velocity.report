//
//  AnnotationExportAPIClientTests.swift
//  VelocityVisualiserTests
//
//  Before this endpoint existed, producing an annotation pack meant finding
//  a run's VRLOG directory and running the CLI export by hand. These tests
//  cover the client that replaces that: the request it sends, and — the part
//  that actually matters for an operator staring at a failed export — that a
//  server error's real message reaches them rather than a bare status code.
//
//  Each test gets its own AnnotationMockURLProtocol session (see that file
//  for why): there is no shared handler for concurrently running tests to
//  race over.
//

import Foundation
import Testing

@testable import VelocityVisualiser

private func makeMockExportClient(
    handler: @escaping (URLRequest) throws -> (HTTPURLResponse, Data)
) -> AnnotationExportAPIClient {
    let (session, baseURL, register) = AnnotationMockURLProtocol.makeSession()
    register(handler)
    return AnnotationExportAPIClient(baseURL: baseURL, session: session)
}

/// Captures what the client actually sent, for assertions after the awaited
/// call returns.
///
/// The mock handler closure runs on whatever thread the URL Loading System
/// chooses to call `startLoading()` from, not on the test's own task, and
/// Swift Testing's `#expect`/`#require` macros are meant to run within a
/// test's task context. Calling them from inside the closure was the actual
/// cause of the three tests below failing deterministically once the
/// cross-test races elsewhere in this file were fixed: recording an issue
/// from off that context does not reliably attach to the running test.
/// Capturing values here and asserting on them back in the test body avoids
/// the question entirely.
///
/// The handler writes from that other thread and the test reads after its
/// `await`, so every access goes through the lock. `@unchecked Sendable` is a
/// promise that the type is made safe by hand; bare `var`s would not keep it.
private final class CapturedRequest: @unchecked Sendable {
    private let lock = NSLock()
    private var storedPath: String?
    private var storedMethod: String?
    private var storedBody: [String: Any] = [:]

    var path: String? {
        get { lock.withLock { storedPath } }
        set { lock.withLock { storedPath = newValue } }
    }

    var method: String? {
        get { lock.withLock { storedMethod } }
        set { lock.withLock { storedMethod = newValue } }
    }

    var body: [String: Any] {
        get { lock.withLock { storedBody } }
        set { lock.withLock { storedBody = newValue } }
    }
}

struct AnnotationExportAPIClientTests {

    @Test func successDecodesThePackDirectoryAndCounts() async throws {
        let captured = CapturedRequest()
        let client = makeMockExportClient { request in
            captured.path = request.url?.path
            captured.method = request.httpMethod
            captured.body = try requestJSONBody(request)

            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            let json = """
                {"pack_dir": "/data/packs/run-123-20260917", "dataset_id": "ds_abc",
                 "sample_count": 20, "point_count": 1377279, "coverage": "full",
                 "frames_without_points": 0, "has_intensity": true, "has_classification": true}
                """
            return (response, Data(json.utf8))
        }

        let result = try await client.exportPack(runID: "run-123", coverage: .full)

        #expect(captured.path == "/api/lidar/runs/run-123/annotation-export")
        #expect(captured.method == "POST")
        #expect(captured.body["coverage"] as? String == "full")
        #expect(result.packDir == "/data/packs/run-123-20260917")
        #expect(result.sampleCount == 20)
        #expect(result.pointCount == 1_377_279)
        #expect(result.hasIntensity)
    }

    @Test func maxSamplesOfZeroIsOmittedRatherThanSentLiterally() async throws {
        let captured = CapturedRequest()
        let client = makeMockExportClient { request in
            captured.body = try requestJSONBody(request)

            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            let json = """
                {"pack_dir": "/x", "dataset_id": "d", "sample_count": 1, "point_count": 1,
                 "coverage": "full", "frames_without_points": 0, "has_intensity": false,
                 "has_classification": false}
                """
            return (response, Data(json.utf8))
        }
        _ = try await client.exportPack(runID: "run-123", coverage: .full)

        // maxSamples: 0 means "use the server's default" and must not be sent
        // as a literal 0, which the server would reject.
        #expect(captured.body["max_samples"] == nil)
        #expect(captured.body["coverage_note"] == nil)
    }

    @Test func explicitMaxSamplesAndNoteAreSent() async throws {
        let captured = CapturedRequest()
        let client = makeMockExportClient { request in
            captured.body = try requestJSONBody(request)

            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            let json = """
                {"pack_dir": "/x", "dataset_id": "d", "sample_count": 50, "point_count": 1,
                 "coverage": "foreground_only", "frames_without_points": 0,
                 "has_intensity": false, "has_classification": false}
                """
            return (response, Data(json.utf8))
        }
        _ = try await client.exportPack(
            runID: "run-123", coverage: .foregroundOnly, coverageNote: "handheld pilot",
            maxSamples: 50)

        #expect(captured.body["max_samples"] as? Int == 50)
        #expect(captured.body["coverage_note"] as? String == "handheld pilot")
    }

    /// The regression this client exists to prevent: a bare "HTTP 400" tells
    /// an operator nothing about why their export failed. The server's
    /// {"error": "..."} body must reach the thrown error's description.
    @Test func serverErrorMessageReachesTheCaller() async throws {
        let client = makeMockExportClient { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 400, httpVersion: nil, headerFields: nil)!
            let json = "{\"error\": \"this run has no VRLOG recording to export from\"}"
            return (response, Data(json.utf8))
        }

        do {
            _ = try await client.exportPack(runID: "run-123", coverage: .full)
            Issue.record("expected exportPack to throw")
        } catch let error as AnnotationExportError {
            let description = error.errorDescription ?? ""
            #expect(description.contains("this run has no VRLOG recording to export from"))
            #expect(description.contains("400"))
        }
    }

    @Test func aNonJSONErrorBodyStillProducesAReadableMessage() async throws {
        // A proxy or misconfigured base URL can return an HTML error page
        // instead of the server's JSON — this must not crash the decoder.
        let client = makeMockExportClient { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 502, httpVersion: nil, headerFields: nil)!
            return (response, Data("<html>Bad Gateway</html>".utf8))
        }
        do {
            _ = try await client.exportPack(runID: "run-123", coverage: .full)
            Issue.record("expected exportPack to throw")
        } catch let error as AnnotationExportError {
            #expect((error.errorDescription ?? "").contains("502"))
        }
    }

    @Test func malformedSuccessBodyIsReportedAsADecodingFailure() async throws {
        let client = makeMockExportClient { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, Data("{\"unexpected\": true}".utf8))
        }
        do {
            _ = try await client.exportPack(runID: "run-123", coverage: .full)
            Issue.record("expected exportPack to throw")
        } catch is AnnotationExportError {
            // Expected: decodingFailed.
        }
    }
}

struct AnnotationExportResultDecodingTests {
    /// AnnotationExportResult declares explicit CodingKeys mapping its
    /// camelCase properties to the wire's snake_case names. Decoding with
    /// `.convertFromSnakeCase` on top of that is a regression that shipped
    /// once already: the strategy converts an incoming "pack_dir" to
    /// "packDir" before matching it against the type's CodingKeys, whose
    /// raw value is still "pack_dir" — so every key looks missing. A plain
    /// JSONDecoder is the only correct pairing with explicit snake_case keys.
    @Test func decodesAgainstThePlainDecoderTheClientActuallyUses() throws {
        let json = """
            {"pack_dir": "/data/packs/x", "dataset_id": "ds_1", "sample_count": 3,
             "point_count": 900, "coverage": "full", "frames_without_points": 0,
             "has_intensity": true, "has_classification": false}
            """
        let result = try JSONDecoder().decode(AnnotationExportResult.self, from: Data(json.utf8))
        #expect(result.packDir == "/data/packs/x")
        #expect(result.datasetID == "ds_1")
        #expect(result.sampleCount == 3)
    }
}

struct AnnotationCoverageTests {
    @Test func rawValuesMatchTheGoExporterConstants() {
        // internal/lidar/annotation.CaptureCoverage: full, foreground_only,
        // decimated. A mismatch here would make the picker send a value the
        // server rejects with "unknown coverage".
        #expect(AnnotationCoverage.full.rawValue == "full")
        #expect(AnnotationCoverage.foregroundOnly.rawValue == "foreground_only")
        #expect(AnnotationCoverage.decimated.rawValue == "decimated")
    }
}

// MARK: - Test helper

/// `httpBody` is nil for a request executed through a custom `URLProtocol` in
/// some Foundation versions, which instead exposes the body through
/// `httpBodyStream`. Reads whichever is set and decodes it as a JSON object.
func requestJSONBody(_ request: URLRequest) throws -> [String: Any] {
    let data: Data
    if let body = request.httpBody {
        data = body
    } else if let stream = request.httpBodyStream {
        stream.open()
        defer { stream.close() }
        var collected = Data()
        let bufferSize = 4096
        var buffer = [UInt8](repeating: 0, count: bufferSize)
        while stream.hasBytesAvailable {
            let read = stream.read(&buffer, maxLength: bufferSize)
            if read <= 0 { break }
            collected.append(buffer, count: read)
        }
        data = collected
    } else {
        data = Data()
    }
    return (try JSONSerialization.jsonObject(with: data) as? [String: Any]) ?? [:]
}
