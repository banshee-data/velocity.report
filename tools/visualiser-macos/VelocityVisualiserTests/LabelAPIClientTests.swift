//
//  LabelAPIClientTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for LabelAPIClient and APIError.
//

import Foundation
import Testing

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
        label.startFrameID = 100
        label.endFrameID = 200
        label.createdNanos = 1_000_000_000
        label.annotator = "david"
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
                "id": "decoded-id",
                "trackID": "track-002",
                "classLabel": "pedestrian",
                "startFrameID": 50,
                "endFrameID": 150,
                "createdNanos": 2000000000,
                "annotator": "jane",
                "notes": "Walking slowly"
            }
            """
        let data = json.data(using: .utf8)!
        let decoder = JSONDecoder()
        let label = try decoder.decode(LabelEvent.self, from: data)

        #expect(label.id == "decoded-id")
        #expect(label.trackID == "track-002")
        #expect(label.classLabel == "pedestrian")
        #expect(label.startFrameID == 50)
        #expect(label.endFrameID == 150)
        #expect(label.createdNanos == 2_000_000_000)
        #expect(label.annotator == "jane")
        #expect(label.notes == "Walking slowly")
    }

    @Test func labelSetEncodesCorrectly() throws {
        var labelSet = LabelSet()
        labelSet.sessionID = "session-abc"
        labelSet.sourceFile = "test.vrlog"
        labelSet.labels = [
            LabelEvent(
                trackID: "t1", classLabel: "car", startFrameID: 0, endFrameID: 100, createdNanos: 0,
                annotator: "", notes: ""),
            LabelEvent(
                trackID: "t2", classLabel: "truck", startFrameID: 50, endFrameID: 200,
                createdNanos: 0, annotator: "", notes: ""),
        ]

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
                        "id": "l1",
                        "trackID": "t1",
                        "classLabel": "bicycle",
                        "startFrameID": 10,
                        "endFrameID": 20,
                        "createdNanos": 0,
                        "annotator": "",
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
