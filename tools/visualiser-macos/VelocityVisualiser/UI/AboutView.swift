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

            Text("VelocityReport.app").font(.title).fontWeight(.semibold)

            HStack(spacing: 1) {
                Text("v\(appVersion)  SHA:").font(.caption).foregroundColor(.secondary).help(
                    "Build time: \(BuildInfo.buildTime)")

                Link("\(gitSHADisplay)", destination: githubRevisionURL).font(.caption)
            }

            Divider().padding(.horizontal, 24)

            // Description
            VStack(alignment: .leading, spacing: 10) {
                aboutSection(
                    title: "Citizen Radar",
                    body:
                        "velocity.report is a citizen radar system that helps communities measure "
                        + "vehicle speeds using affordable, privacy-preserving sensors. No cameras, "
                        + "no licence plates, just velocity measurements that respect privacy.")

                aboutSection(
                    title: "How It Works",
                    body:
                        "This visualiser connects to a velocity.report backend server via gRPC to "
                        + "display real-time LiDAR point clouds, tracked objects, and velocity data. "
                        + "A running server instance on a local machine is required.")

                aboutSection(
                    title: "Reports & Evidence",
                    body:
                        "Collected speed data is used to generate PDF reports with speed distributions, "
                        + "percentile analysis, and hourly heatmaps; evidence that neighbourhood "
                        + "change-makers can present to councils to advocate for safer streets.")
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

            Text("© 2026 Banshee, Inc.").font(.caption2).foregroundColor(.secondary).padding(
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
