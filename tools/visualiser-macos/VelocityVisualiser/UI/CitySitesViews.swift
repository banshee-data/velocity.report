// CitySitesViews.swift
// The toolbar's site picker and Settings ▸ Servers' City view section
// (lidar-city-view-plan Item 5). The picker lists `city/sites.json`;
// choosing a site, or Previous and Next, switches the main view to it.

import AppKit
import SwiftUI
import UniformTypeIdentifiers

/// The open panel for a city location: a folder, or a JSON file.
@MainActor enum CityLocationPanel {
    static func choose(title: String, message: String, current: String) -> URL? {
        let panel = NSOpenPanel()
        panel.title = title
        panel.message = message
        panel.prompt = "Choose"
        panel.canChooseDirectories = true
        panel.canChooseFiles = true
        panel.allowsMultipleSelection = false
        panel.allowedContentTypes = [.folder, .json]
        if let url = CityLocationSetting.url(for: current), url.isFileURL {
            panel.directoryURL = url.hasDirectoryPath ? url : url.deletingLastPathComponent()
        }
        return panel.runModal() == .OK ? panel.url : nil
    }
}

/// What a site's row says: its title, its date, and "not yet placed" for a
/// site without an accepted pose.
enum CitySiteText {
    static func row(_ site: CitySite) -> String {
        var parts = [site.title]
        if let recorded = site.recordedOn, !recorded.isEmpty { parts.append(recorded) }
        if !site.isPlaced { parts.append("not yet placed") }
        return parts.joined(separator: " · ")
    }
}

@available(macOS 15.0, *) struct SitePickerToolbar: View {
    @ObservedObject var city: CityViewSession
    @EnvironmentObject var appState: AppState

    var body: some View {
        if !city.sites.isEmpty {
            Divider().frame(height: 20)
            HStack(spacing: 4) {
                Button(action: { Task { await city.step(-1) } }) {
                    Image(systemName: "chevron.left")
                }.help("Previous site").disabled(city.switchingTo != nil)
                Menu {
                    ForEach(city.sites) { site in
                        Button(CitySiteText.row(site)) { Task { await city.select(site) } }
                    }
                    if appState.siteContext.site != nil {
                        Divider()
                        Button("No site: show what the server streams") { appState.leaveCitySite() }
                    }
                } label: {
                    Label(menuTitle, systemImage: "mappin.and.ellipse")
                }.frame(maxWidth: 220).help(menuHelp)
                Button(action: { Task { await city.step(1) } }) {
                    Image(systemName: "chevron.right")
                }.help("Next site").disabled(city.switchingTo != nil)
            }
        }
    }

    private var menuTitle: String {
        if let id = city.switchingTo { return "Opening \(city.index?.site(id: id)?.title ?? id)…" }
        return appState.siteContext.site?.title ?? "Sites"
    }

    private var menuHelp: String {
        if let error = city.switchError { return error }
        return "Switch the main view to another site the connected server can play"
    }
}

/// Settings ▸ Servers ▸ City view: where the site index is, and the live
/// server of any site with a Pi in the field.
@available(macOS 15.0, *) struct CitySettingsView: View {
    @ObservedObject var city: CityViewSession
    @State private var locationText = ""
    @State private var tilesText = ""
    @State private var liveTexts: [String: String] = [:]

    var body: some View {
        Section {
            TextField(
                "City tiles", text: $tilesText,
                prompt: Text("From Gerardus; or a folder or URL to use instead")
            ).onSubmit { Task { await city.useTilesLocation(tilesText) } }
            HStack {
                Button("Choose…") {
                    guard
                        let url = CityLocationPanel.choose(
                            title: "City tiles",
                            message:
                                "Choose Gerardus's city-tiles folder or one of its epoch folders.",
                            current: tilesText)
                    else { return }
                    Task { await city.chooseTilesLocation(url) }
                }.help("Choose the tiles folder, so the app may read it")
                Button("Read tiles") { Task { await city.useTilesLocation(tilesText) } }
                Spacer()
                Text(city.tilesStatus).font(.caption).foregroundStyle(
                    city.tilesError == nil ? Color.secondary : Color.red)
            }
            TextField(
                "Site index", text: $locationText,
                prompt: Text("Bundled with the app; or a folder or URL to use instead")
            ).onSubmit { Task { await city.useSitesLocation(locationText) } }
            HStack {
                Button("Choose…") {
                    guard
                        let url = CityLocationPanel.choose(
                            title: "Site index",
                            message:
                                "Choose the folder that holds city/sites.json and the published "
                                + "scenes.", current: locationText)
                    else { return }
                    Task { await city.chooseSitesLocation(url) }
                }.help("Choose the published scenes' folder, so the app may read it").disabled(
                    city.isLoading)
                Button("Read") { Task { await city.useSitesLocation(locationText) } }.disabled(
                    city.isLoading)
                if city.isLoading { ProgressView().controlSize(.small) }
                Spacer()
                Text(status).font(.caption).foregroundStyle(
                    city.loadError == nil ? Color.secondary : Color.red)
            }
            if !city.sites.isEmpty {
                DisclosureGroup("Live servers") {
                    Text(
                        "A site with its own Pi in the field is attached to instead of replayed. "
                            + "Its address stays on this Mac."
                    ).font(.caption).foregroundStyle(.secondary)
                    ForEach(city.sites) { site in
                        TextField(
                            site.title,
                            text: Binding(
                                get: { liveTexts[site.id] ?? city.liveServerText(for: site.id) },
                                set: { liveTexts[site.id] = $0 }),
                            prompt: Text("http://pi.local:8080")
                        ).onSubmit { city.setLiveServer(liveTexts[site.id] ?? "", for: site.id) }
                    }
                }
            }
        } header: {
            Text("City view")
        } footer: {
            Text(
                "Nothing needs setting here. The tiles come from the connected Gerardus server, "
                    + "which serves its newest bake over HTTP on its gRPC port, and the site "
                    + "index is the one published when this app was built. Either field "
                    + "overrides that (\(CityViewSession.tilesLocationKey), "
                    + "\(CityViewSession.sitesLocationKey)): an http(s) URL, or a folder given "
                    + "with Choose…, as the app may read only what it is given. For tiles, an "
                    + "epoch folder, a city-tiles folder or a tileset.json; for the index, the "
                    + "folder holding city/sites.json and the published scenes, which nearby "
                    + "sites play from. Clear a field and press Return to go back to the default."
            ).font(.caption).foregroundStyle(.secondary)
        }.onAppear {
            locationText = city.sitesLocation
            tilesText = city.tilesLocation
        }.onChange(of: city.sitesLocation) { _, next in locationText = next }.onChange(
            of: city.tilesLocation
        ) { _, next in tilesText = next }
    }

