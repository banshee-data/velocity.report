// CitySitesViews.swift
// The toolbar's site picker and Settings ▸ Servers' City view section
// (lidar-city-view-plan Item 5). The picker lists `city/sites.json`;
// choosing a site, or Previous and Next, switches the main view to it.

import SwiftUI

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
    @State private var liveTexts: [String: String] = [:]

    var body: some View {
        Section {
            TextField(
                "Site index", text: $locationText,
                prompt: Text("Folder, sites.json or URL of the published scenes")
            ).onSubmit { Task { await city.useSitesLocation(locationText) } }
            HStack {
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
                "Kept as \(CityViewSession.sitesLocationKey). The index is city/sites.json beside "
                    + "the published scenes; it names each site's captures, heading and placement."
            ).font(.caption).foregroundStyle(.secondary)
        }.onAppear { locationText = city.sitesLocation }
    }

    private var status: String {
        if let error = city.loadError { return error }
        if city.index == nil { return "No index read" }
        let placed = city.sites.filter(\.isPlaced).count
        return "\(city.sites.count) sites, \(placed) placed"
    }
}
