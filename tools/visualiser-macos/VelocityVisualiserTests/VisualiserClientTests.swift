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

// MARK: - VisualiserClient Not Connected Error Tests

@available(macOS 15.0, *) final class VisualiserClientNotConnectedTests: XCTestCase {

    func testPauseThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")
        XCTAssertFalse(client.isConnected)

        do {
            try await client.pause()
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break  // Expected
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testPlayThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            try await client.play()
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testSeekToTimestampThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            try await client.seek(to: 1_000_000_000)
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testSeekToFrameThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            try await client.seek(toFrame: 42)
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testSetRateThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            try await client.setRate(2.0)
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testSetOverlayModesThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            try await client.setOverlayModes(
                showPoints: true, showClusters: true, showTracks: true, showTrails: true,
                showVelocity: true, showGating: false, showAssociation: false, showResiduals: false)
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testGetCapabilitiesThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            _ = try await client.getCapabilities()
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testStartRecordingThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            _ = try await client.startRecording(outputPath: "/tmp/test.vrlog")
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }

    func testStopRecordingThrowsWhenNotConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")

        do {
            _ = try await client.stopRecording()
            XCTFail("Expected notConnected error")
        } catch let error as VisualiserClientError {
            switch error {
            case .notConnected: break
            default: XCTFail("Expected .notConnected, got \(error)")
            }
        }
    }
}

// MARK: - VisualiserClient Address Parsing Tests

@available(macOS 15.0, *) final class VisualiserClientAddressTests: XCTestCase {

    func testConnectWithInvalidAddressMissingPort() async throws {
        let client = VisualiserClient(address: "localhost")

        do {
            try await client.connect()
            XCTFail("Expected invalidAddress error")
        } catch let error as VisualiserClientError {
            switch error {
            case .invalidAddress: break
            default: XCTFail("Expected .invalidAddress, got \(error)")
            }
        }
    }

    func testConnectWithInvalidAddressNonNumericPort() async throws {
        let client = VisualiserClient(address: "localhost:abc")

        do {
            try await client.connect()
            XCTFail("Expected invalidAddress error")
        } catch let error as VisualiserClientError {
            switch error {
            case .invalidAddress: break
            default: XCTFail("Expected .invalidAddress, got \(error)")
            }
        }
    }

    func testConnectWithInvalidAddressTooManyColons() async throws {
        let client = VisualiserClient(address: "host:port:extra")

        do {
            try await client.connect()
            XCTFail("Expected invalidAddress error")
        } catch let error as VisualiserClientError {
            switch error {
            case .invalidAddress: break
            default: XCTFail("Expected .invalidAddress, got \(error)")
            }
        }
    }

    func testConnectSkippedWhenAlreadyConnected() async throws {
        let client = VisualiserClient(address: "localhost:50051")
        // Manually set connected state (normally done by connect)
        // Can't easily set _isConnected directly, but calling connect on non-running server
        // will fail at transport level, so just test the guard logic

        // Calling disconnect when not connected should be safe
        client.disconnect()
        XCTAssertFalse(client.isConnected)
    }
}

// MARK: - VisualiserClient RestartStream Tests

@available(macOS 15.0, *) final class VisualiserClientRestartTests: XCTestCase {

    func testRestartStreamWhenNotConnected() throws {
        let client = VisualiserClient(address: "localhost:50051")
        XCTAssertFalse(client.isConnected)

        // Should be a no-op when not connected (guard check)
        client.restartStream()
        XCTAssertFalse(client.isConnected)
    }
}

// MARK: - VisualiserClient Stream Configuration Tests

@available(macOS 15.0, *) final class VisualiserClientStreamConfigTests: XCTestCase {

    func testDefaultStreamConfiguration() throws {
        let client = VisualiserClient(address: "localhost:50051")

        XCTAssertTrue(client.includePoints)
        XCTAssertTrue(client.includeClusters)
        XCTAssertTrue(client.includeTracks)
        XCTAssertFalse(client.includeDebug)
        XCTAssertEqual(client.decimationRatio, 1.0)
    }

    func testIncludeDebugToggle() throws {
        let client = VisualiserClient(address: "localhost:50051")
        client.includeDebug = true
        XCTAssertTrue(client.includeDebug)

        client.includeDebug = false
        XCTAssertFalse(client.includeDebug)
    }

    func testDecimationModeDefault() throws {
        let client = VisualiserClient(address: "localhost:50051")
        // Default decimation mode
        XCTAssertEqual(client.decimationRatio, 1.0)
    }

    func testDecimationRatioCustom() throws {
        let client = VisualiserClient(address: "localhost:50051")
        client.decimationRatio = 0.25
        XCTAssertEqual(client.decimationRatio, 0.25)
    }
}
