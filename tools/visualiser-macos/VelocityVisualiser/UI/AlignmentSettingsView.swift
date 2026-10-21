// AlignmentSettingsView.swift
// Settings (⌘,): where the Gerardus alignment server is. The address is kept
// in the app's preferences as `alignment.serviceAddress`; the local server,
// 127.0.0.1:50061, is the default. Changing it reconnects at once, and the
// status line says which address is in use and whether it answered.

import SwiftUI

@available(macOS 15.0, *) struct AlignmentSettingsView: View {
    @ObservedObject var session: AlignmentSession
    @State private var text = ""
    @State private var parseError: String?

    var body: some View {
        Form {
            Section {
                TextField("Server", text: $text, prompt: Text(GerardusAddress.local.description))
                    .onSubmit(apply).autocorrectionDisabled()
                Text(
                    "A host name or IP address, with an optional port (50061 if left out): machine.tailnet.ts.net, 100.64.0.1:50061, [fd7a:115c:a1e0::1]:50061."
                ).font(.caption).foregroundStyle(.secondary)
                HStack {
                    Button("Connect", action: apply).keyboardShortcut(.defaultAction)
                    Button("Use this Mac") {
                        text = GerardusAddress.local.description
                        apply()
                    }.disabled(
                        session.serviceAddress == .local
                            && text == GerardusAddress.local.description)
                    Spacer()
                }
                status
            } header: {
                Text("Gerardus alignment server")
            }
        }.formStyle(.grouped).frame(width: 480).onAppear {
            text = session.serviceAddress.description
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
                    "Not asked yet: the app asks \(address) once it is connected to velocity.report.",
                    systemImage: "circle.dashed"
                ).foregroundStyle(.secondary)
            }
        }
    }

    private func apply() {
        do {
            let address = try GerardusAddress.parse(text)
            parseError = nil
            text = address.description
            session.useServiceAddress(address)
        } catch { parseError = error.localizedDescription }
    }
}
