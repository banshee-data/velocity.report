// ReplayFullFrames.swift
// velocity.report's full-frames mode for a PCAP replay, which Accumulate
// frames switches on while it collects and off again after:
//
//   POST /api/lidar/replay/full_frames   {"enabled": true|false}
//
// Split streaming sends each frame's foreground points only and the
// background as periodic snapshots, which leaves nothing to accumulate. With
// full frames on, every frame carries its whole classified cloud (0
// background, 1 foreground). The server answers 409 unless a PCAP replay is
// running, ends the mode with the replay whatever the client does, and keeps
// its VRLOG recording foreground-only either way.

import Foundation

/// Switches full frames on or off for the PCAP replay that is running.
protocol ReplayFullFramesSwitch: Sendable { func setFullFrames(_ enabled: Bool) async throws }

enum ReplayFullFramesError: Error, Equatable, LocalizedError {
    /// The server would not: no PCAP replay is running (409), or it has no
    /// visualiser stream (501). Carries the server's reason.
    case refused(String)
    /// The server does not have the endpoint: a velocity.report from before it.
    case unsupported
    /// Anything else: the request failed, or the server said something unexpected.
    case failed(String)

    var errorDescription: String? {
        switch self {
        case .refused(let reason): return "velocity.report would not stream full frames: \(reason)"
        case .unsupported:
            return
                "This velocity.report cannot stream full frames (it has no POST /api/lidar/replay/full_frames): update it to accumulate frames from a pcap replay."
        case .failed(let reason):
            return "Could not switch velocity.report to full frames: \(reason)"
        }
    }
}

/// The HTTP client for the switch, against the same server as the other
/// velocity.report clients here.
struct ReplayFullFramesClient: ReplayFullFramesSwitch {
    /// A fixed server, for tests; nil follows Settings ▸ Servers at each call.
    let configuredBaseURL: URL?
    var baseURL: URL { configuredBaseURL ?? VelocityReportServer.currentHTTPURL }
    private let session: URLSession

    init(baseURL: URL? = nil, session: URLSession = APISession.shared) {
        self.configuredBaseURL = baseURL
        self.session = session
    }

    private struct Answer: Decodable {
        var fullFrames: Bool?
        var error: String?
    }

    func setFullFrames(_ enabled: Bool) async throws {
        var request = URLRequest(
            url: baseURL.appendingPathComponent("api/lidar/replay/full_frames"))
        request.httpMethod = "POST"
        request.timeoutInterval = 5
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: ["enabled": enabled])
        let data: Data
        let response: URLResponse
        do { (data, response) = try await session.data(for: request) } catch {
            throw ReplayFullFramesError.failed(error.localizedDescription)
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let answer = try? decoder.decode(Answer.self, from: data)
        switch status {
        case 200..<300:
            // A server without the route can still answer 200 from a catch-all
            // (the web app): only a reply that states the mode counts.
            guard let state = answer?.fullFrames else { throw ReplayFullFramesError.unsupported }
            guard state == enabled else {
                throw ReplayFullFramesError.failed(
                    "it reports full frames \(state ? "on" : "off") after being asked to turn them \(enabled ? "on" : "off")"
                )
            }
        case 409, 501:
            throw ReplayFullFramesError.refused(
                answer?.error ?? HTTPURLResponse.localizedString(forStatusCode: status))
        case 404, 405: throw ReplayFullFramesError.unsupported
        default:
            throw ReplayFullFramesError.failed(
                "HTTP \(status)" + (answer?.error.map { ": \($0)" } ?? ""))
        }
    }
}
