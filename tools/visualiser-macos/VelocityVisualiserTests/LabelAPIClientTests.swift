//
//  LabelAPIClientTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for LabelAPIClient and APIError.
//

import Foundation
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - LabelAPIClient Initialisation Tests

struct LabelAPIClientInitTests {
    @Test func defaultInitialisation() throws {
        let client = LabelAPIClient()
        // Should use default baseURL
        #expect(!client.sessionID.isEmpty)  // UUID generated
        #expect(client.sourceFile == "")
    }

    @Test func customBaseURL() throws {
        let customURL = URL(string: "http://192.168.1.100:8080")!
        let client = LabelAPIClient(baseURL: customURL)
        #expect(!client.sessionID.isEmpty)
    }

    @Test func sessionIDIsUnique() throws {
        let client1 = LabelAPIClient()
        let client2 = LabelAPIClient()
        #expect(client1.sessionID != client2.sessionID)
    }

    @Test func sourceFileCanBeSet() throws {
        let client = LabelAPIClient()
        client.sourceFile = "recording-2024-01-15.vrlog"
        #expect(client.sourceFile == "recording-2024-01-15.vrlog")
    }

    @Test func sessionIDCanBeSet() throws {
        let client = LabelAPIClient()
        client.sessionID = "custom-session-123"
        #expect(client.sessionID == "custom-session-123")
    }
}

// MARK: - APIError Tests

struct APIErrorTests {
    @Test func requestFailedError() throws {
        let response = HTTPURLResponse(
            url: URL(string: "http://localhost")!, statusCode: 500, httpVersion: nil,
            headerFields: nil)!
        let error = LabelAPIClient.APIError.requestFailed(response)

        // Verify the error contains the response
        switch error {
        case .requestFailed(let resp): #expect((resp as? HTTPURLResponse)?.statusCode == 500)
        case .decodingFailed: Issue.record("Expected requestFailed error")
        }
    }

    @Test func decodingFailedError() throws {
        struct TestError: Error {}
        let error = LabelAPIClient.APIError.decodingFailed(TestError())

        switch error {
        case .requestFailed: Issue.record("Expected decodingFailed error")
        case .decodingFailed:
            // Success - correct error type
            break
        }
    }
}

// MARK: - Label Model Encoding Tests

struct LabelEncodingTests {
    @Test func labelEventEncodesAllFields() throws {
        var label = LabelEvent()
        label.id = "test-id-123"
        label.trackID = "track-001"
        label.classLabel = "car"
        label.startTimestampNs = 100_000_000
        label.endTimestampNs = 200_000_000
        label.createdAtNs = 1_000_000_000
        label.createdBy = "david"
        label.notes = "White sedan heading north"

        let encoder = JSONEncoder()
        let data = try encoder.encode(label)
        let json = String(data: data, encoding: .utf8)!

        #expect(json.contains("test-id-123"))
        #expect(json.contains("track-001"))
        #expect(json.contains("car"))
        #expect(json.contains("david"))
        #expect(json.contains("White sedan heading north"))
    }

    @Test func labelEventDecodesAllFields() throws {
        let json = """
            {
                "label_id": "decoded-id",
                "track_id": "track-002",
                "class_label": "pedestrian",
                "start_timestamp_ns": 50000000,
                "end_timestamp_ns": 150000000,
                "created_at_ns": 2000000000,
                "created_by": "jane",
                "notes": "Walking slowly"
            }
            """
        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let label = try decoder.decode(LabelEvent.self, from: data)

        #expect(label.id == "decoded-id")
        #expect(label.trackID == "track-002")
        #expect(label.classLabel == "pedestrian")
        #expect(label.startTimestampNs == 50_000_000)
        #expect(label.endTimestampNs == 150_000_000)
        #expect(label.createdAtNs == 2_000_000_000)
        #expect(label.createdBy == "jane")
        #expect(label.notes == "Walking slowly")
    }

    @Test func labelSetEncodesCorrectly() throws {
        var labelSet = LabelSet()
        labelSet.sessionID = "session-abc"
        labelSet.sourceFile = "test.vrlog"

        var l1 = LabelEvent()
        l1.trackID = "t1"
        l1.classLabel = "car"
        l1.startTimestampNs = 0
        l1.endTimestampNs = 100_000_000

        var l2 = LabelEvent()
        l2.trackID = "t2"
        l2.classLabel = "truck"
        l2.startTimestampNs = 50_000_000
        l2.endTimestampNs = 200_000_000

        labelSet.labels = [l1, l2]

        let encoder = JSONEncoder()
        let data = try encoder.encode(labelSet)
        let json = String(data: data, encoding: .utf8)!

        #expect(json.contains("session-abc"))
        #expect(json.contains("test.vrlog"))
        #expect(json.contains("car"))
        #expect(json.contains("truck"))
    }

