// ServerSettingsView.swift
// Settings (⌘,) ▸ Servers: the two servers the app talks to.
//
// velocity.report: its HTTP URL and the frame stream's port, kept as
// `velocityReport.serverURL` and `velocityReport.streamPort`. A change is
// applied by Apply or Enter, never per keystroke, and then reconnects once.
// Test connection asks `/api/version` and the stream's GetCapabilities.
//
// Gerardus: as before (GerardusSettingsView), with its own Test connection.

import SwiftUI

/// Settings' tabs, by the name the selection is kept under.
enum SettingsTab: String {
    case servers

    /// UserDefaults key for the selected tab.
    static let key = "settings.selectedTab"

    /// Makes `tab` the one Settings opens on.
    static func select(_ tab: SettingsTab, in defaults: UserDefaults = .standard) {
        defaults.set(tab.rawValue, forKey: key)
    }
}

/// What Apply does with the velocity.report fields. Kept apart from the
/// view so it can be tested.
enum VelocityReportSettings {
    /// Reads the typed URL and stream port and, when they parse, points the
    /// app at them, which saves them and reconnects once if they changed.
    /// The parse error otherwise, and the app is left as it was.
    @available(macOS 15.0, *) @MainActor static func apply(
        url: String, streamPort: String, to appState: AppState
    ) -> Result<VelocityReportServer, VelocityReportServer.ParseError> {
        do {
            let server = try VelocityReportServer.parse(url: url, streamPort: streamPort)
            appState.useVelocityReportServer(server)
            return .success(server)
        } catch let error as VelocityReportServer.ParseError { return .failure(error) } catch {
            return .failure(.notAURL(url))
        }
    }

    /// The stream port field's text: empty for the default.
    static func streamPortText(_ server: VelocityReportServer) -> String {
        server.streamPort == VelocityReportServer.defaultStreamPort ? "" : String(server.streamPort)
    }
}

@available(macOS 15.0, *) struct SettingsView: View {
    /// Not observed: AppState publishes on every frame. The sections observe
    /// the connection and the alignment session instead.
    let appState: AppState
    @AppStorage(SettingsTab.key) private var tab = SettingsTab.servers.rawValue

    var body: some View {
        TabView(selection: $tab) {
            ServerSettingsView(
                appState: appState, connection: appState.connection, session: appState.alignment
            ).tabItem { Label("Servers", systemImage: "server.rack") }.tag(
                SettingsTab.servers.rawValue)
        }.frame(width: 560, height: 720)
    }
}

@available(macOS 15.0, *) struct ServerSettingsView: View {
    let appState: AppState
    @ObservedObject var connection: VelocityReportConnection
    @ObservedObject var session: AlignmentSession

    var body: some View {
        Form {
            VelocityReportSettingsView(appState: appState, connection: connection)
            GerardusSettingsView(session: session)
        }.formStyle(.grouped)
    }
}

@available(macOS 15.0, *) struct VelocityReportSettingsView: View {
    let appState: AppState
    @ObservedObject var connection: VelocityReportConnection
    @State private var urlText = ""
    @State private var portText = ""
    @State private var parseError: String?
    @State private var testing = false
    @State private var report: VelocityReportTestReport?

