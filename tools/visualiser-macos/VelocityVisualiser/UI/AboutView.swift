// AboutView.swift
// Custom About panel for the Velocity Visualiser macOS application.

import SwiftUI

// MARK: - About View

struct AboutView: View {
    private let projectURL = URL(string: "https://velocity.report")!
    private let githubURL = URL(string: "https://github.com/banshee-data/velocity.report")!

    private var appVersion: String {
        Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "—"
    }

    private var buildNumber: String {
        Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "—"
    }

    var body: some View {
        VStack(spacing: 16) {
            // App icon + title
            Image(nsImage: NSApp.applicationIconImage).resizable().frame(width: 80, height: 80)

            Text("VelocityVisualiser.app").font(.title).fontWeight(.semibold)

            Text("v\(appVersion) (\(buildNumber))").font(.caption).foregroundColor(.secondary)

            Divider().padding(.horizontal, 24)

            // Description
            VStack(alignment: .leading, spacing: 10) {
                aboutSection(
                    title: "Citizen Radar",
                    body:
                        "velocity.report is a citizen radar system that helps communities measure "
                        + "vehicle speeds using affordable, privacy-preserving sensors. No cameras, "
                        + "no licence plates — just velocity measurements that respect privacy.")

                aboutSection(
                    title: "How It Works",
                    body:
                        "This visualiser connects to a velocity.report backend server via gRPC to "
                        + "display real-time LiDAR point clouds, tracked objects, and velocity data. "
                        + "A running server instance (on a Raspberry Pi or local machine) is required."
                )

                aboutSection(
                    title: "Reports & Evidence",
                    body:
                        "Collected speed data is used to generate PDF reports with speed distributions, "
                        + "percentile analysis, and hourly heatmaps — evidence that neighbourhood "
                        + "change-makers can present to councils to advocate for safer streets.")
            }.padding(.horizontal, 16)

            Divider().padding(.horizontal, 24)

            // Links
            HStack(spacing: 16) {
                Link(destination: projectURL) {
                    Label("velocity.report", systemImage: "globe").font(.caption)
                }
                Link(destination: githubURL) {
                    Label("GitHub", systemImage: "chevron.left.forwardslash.chevron.right").font(
                        .caption)
                }
            }

            Text("© 2025–2026 Banshee Data. All rights reserved.").font(.caption2).foregroundColor(
                .secondary
            ).padding(.top, 4)
        }.padding(24).frame(width: 420)
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
