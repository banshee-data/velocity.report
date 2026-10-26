// ServerStatusIndicator.swift
// Inlaid in the top left of the 3D view: a dot and a name for
// velocity.report and, under it, for Gerardus. A healthy server is its dot
// and its name alone; one in error adds the salient part of what went
// wrong. It reports; it does not toggle anything. The connection is the
// app's to keep up (it connects at launch and retries), and File ▸
// Reconnect (⌘R) is the manual retry. A click opens Settings ▸ Servers,
// where the servers are set and tested, and the tooltip has the rest.

import SwiftUI

@available(macOS 15.0, *) struct ServerStatusIndicator: View {
    @ObservedObject var connection: VelocityReportConnection
    @ObservedObject var session: AlignmentSession
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        let velocity = connection.status
        let gerardus = Self.gerardusStatus(session)
        Button {
            SettingsTab.select(.servers)
            openSettings()
        } label: {
            VStack(alignment: .leading, spacing: 4) {
                row("velocity.report", velocity, lastError: connection.lastError)
                row("Gerardus", gerardus, lastError: session.probeFailure)
            }.padding(.horizontal, 8).padding(.vertical, 6).background(.black.opacity(0.55))
                .clipShape(RoundedRectangle(cornerRadius: 6)).contentShape(
                    RoundedRectangle(cornerRadius: 6))
        }.buttonStyle(.plain).help(
            Self.tooltip(connection: connection, session: session, gerardus: gerardus)
        ).accessibilityLabel(
            "velocity.report: \(velocity.label). Gerardus: \(gerardus.label). Opens Settings, Servers."
        )
    }

    private func row(_ name: String, _ status: ServerStatus, lastError: String?) -> some View {
        HStack(spacing: 6) {
            ServerStatusDot(status: status)
            Text(name).fontWeight(.medium).foregroundStyle(.white.opacity(0.9))
            if let reason = Self.frameReason(status, lastError: lastError) {
                Text(reason).foregroundStyle(Color(nsColor: .systemRed).opacity(0.95))
            }
        }.font(.caption).lineLimit(1).fixedSize()
    }

    /// What the row says after the name: nothing, unless the server is in
    /// error, and then the salient part of the full error when there is
    /// one, else of the status's own reason.
    static func frameReason(_ status: ServerStatus, lastError: String?) -> String? {
        guard let reason = status.errorReason else { return nil }
        if let lastError, !lastError.isEmpty { return ServerStatus.salientReason(lastError) }
        return ServerStatus.salientReason(reason)
    }

    static func gerardusStatus(_ session: AlignmentSession) -> ServerStatus {
        ServerStatus.gerardus(
            availability: session.availability, isChecking: session.isChecking,
            failure: session.probeFailure)
    }

    /// Where each server is and what last went wrong with it.
    static func tooltip(
        connection: VelocityReportConnection, session: AlignmentSession, gerardus: ServerStatus
    ) -> String {
        var lines = [connection.tooltip, ""]
        lines.append("Gerardus \(session.serviceAddress.description)")
        lines.append(gerardus.label)
        if let failure = session.probeFailure { lines.append("Last error: \(failure)") }
        lines += ["", "Click for Settings ▸ Servers."]
        return lines.joined(separator: "\n")
    }
}