    @Test func labelSetDecodesCorrectly() throws {
        let json = """
            {
                "sessionID": "session-xyz",
                "sourceFile": "data.vrlog",
                "labels": [
                    {
                        "label_id": "l1",
                        "track_id": "t1",
                        "class_label": "bicycle",
                        "start_timestamp_ns": 10000000,
                        "end_timestamp_ns": 20000000,
                        "created_at_ns": 0,
                        "notes": ""
                    }
                ]
            }
            """
        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let labelSet = try decoder.decode(LabelSet.self, from: data)

        #expect(labelSet.sessionID == "session-xyz")
        #expect(labelSet.sourceFile == "data.vrlog")
        #expect(labelSet.labels.count == 1)
        #expect(labelSet.labels[0].classLabel == "bicycle")
    }

    @Test func emptyLabelSetEncodesCorrectly() throws {
        let labelSet = LabelSet()
        let encoder = JSONEncoder()
        let data = try encoder.encode(labelSet)
        let decoder = JSONDecoder()
        let decoded = try decoder.decode(LabelSet.self, from: data)

        #expect(decoded.sessionID == "")
        #expect(decoded.sourceFile == "")
        #expect(decoded.labels.isEmpty)
    }
}

// MARK: - Label Identity Tests

struct LabelIdentityTests {
    @Test func labelEventHasUniqueID() throws {
        let label1 = LabelEvent()
        let label2 = LabelEvent()
        #expect(label1.id != label2.id)
    }

    @Test func labelEventIDIsValidUUID() throws {
        let label = LabelEvent()
        let uuid = UUID(uuidString: label.id)
        #expect(uuid != nil)
    }

    @Test func labelEventConformsToIdentifiable() throws {
        let label = LabelEvent()
        // Identifiable requires an 'id' property
        let _: String = label.id
        #expect(!label.id.isEmpty)
    }
}

// MARK: - LabelAPIClient URL Construction Tests

struct LabelAPIClientURLTests {
    @Test func defaultBaseURL() throws {
        let client = LabelAPIClient()
        // Just verify creation doesn't fail
        #expect(!client.sessionID.isEmpty)
    }

    @Test func customBaseURLPreserved() throws {
        let customURL = URL(string: "https://192.168.1.50:9090")!
        let client = LabelAPIClient(baseURL: customURL)
        #expect(!client.sessionID.isEmpty)
    }

    @Test func localhostIPv4BaseURL() throws {
        let url = URL(string: "http://127.0.0.1:8080")!
        let client = LabelAPIClient(baseURL: url)
        #expect(!client.sessionID.isEmpty)
    }
}

// MARK: - LabelAPIClient Mock HTTP Tests

