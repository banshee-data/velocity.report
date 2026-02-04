//
//  VisualiserClientTests.swift
//  VelocityVisualiserTests
//
//  Unit tests for VisualiserClient, supporting types, and error handling.
//

import Foundation
import Testing
import XCTest

@testable import VelocityVisualiser

// MARK: - VisualiserClientError Tests

struct VisualiserClientErrorTests {
    @Test func notConnectedErrorDescription() throws {
        let error = VisualiserClientError.notConnected
        #expect(error.errorDescription == "Not connected to server")
    }

    @Test func connectionFailedErrorDescription() throws {
        let error = VisualiserClientError.connectionFailed("timeout")
        #expect(error.errorDescription == "Connection failed: timeout")
    }

    @Test func streamErrorErrorDescription() throws {
        let error = VisualiserClientError.streamError("stream closed")
        #expect(error.errorDescription == "Stream error: stream closed")
    }

    @Test func invalidAddressErrorDescription() throws {
        let error = VisualiserClientError.invalidAddress("bad format")
        #expect(error.errorDescription == "Invalid address: bad format")
    }

    @Test func errorConformsToLocalizedError() throws {
        let error: Error = VisualiserClientError.notConnected
        #expect(error.localizedDescription == "Not connected to server")
    }
}

// MARK: - LockedState Tests

struct LockedStateTests {
    @Test func initialValue() throws {
        let state = LockedState(42)
        #expect(state.value == 42)
    }

    @Test func setValue() throws {
        let state = LockedState(0)
        state.value = 100
        #expect(state.value == 100)
    }

    @Test func stringValue() throws {
        let state = LockedState("initial")
        #expect(state.value == "initial")
        state.value = "updated"
        #expect(state.value == "updated")
    }

    @Test func optionalValue() throws {
        let state = LockedState<Int?>(nil)
        #expect(state.value == nil)
        state.value = 42
        #expect(state.value == 42)
        state.value = nil
        #expect(state.value == nil)
    }

    @Test func booleanValue() throws {
        let state = LockedState(false)
        #expect(state.value == false)
        state.value = true
        #expect(state.value == true)
    }
}

// MARK: - ServerCapabilities Tests

struct ServerCapabilitiesTests {
    @Test func defaultInitialisation() throws {
        let caps = ServerCapabilities()
        #expect(caps.supportsPoints == false)
        #expect(caps.supportsClusters == false)
        #expect(caps.supportsTracks == false)
        #expect(caps.supportsDebug == false)
        #expect(caps.supportsReplay == false)
        #expect(caps.supportsRecording == false)
        #expect(caps.availableSensors.isEmpty)
    }

    @Test func customInitialisation() throws {
        let caps = ServerCapabilities(
            supportsPoints: true, supportsClusters: true, supportsTracks: true,
            supportsDebug: false, supportsReplay: true, supportsRecording: true,
            availableSensors: ["hesai-01", "hesai-02"])
        #expect(caps.supportsPoints == true)
        #expect(caps.supportsClusters == true)
        #expect(caps.supportsTracks == true)
        #expect(caps.supportsDebug == false)
        #expect(caps.supportsReplay == true)
        #expect(caps.supportsRecording == true)
        #expect(caps.availableSensors.count == 2)
        #expect(caps.availableSensors[0] == "hesai-01")
    }
}

// MARK: - RecordingStatus Tests

struct RecordingStatusTests {
    @Test func defaultInitialisation() throws {
        let status = RecordingStatus()
        #expect(status.recording == false)
        #expect(status.outputPath == "")
        #expect(status.framesRecorded == 0)
    }

    @Test func activeRecording() throws {
        let status = RecordingStatus(
            recording: true, outputPath: "/var/lib/velocity-report/recordings/2024-01-15.vrlog",
            framesRecorded: 5000)
        #expect(status.recording == true)
        #expect(status.outputPath.contains("2024-01-15.vrlog"))
        #expect(status.framesRecorded == 5000)
    }
}

// MARK: - VisualiserClient Tests (XCTest for @available compatibility)

@available(macOS 15.0, *) final class VisualiserClientInitTests: XCTestCase {

    func testInitialisation() throws {
        let client = VisualiserClient(address: "localhost:50051")
        XCTAssertEqual(client.address, "localhost:50051")
        XCTAssertFalse(client.isConnected)
        XCTAssertTrue(client.includePoints)
        XCTAssertTrue(client.includeClusters)
        XCTAssertTrue(client.includeTracks)
        XCTAssertFalse(client.includeDebug)
        XCTAssertEqual(client.decimationRatio, 1.0)
    }

    func testCustomAddress() throws {
        let client = VisualiserClient(address: "192.168.1.100:9999")
        XCTAssertEqual(client.address, "192.168.1.100:9999")
    }

    func testConfigurationOptions() throws {
        let client = VisualiserClient(address: "localhost:50051")
        client.includePoints = false
        client.includeClusters = false
        client.includeTracks = false
        client.includeDebug = true
        client.decimationRatio = 0.5

        XCTAssertFalse(client.includePoints)
        XCTAssertFalse(client.includeClusters)
        XCTAssertFalse(client.includeTracks)
        XCTAssertTrue(client.includeDebug)
        XCTAssertEqual(client.decimationRatio, 0.5)
    }

    func testDisconnectWhenNotConnected() throws {
        let client = VisualiserClient(address: "localhost:50051")
        // Should not crash when disconnecting while not connected
        client.disconnect()
        XCTAssertFalse(client.isConnected)
    }
}

// MARK: - VisualiserClientDelegate Protocol Tests

@available(macOS 15.0, *) final class MockClientDelegate: VisualiserClientDelegate {
    var didConnect = false
    var didDisconnect = false
    var disconnectError: Error?
    var receivedFrames: [FrameBundle] = []
    var didFinishStream = false

    func clientDidConnect(_ client: VisualiserClient) { didConnect = true }

    func clientDidDisconnect(_ client: VisualiserClient, error: Error?) {
        didDisconnect = true
        disconnectError = error
    }

    func client(_ client: VisualiserClient, didReceiveFrame frame: FrameBundle) {
        receivedFrames.append(frame)
    }

    func clientDidFinishStream(_ client: VisualiserClient) { didFinishStream = true }
}

@available(macOS 15.0, *) final class VisualiserClientDelegateTests: XCTestCase {

    func testDelegateCanBeSet() throws {
        let client = VisualiserClient(address: "localhost:50051")
        let delegate = MockClientDelegate()
        client.delegate = delegate
        // Weak reference, just verify no crash
        XCTAssertNotNil(client.delegate)
    }

    func testMockDelegateInitialState() throws {
        let delegate = MockClientDelegate()
        XCTAssertFalse(delegate.didConnect)
        XCTAssertFalse(delegate.didDisconnect)
        XCTAssertNil(delegate.disconnectError)
        XCTAssertTrue(delegate.receivedFrames.isEmpty)
        XCTAssertFalse(delegate.didFinishStream)
    }
}