    var body: some View {
        Section {
            TextField(
                "Server URL", text: $urlText,
                prompt: Text(VelocityReportServer.local.urlDescription)
            ).onSubmit(apply).autocorrectionDisabled()
            TextField(
                "Stream port", text: $portText,
                prompt: Text(String(VelocityReportServer.defaultStreamPort))
            ).onSubmit(apply)
            Text(
                "The HTTP API's URL (http with no port means 8080), and the gRPC frame stream's port on the same host (50051 if left empty). Changes take effect on Apply or Enter, and reconnect."
            ).font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("Apply", action: apply).disabled(!isEdited)
                Button("Use this Mac") {
                    urlText = VelocityReportServer.local.urlDescription
                    portText = ""
                    apply()
                }.disabled(connection.server == .local && !isEdited)
                Button("Test connection", action: test).disabled(testing).help(
                    "Ask /api/version and the stream's GetCapabilities, with a 3 s timeout")
                Spacer()
            }
            if let parseError {
                Label(parseError, systemImage: "exclamationmark.triangle").foregroundStyle(.red)
            }
            LabeledContent("Connection") {
                HStack(spacing: 6) {
                    ServerStatusDot(status: connection.status)
                    Text(connection.status.label)
                }
            }
            LabeledContent("In use", value: connection.server.urlDescription).textSelection(
                .enabled)
            LabeledContent("Stream", value: connection.server.streamAddress).textSelection(.enabled)
            if let lastError = connection.lastError {
                LabeledContent("Last error", value: lastError).foregroundStyle(.secondary)
            }
            if let retryIn = connection.retryIn {
                Text("Retrying in \(ConnectionTestResult.seconds(retryIn)).").font(.caption)
                    .foregroundStyle(.secondary)
            }
        } header: {
            Text("velocity.report")
        }.onAppear(perform: load)
        if testing || report != nil {
            Section {
                ConnectionTestRow(title: "HTTP API", result: report?.api, testing: testing)
                ConnectionTestRow(title: "Stream", result: report?.stream, testing: testing)
                if case .ok(let identity, _) = report?.api {
                    LabeledContent("Version", value: identity.version ?? "—")
                    LabeledContent("Git sha", value: identity.gitSHA ?? "not reported")
                        .textSelection(.enabled)
                    if let built = identity.buildTime { LabeledContent("Built", value: built) }
                }
                if case .ok(let identity, _) = report?.stream, let sensors = identity.sensors,
                    !sensors.isEmpty
                {
                    LabeledContent("Sensors", value: sensors.joined(separator: ", "))
                }
            } header: {
                Text("Test connection")
            }
        }
    }

    /// Whether the fields say something other than the server in use.
    private var isEdited: Bool {
        guard let typed = try? VelocityReportServer.parse(url: urlText, streamPort: portText) else {
            return true
        }
        return typed != connection.server
    }

    private func load() {
        urlText = connection.server.urlDescription
        portText = VelocityReportSettings.streamPortText(connection.server)
    }

    private func apply() {
        switch VelocityReportSettings.apply(url: urlText, streamPort: portText, to: appState) {
        case .success(let server):
            parseError = nil
            urlText = server.urlDescription
            portText = VelocityReportSettings.streamPortText(server)
        case .failure(let error): parseError = error.localizedDescription
        }
    }

    /// Tests what is typed, so a server can be tried before it is used.
    private func test() {
        let server: VelocityReportServer
        do { server = try VelocityReportServer.parse(url: urlText, streamPort: portText) } catch {
            parseError = error.localizedDescription
            return
        }
        parseError = nil
        testing = true
        report = nil
        Task {
            report = await ServerConnectionTest.velocityReport(server)
            testing = false
        }
    }
}

/// One line of a Test connection's result.
struct ConnectionTestRow: View {
    let title: String
    let result: ConnectionTestResult?
    let testing: Bool

    var body: some View {
        LabeledContent(title) {
            if testing && result == nil {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.small)
                    Text("Testing…")
                }
            } else if let result {
                Label(
                    result.summary, systemImage: result.isOK ? "checkmark.circle" : "xmark.circle"
                ).foregroundStyle(result.isOK ? .green : .orange).textSelection(.enabled)
            }
        }
    }
}

/// The coloured dot for a server's status.
struct ServerStatusDot: View {
    let status: ServerStatus

    static func colour(_ status: ServerStatus) -> Color {
        switch status {
        case .connected: return .green
        case .connecting: return .yellow
        case .disconnected: return .gray
        case .error: return .red
        }
    }

    var body: some View { Circle().fill(Self.colour(status)).frame(width: 8, height: 8) }
}
