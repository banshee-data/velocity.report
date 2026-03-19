// AboutView.swift
// Custom About panel for the VelocityReport macOS application.

import SwiftUI

// MARK: - About View

struct AboutView: View {
    @Environment(\.dismiss) private var dismiss

    private let projectURL = URL(string: "https://velocity.report")!
    private let githubURL = URL(string: "https://github.com/banshee-data/velocity.report")!
    private let licenceURL = URL(string: "https://www.apache.org/licenses/LICENSE-2.0")!
    private var gitSHADisplay: String { String(BuildInfo.gitSHA.prefix(7)) }
    private var githubRevisionURL: URL {
        URL(string: "https://github.com/banshee-data/velocity.report/tree/\(BuildInfo.gitSHA)")!
    }

    private var appVersion: String {
        Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "—"
    }

    private func closeAboutWindow() {
        if let window = NSApp.keyWindow {
            window.performClose(nil)
            return
        }
        dismiss()
    }

    var body: some View {
        VStack(spacing: 16) {
            // App icon + title
            Image(nsImage: NSApp.applicationIconImage).resizable().frame(width: 80, height: 80)

            Text("VelocityVisualiser.app").font(.title).fontWeight(.semibold)

            HStack(spacing: 1) {
                Text("v\(appVersion)  SHA:").font(.caption).foregroundColor(.secondary).help(
                    "Build time: \(BuildInfo.buildTime)")

                Link("\(gitSHADisplay)", destination: githubRevisionURL).font(.caption)
            }

            Divider().padding(.horizontal, 24)

            // Description
            VStack(alignment: .leading, spacing: 10) {
                aboutSection(
                    title: "Civic Engagement Platform",
                    body: "velocity.report is a street safety platform for neighbourhoods. "
                        + "It measures vehicle speeds using affordable radar sensors and turns that data "
                        + "into PDF reports — speed distributions, percentile analysis — "
                        + "the kind of evidence a residents\u{2019} group can put in front of a council and "
                        + "expect to be taken seriously. "
                        + "Designed for streets where people of all ages should feel safe: "
                        + "the eight-year-old on a bicycle, the eighty-year-old crossing to the shops."
                )

                aboutSection(
                    title: "Two Stacks, One Purpose",
                    body:
                        "The production stack pairs a radar sensor with a Go server, SQLite database, "
                        + "and PDF reports — local, offline, and deliberately self-contained. "
                        + "This visualiser belongs to the research stack: a LiDAR-based scene analysis pipeline "
                        + "being built toward unified object tracking and richer spatial understanding. "
                        + "Both stacks serve the same end. The production stack is ready today. "
                        + "The research stack is where today\u{2019}s curiosity becomes tomorrow\u{2019}s capability."
                )

                aboutSection(
                    title: "Privacy by Design",
                    body: "No cameras. No licence plates. No biometric data of any kind. "
                        + "The system records speed measurements, not identities. "
                        + "All data stays on the device that collected it. "
                        + "Communities should not need to build a surveillance infrastructure "
                        + "in order to ask for a safer street.")

                aboutSection(
                    title: "Source & Setup",
                    body: "The full source is on GitHub under an Apache 2.0 licence. "
                        + "Clone the repository, follow the README, and the production stack "
                        + "can be running the same afternoon. "
                        + "The visualiser requires a local server with LiDAR support. "
                        + "Both are documented; neither requires special expertise to deploy, "
                        + "only patience and a willingness to read before clicking.")
            }.padding(.horizontal, 16)

            Divider().padding(.horizontal, 24)

            // Website link
            Link(destination: projectURL) {
                Label("velocity.report", systemImage: "globe").font(.caption)
            }

            // Licence and source buttons
            HStack(spacing: 12) {
                Link(destination: licenceURL) {
                    HStack(spacing: 6) {
                        Image("ASFFeather").resizable().aspectRatio(contentMode: .fit).frame(
                            width: 14, height: 14)
                        Text("Apache License 2.0")
                    }.font(.caption).padding(.horizontal, 10).padding(.vertical, 6).background(
                        .fill.tertiary
                    ).clipShape(RoundedRectangle(cornerRadius: 6))
                }

                Link(destination: githubURL) {
                    HStack(spacing: 6) {
                        Image("GitHubMark").resizable().aspectRatio(contentMode: .fit).frame(
                            width: 14, height: 14)
                        Text("GitHub")
                    }.font(.caption).padding(.horizontal, 10).padding(.vertical, 6).background(
                        .fill.tertiary
                    ).clipShape(RoundedRectangle(cornerRadius: 6))
                }
            }

            Text("© 2025–2026 Banshee, Inc.").font(.caption2).foregroundColor(.secondary).padding(
                .top, 4)
        }.padding(24).frame(width: 420).background {
            Button("Close About Panel") { closeAboutWindow() }.keyboardShortcut(.cancelAction)
                .opacity(0).frame(width: 0, height: 0).allowsHitTesting(false).accessibilityHidden(
                    true)
        }.onExitCommand(perform: closeAboutWindow)
    }

    @ViewBuilder private func aboutSection(title: String, body: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title).font(.caption).fontWeight(.semibold)
            Text(body).font(.caption).foregroundColor(.secondary).fixedSize(
                horizontal: false, vertical: true)
        }
    }
}

#Preview { AboutView() }
