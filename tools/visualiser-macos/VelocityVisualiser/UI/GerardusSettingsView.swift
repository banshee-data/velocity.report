// GerardusSettingsView.swift
// Settings (⌘,) ▸ Servers, the Gerardus sections: where the Gerardus server
// is, whether it answers and what it is, whether it serves registered scans,
// and the street painter's author name.
//
// The address is kept in the app's preferences as `alignment.serviceAddress`
// (GerardusAddress); the local server, 127.0.0.1:50061, is the default.
// Changing it reconnects at once, and the status line says which address is
// in use and whether it answered. The author is kept as `streets.author`,
// which the painter's own Author field shares.

import SwiftUI

/// What the settings show about a server, from GetServerInfo. Kept apart from
/// the view so it can be tested.
enum GerardusSettings {
    /// "0.1.0+3f2a9c1e0b7d", Gerardus's server_version (the crate's version
    /// with its commit as semver build metadata), as its version and git sha.
    /// A version without build metadata has no sha.
    static func version(_ serverVersion: String) -> (version: String, gitSHA: String?) {
        let trimmed = serverVersion.trimmingCharacters(in: .whitespaces)
        guard let plus = trimmed.firstIndex(of: "+") else { return (trimmed, nil) }
        let sha = String(trimmed[trimmed.index(after: plus)...])
        return (String(trimmed[..<plus]), sha.isEmpty ? nil : sha)
    }

    /// Whether the server serves registered scans (`serve --serve-scans`),
    /// as GetServerInfo reports it: it lists REGISTERED_SCANS among its
    /// layers only when it does. Nil when it lists no layers at all, which
    /// says nothing either way.
    static func servesScans(_ info: Gerardus_Align_V1alpha1_GetServerInfoResponse) -> Bool? {
        guard !info.availableLayers.isEmpty else { return nil }
        return info.availableLayers.contains(.registeredScans)
    }

    /// Reads a typed address and, when it parses, points the session at it,
    /// which saves it and asks it at once. The parse error otherwise, and the
    /// session is left as it was.
    @available(macOS 15.0, *) @MainActor static func apply(
        _ text: String, to session: AlignmentSession
    ) -> Result<GerardusAddress, GerardusAddress.ParseError> {
        do {
            let address = try GerardusAddress.parse(text)
            session.useServiceAddress(address)
            return .success(address)
        } catch let error as GerardusAddress.ParseError { return .failure(error) } catch {
            return .failure(.notAnAddress(text))
        }
    }
}

/// The Gerardus sections of Settings ▸ Servers, inside its Form.
@available(macOS 15.0, *) struct GerardusSettingsView: View {
    @ObservedObject var session: AlignmentSession
    @AppStorage(StreetPainterSession.authorKey) private var author = ""
    @State private var text = ""
    @State private var parseError: String?
    @State private var testing = false
    @State private var testResult: ConnectionTestResult?

    var body: some View {
        Section {
            TextField("Server", text: $text, prompt: Text(GerardusAddress.local.description))
                .onSubmit(apply).autocorrectionDisabled()
            Text(
                "A host name or IP address, with an optional port (50061 if left out): machine.tailnet.ts.net, 100.64.0.1:50061, [fd7a:115c:a1e0::1]:50061."
            ).font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("Connect", action: apply)
                Button("Use this Mac") {
                    text = GerardusAddress.local.description
                    apply()
                }.disabled(
                    session.serviceAddress == .local && text == GerardusAddress.local.description)
                Button("Test connection", action: test).disabled(testing).help(
                    "Ask the server for GetServerInfo, with a 3 s timeout")
                Spacer()
            }
            status
            if testing || testResult != nil {
                ConnectionTestRow(title: "Test", result: testResult, testing: testing)
            }
        } header: {
            Text("Gerardus")
        }.onAppear { text = session.serviceAddress.description }
        if let identity { serverSection(identity) }
        Section {
            TextField("Author", text: $author, prompt: Text("Your name"))
            Text(
                "Gerardus records every street-use edit under this name, and refuses one without it. The painter's Author field is the same setting."
            ).font(.caption).foregroundStyle(.secondary)
        } header: {
            Text("Street painter")
        }
    }

    /// What the last test found, else what the session's server said.
    private var identity: ServerIdentity? {
        if case .ok(let identity, _) = testResult { return identity }
        guard let info = session.serverInfo else { return nil }
        let version = GerardusSettings.version(info.serverVersion)
        return ServerIdentity(
            version: version.version.isEmpty ? nil : version.version, gitSHA: version.gitSHA,
            apiVersion: info.apiVersion.isEmpty ? nil : info.apiVersion,
            servesScans: GerardusSettings.servesScans(info))
    }

    @ViewBuilder private func serverSection(_ identity: ServerIdentity) -> some View {
        Section {
            LabeledContent("Version", value: identity.version ?? "—")
            LabeledContent("Git sha", value: identity.gitSHA ?? "not reported").textSelection(
                .enabled)
            if let api = identity.apiVersion { LabeledContent("API", value: api) }
            if let serves = identity.servesScans {
                LabeledContent(
                    "Serves registered scans", value: serves ? "yes (serve --serve-scans)" : "no")
                Text(
                    "Set where Gerardus runs, not here: it serves scans only to loopback and tailnet peers."
                ).font(.caption).foregroundStyle(.secondary)
            }
        } header: {
            Text("This Gerardus")
        }
    }

    @ViewBuilder private var status: some View {
        let address = session.serviceAddress.description
        if let parseError {
            Label(parseError, systemImage: "exclamationmark.triangle").foregroundStyle(.red)
        } else if session.isChecking {
            HStack(spacing: 6) {
                ProgressView().controlSize(.small)
                Text("Asking \(address)…")
            }
        } else {
            switch session.availability {
            case .available(let version):
                Label(
                    "Gerardus \(version) answered at \(address).", systemImage: "checkmark.circle"
                ).foregroundStyle(.green)
            case .unavailable:
                Label(
                    "No answer from \(address)" + (session.probeFailure.map { ": \($0)" } ?? "."),
                    systemImage: "xmark.circle"
                ).foregroundStyle(.orange)
            case .unknown:
                Label(
                    "Not asked yet: the app asks \(address) once it is connected to velocity.report, or press Test connection.",
                    systemImage: "circle.dashed"
                ).foregroundStyle(.secondary)
            }
        }
    }

    private func apply() {
        switch GerardusSettings.apply(text, to: session) {
        case .success(let address):
            parseError = nil
            testResult = nil
            text = address.description
        case .failure(let error): parseError = error.localizedDescription
        }
    }

    private func test() {
        testing = true
        testResult = nil
        Task {
            // What is typed, when it is an address: it can be tried before
            // it is used.
            testResult = await session.testConnection(try? GerardusAddress.parse(text))
            testing = false
        }
    }
}