    private var status: String {
        if let error = city.loadError { return error }
        if city.index == nil { return "No index read" }
        let placed = city.sites.filter(\.isPlaced).count
        let from = city.usesBundledIndex ? " · bundled with the app" : ""
        return "\(city.sites.count) sites, \(placed) placed\(from)"
    }
}

/// The toolbar's City switch: the camera over the city, with the sites.
/// Shown before the city view is set up too: then it opens Settings ▸
/// Servers, where the tiles and the site index are chosen.
@available(macOS 15.0, *) struct CityModeToolbarButton: View {
    @ObservedObject var city: CityViewSession
    @Environment(\.openSettings) private var openSettings

    static let setUpHelp =
        "Set up the city view: choose the city tiles and the site index in Settings ▸ Servers"

    var body: some View {
        if city.canShowCity {
            Toggle(isOn: Binding(get: { city.isCityMode }, set: { city.setCityMode($0) })) {
                Label("City", systemImage: "building.2")
            }.toggleStyle(.button).help(
                "Show the city: its streets, its buildings and every site with data")
        } else {
            Button {
                SettingsTab.select(.servers)
                openSettings()
            } label: {
                Label("City", systemImage: "building.2")
            }.help(Self.setUpHelp).foregroundStyle(.secondary)
        }
    }
}

/// City mode's markers, street names and line of copy, over the 3D view.
@available(macOS 15.0, *) struct CityOverlayView: View {
    @ObservedObject var city: CityViewSession
    /// The playing site's clock, from the stream's timestamp.
    var clock: String?

    var body: some View {
        if city.isCityMode {
            ZStack(alignment: .topLeading) {
                ForEach(city.overlay.streets) { label in
                    Text(label.name).font(.system(size: label.arterial ? 11 : 9.5, weight: .medium))
                        .foregroundStyle(Color.white.opacity(label.arterial ? 0.85 : 0.65)).shadow(
                            color: .black, radius: 1.5
                        ).fixedSize().rotationEffect(.degrees(label.angleDeg)).position(
                            label.screen
                        ).allowsHitTesting(false)
                }
                ForEach(city.overlay.markers) { marker in
                    Button {
                        guard let site = city.index?.site(id: marker.id) else { return }
                        Task { await city.select(site) }
                    } label: {
                        CityMarkerLabel(
                            marker: marker,
                            clock: marker.focused ? clock : city.siteClocks[marker.id])
                    }.buttonStyle(.plain).fixedSize().position(
                        x: marker.screen.x, y: marker.screen.y - 14
                    ).help(
                        marker.placed
                            ? "Switch to \(marker.title) and fly there"
                            : "Not yet placed: \(marker.title) has no accepted pose")
                }
                VStack {
                    Spacer()
                    Text(SeparationCopy.line).font(.caption).foregroundStyle(.secondary).padding(
                        .horizontal, 10
                    ).padding(.vertical, 5).background(.ultraThinMaterial, in: Capsule()).padding(
                        .bottom, 10)
                }.frame(maxWidth: .infinity).allowsHitTesting(false)
            }
        }
    }
}

struct CityMarkerLabel: View {
    let marker: CityMarker
    let clock: String?

    var body: some View {
        VStack(spacing: 1) {
            Text(marker.title).font(.system(size: 11, weight: marker.focused ? .bold : .semibold))
            Text(subtitle).font(.system(size: 9.5)).foregroundStyle(.secondary)
        }.padding(.horizontal, 6).padding(.vertical, 3).background(
            RoundedRectangle(cornerRadius: 5).fill(
                marker.focused ? Color.accentColor.opacity(0.35) : Color.black.opacity(0.55))
        ).overlay(
            RoundedRectangle(cornerRadius: 5).stroke(
                marker.placed ? Color.white.opacity(0.5) : Color.orange.opacity(0.8),
                style: StrokeStyle(lineWidth: 1, dash: marker.placed ? [] : [3, 2]))
        ).foregroundStyle(.white)
    }

    /// Its own date, and the playing site's own clock: sites recorded on
    /// different days are never shown as one moment.
    private var subtitle: String {
        var parts = [String]()
        if !marker.date.isEmpty { parts.append(marker.date) }
        if let clock { parts.append(clock) }
        if !marker.placed { parts.append("not yet placed") }
        return parts.joined(separator: " · ")
    }
}
