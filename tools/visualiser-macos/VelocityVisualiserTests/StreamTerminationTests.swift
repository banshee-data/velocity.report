//
//  StreamTerminationTests.swift
//  VelocityVisualiserTests
//
//  A stream that ends is a transport event. Only "replay finished" was
//  signalled for it, so a server restart — which closes the stream this way —
//  left the client showing "connected" against a socket that was gone, with no
//  frames arriving and nothing on screen to say why.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) @MainActor final class StreamTerminationTests: XCTestCase {

    func testStreamFinishClearsTheConnection() async throws {
        let state = AppState()
        state.isConnected = true
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidFinishStream(client)

        XCTAssertFalse(
            state.isConnected,
            "the client still reported connected after its stream ended; no frames can arrive on it")
    }

    /// The finish semantics must survive: a replay reaching its end still
    /// registers as finished, it is only the connection claim that changes.
    func testStreamFinishStillMarksTheReplayFinished() async throws {
        let state = AppState()
        state.isConnected = true
        state.sourceMode = .vrlog
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidFinishStream(client)

        XCTAssertTrue(state.replayFinished, "the replay-finished signal was lost")
    }

    /// An explicit disconnect keeps clearing the flag, as it always did.
    func testExplicitDisconnectClearsTheConnection() async throws {
        let state = AppState()
        state.isConnected = true
        let delegate = ClientDelegateAdapter(appState: state)
        let client = VisualiserClient(address: "localhost:50051")

        delegate.clientDidDisconnect(client, error: nil)

        // clientDidDisconnect hops through a Task; give it a turn to land.
        let deadline = Date().addingTimeInterval(2)
        while state.isConnected && Date() < deadline {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertFalse(state.isConnected)
    }
}