/// Custom URLProtocol for intercepting and mocking HTTP requests in tests.
class MockURLProtocol: URLProtocol {
    /// Handler closure to produce mock responses. Set before running tests.
    static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let handler = MockURLProtocol.requestHandler else {
            let error = NSError(
                domain: "MockURLProtocol", code: 1,
                userInfo: [
                    NSLocalizedDescriptionKey:
                        "No request handler set — unexpected request: \(request.url?.absoluteString ?? "nil")"
                ])
            client?.urlProtocol(self, didFailWithError: error)
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

/// Helper to create a LabelAPIClient with a mocked URLSession.
func makeMockLabelClient() -> (LabelAPIClient, URLSession) {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [MockURLProtocol.self]
    let session = URLSession(configuration: config)
    let client = LabelAPIClient(baseURL: URL(string: "http://localhost:8080")!, session: session)
    return (client, session)
}

final class LabelAPIClientHTTPTests: XCTestCase {

    func testCreateLabelSuccess() async throws {
        let (client, _) = makeMockLabelClient()

        let responseJSON = """
            {
                "label_id": "new-label-1",
                "track_id": "track-001",
                "class_label": "car",
                "start_timestamp_ns": 1000000000,
                "end_timestamp_ns": 0,
                "created_at_ns": 0,
                "notes": ""
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertTrue(request.url!.path.contains("api/lidar/labels"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let label = try await client.createLabel(
            trackID: "track-001", classLabel: "car", startTimestampNs: 1_000_000_000)
        XCTAssertEqual(label.trackID, "track-001")
        XCTAssertEqual(label.classLabel, "car")
    }

    func testCreateLabelWithOptionalFields() async throws {
        let (client, _) = makeMockLabelClient()
        client.sourceFile = "test.vrlog"

        let responseJSON = """
            {
                "label_id": "new-label-2",
                "track_id": "track-002",
                "class_label": "pedestrian",
                "start_timestamp_ns": 1000000000,
                "end_timestamp_ns": 2000000000,
                "created_at_ns": 0,
                "notes": "Walking"
            }
            """
        MockURLProtocol.requestHandler = { request in
            // Verify optional fields are included in the request body
            if let body = request.httpBody, let json = try? JSONSerialization.jsonObject(with: body)
                as? [String: Any]
            {
                XCTAssertEqual(json["end_timestamp_ns"] as? Int64, 2_000_000_000)
                XCTAssertEqual(json["confidence"] as? Double, 0.95, accuracy: 0.01)
                XCTAssertEqual(json["source_file"] as? String, "test.vrlog")
            }
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 201, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let label = try await client.createLabel(
            trackID: "track-002", classLabel: "pedestrian", startTimestampNs: 1_000_000_000,
            endTimestampNs: 2_000_000_000, confidence: 0.95)
        XCTAssertEqual(label.classLabel, "pedestrian")
    }

    func testCreateLabelServerError() async throws {
        let (client, _) = makeMockLabelClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.createLabel(
                trackID: "track-001", classLabel: "car", startTimestampNs: 0)
            XCTFail("Expected requestFailed error")
        } catch is LabelAPIClient.APIError {
            // Expected
        }
    }

    func testGetLabelsForSession() async throws {
        let (client, _) = makeMockLabelClient()
        client.sessionID = "test-session"

        let responseJSON = """
            [{
                "label_id": "l1",
                "track_id": "track-001",
                "class_label": "car",
                "start_timestamp_ns": 0,
                "end_timestamp_ns": 0,
                "created_at_ns": 0,
                "notes": ""
            }]
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.absoluteString.contains("session_id=test-session"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let labels = try await client.getLabelsForSession()
        XCTAssertEqual(labels.count, 1)
        XCTAssertEqual(labels[0].classLabel, "car")
    }

    func testGetLabelsForTrack() async throws {
        let (client, _) = makeMockLabelClient()

        let responseJSON = """
            [{
                "label_id": "l1",
                "track_id": "track-abc",
                "class_label": "pedestrian",
                "start_timestamp_ns": 0,
                "end_timestamp_ns": 0,
                "created_at_ns": 0,
                "notes": ""
            }]
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.absoluteString.contains("track_id=track-abc"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let labels = try await client.getLabelsForTrack("track-abc")
        XCTAssertEqual(labels.count, 1)
        XCTAssertEqual(labels[0].trackID, "track-abc")
    }

    func testUpdateLabel() async throws {
        let (client, _) = makeMockLabelClient()

        var label = LabelEvent()
        label.id = "label-to-update"
        label.trackID = "track-001"
        label.classLabel = "truck"

        let responseJSON = """
            {
                "label_id": "label-to-update",
                "track_id": "track-001",
                "class_label": "truck",
                "start_timestamp_ns": 0,
                "end_timestamp_ns": 0,
                "created_at_ns": 0,
                "notes": ""
            }
            """
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "PUT")
            XCTAssertTrue(request.url!.path.contains("label-to-update"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, responseJSON.data(using: .utf8)!)
        }

        let updated = try await client.updateLabel(label)
        XCTAssertEqual(updated.classLabel, "truck")
    }

    func testDeleteLabelSuccess() async throws {
        let (client, _) = makeMockLabelClient()

        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "DELETE")
            XCTAssertTrue(request.url!.path.contains("label-123"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        try await client.deleteLabel("label-123")
    }

    func testDeleteLabelServerError() async throws {
        let (client, _) = makeMockLabelClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 404, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            try await client.deleteLabel("nonexistent")
            XCTFail("Expected requestFailed error")
        } catch is LabelAPIClient.APIError {
            // Expected
        }
    }

    func testExportLabels() async throws {
        let (client, _) = makeMockLabelClient()
        client.sessionID = "export-session"

        let exportData = """
            {"labels": [{"track_id": "t1", "class_label": "car"}]}
            """.data(using: .utf8)!

        MockURLProtocol.requestHandler = { request in
            XCTAssertTrue(request.url!.path.contains("export"))
            XCTAssertTrue(request.url!.absoluteString.contains("session_id=export-session"))
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, exportData)
        }

        let data = try await client.exportLabels()
        XCTAssertGreaterThan(data.count, 0)
    }

    func testExportToFile() async throws {
        let (client, _) = makeMockLabelClient()

        let exportData = """
            {"labels": []}
            """.data(using: .utf8)!

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, exportData)
        }

        let tempURL = FileManager.default.temporaryDirectory.appendingPathComponent(
            "test-export-\(UUID().uuidString).json")
        try await client.exportToFile(tempURL)

        let fileData = try Data(contentsOf: tempURL)
        XCTAssertGreaterThan(fileData.count, 0)

        // Clean up
        try? FileManager.default.removeItem(at: tempURL)
    }

    func testGetLabelsForSessionServerError() async throws {
        let (client, _) = makeMockLabelClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 503, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.getLabelsForSession()
            XCTFail("Expected requestFailed error")
        } catch is LabelAPIClient.APIError {
            // Expected
        }
    }

    func testUpdateLabelServerError() async throws {
        let (client, _) = makeMockLabelClient()

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 400, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        var label = LabelEvent()
        label.id = "bad-label"
        do {
            _ = try await client.updateLabel(label)
            XCTFail("Expected requestFailed error")
        } catch is LabelAPIClient.APIError {
            // Expected
        }
    }
}
