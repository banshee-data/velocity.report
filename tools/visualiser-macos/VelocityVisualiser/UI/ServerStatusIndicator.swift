// ServerStatusIndicator.swift
// The top left of the main window: a dot and a word for velocity.report and,
// when there is room, for Gerardus. It reports; it does not toggle anything.
// The connection is the app's to keep up (it connects at launch and retries),
// and File ▸ Reconnect (⌘R) is the manual retry. A click opens Settings ▸
// Servers, where the servers are set and tested.

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
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 12) {
                    item("velocity.report", velocity)
                    item("Gerardus", gerardus)
                }
                item("velocity.report", velocity)
                ServerStatusDot(status: velocity)
            }.contentShape(Rectangle())
        }.buttonStyle(.plain).help(
            Self.tooltip(connection: connection, session: session, gerardus: gerardus)
        ).accessibilityLabel(
            "velocity.report: \(velocity.label). Gerardus: \(gerardus.label). Opens Settings, Servers."
        )
    }

    private func item(_ name: String, _ status: ServerStatus) -> some View {
        HStack(spacing: 5) {
            ServerStatusDot(status: status)
            Text(name).fontWeight(.medium)
            Text(status.label).foregroundStyle(status == .connected ? .secondary : .primary)
        }.font(.callout).lineLimit(1).fixedSize()
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
