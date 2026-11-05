// SiteContextTests.swift
// The city site index, and switching the main view between sites without
// leaking one site's tracks, background or reference into another's
// (lidar-city-view-plan Item 5).

import Foundation
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

/// The repository's shared city-view fixtures.
let cityViewFixtures = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    .appendingPathComponent("testdata/city-view")

func fixtureSiteIndex() throws -> CitySiteIndex {
    try CitySiteIndex.decode(
        Data(contentsOf: cityViewFixtures.appendingPathComponent("sites.json")))
}

struct CitySiteIndexTests {
    @Test func readsTheFixture() throws {
        let index = try fixtureSiteIndex()
        #expect(index.sites.map(\.id) == ["columbus-broadway", "embarcadero-broadway", "soma1"])
        let columbus = try #require(index.site(id: "columbus-broadway"))
        #expect(columbus.title == "Columbus at Broadway")
        #expect(columbus.cells.l16 == "808580f47")
        #expect(columbus.recordedOn == "2026-09-02")
        #expect(columbus.registrationState == .registered)
        #expect(columbus.headingDeg == 143.54)
        #expect(columbus.scenePlacement == "/surveys/columbus-broadway/assets/placement.json")
        #expect(columbus.isPlaced)
        let position = try #require(columbus.sensorCityPosition)
        #expect(position == SIMD3(2947.3276, 4192.5448, 21.2232))
        #expect(columbus.markerCityENU == SIMD3(2947.328, 4192.545, 21.223))
        // Row-major on disk: the first row's first two entries are the
        // sensor's +X and +Y east components, so simd's first column holds
        // the first entries of each row.
        let m = try #require(columbus.sensorToCity)
        #expect(m.columns.0.x == -0.804043494685)
        #expect(m.columns.1.x == 0.593917370073)
        #expect(m.columns.0.y == -0.594570424708)
        #expect(index.site(id: "embarcadero-broadway")?.registrationState == .manuallyPlaced)
    }

    @Test func theHeadingIsTheBearingOfPlusY() throws {
        let m = try #require(try fixtureSiteIndex().site(id: "columbus-broadway")?.sensorToCity)
        // +Y in the city: east, north components of the second column.
        let bearing = atan2(m.columns.1.x, m.columns.1.y) * 180 / .pi
        #expect(abs(SitePose.wrap360(bearing) - 143.54) < 0.05)
        // And the alignment pane's north_azimuth_deg gives it back.
        let columbus = try #require(try fixtureSiteIndex().site(id: "columbus-broadway"))
        let typed = try #require(columbus.northAzimuthDeg)
        #expect(abs(AlignmentSession.hintHeading(northAzimuthDeg: typed) - 143.54) < 1e-9)
    }

    @Test func aSiteWithoutAPoseIsNeverPlaced() throws {
        let unplaced = try #require(try fixtureSiteIndex().site(id: "soma1"))
        #expect(!unplaced.isPlaced)
        #expect(unplaced.sensorToCity == nil)
        #expect(unplaced.sensorCityPosition == nil)
        #expect(unplaced.cells.l16.isEmpty)
        #expect(CitySiteText.row(unplaced) == "SoMa 1 · 2025-12-06 · not yet placed")
    }

    @Test func onlyAPlacedStateWithAWholeMatrixIsPlaced() throws {
        let identity = "[[1,0,0,5],[0,1,0,6],[0,0,1,7],[0,0,0,1]]"
        let json = """
            {"version": 1, "sites": [
              {"id": "a", "placement": {"state": "placed"}},
              {"id": "b", "placement": {"state": "not_placed", "sensor_to_city": \(identity)}},
              {"id": "c", "placement": {"state": "placed",
                "sensor_to_city": [[1,0,0,5],[0,1,0,6],[0,0,1,7],[0,0,1,1]]}},
              {"id": "d", "placement": {"state": "placed", "sensor_to_city": \(identity),
                "marker": {"source": "eye_read", "city_enu_m": [1, 2, 3],
                           "lat_deg": 37.8, "lon_deg": -122.4}}}]}
            """
        let index = try CitySiteIndex.decode(Data(json.utf8))
        #expect(index.sites.map(\.isPlaced) == [false, false, false, true])
        #expect(index.sites[3].sensorCityPosition == SIMD3(5, 6, 7))
        #expect(index.sites[3].markerIsEyeRead)
        #expect(index.sites[3].latitude == 37.8)
        #expect(index.sites[0].title == "a")
    }

    @Test func readsAScenePlacement() throws {
        let url = cityViewFixtures.appendingPathComponent(
            "scenes/columbus-broadway/assets/placement.json")
        let placement = try ScenePlacement.decode(Data(contentsOf: url))
        #expect(placement.siteID == "columbus-broadway")
        #expect(placement.isPlaced)
        #expect(placement.recordID == "gms:01M5GKWEZRV8YEVPSRTA1YGE6Y")
        // sensor-to-city is site-to-city × sensor-to-site, as the sidecar says.
        let composed = try #require(placement.siteToCity) * (try #require(placement.sensorToSite))
        let stated = try #require(placement.sensorToCity)
        for c in 0..<4 { for r in 0..<4 { #expect(abs(composed[c][r] - stated[c][r]) < 1e-3) } }
        // The index carries the same matrix.
        let indexed = try #require(
            try fixtureSiteIndex().site(id: "columbus-broadway")?.sensorToCity)
        #expect(indexed == stated)

        let unplaced = try ScenePlacement.decode(
            Data(#"{"site": "soma1", "state": "not_placed", "reason": "no cell"}"#.utf8))
        #expect(!unplaced.isPlaced && unplaced.sensorToCity == nil && unplaced.reason == "no cell")
    }

    @Test func findsASiteByCaptureAndByCell() throws {
        let index = try fixtureSiteIndex()
        #expect(
            index.site(
                capturePath: "/srv/pcaps/s2/20260902T1320_80858-0f4_columbus-broadway.pcapng")?.id
                == "columbus-broadway")
        #expect(index.site(capturePath: "elsewhere.pcapng") == nil)
        #expect(index.site(l16Token: "8085805e5")?.id == "embarcadero-broadway")
    }

    @Test func nextAndPreviousWrap() throws {
        let index = try fixtureSiteIndex()
        #expect(index.neighbour(of: nil, step: 1)?.id == "columbus-broadway")
        #expect(index.neighbour(of: nil, step: -1)?.id == "soma1")
        #expect(index.neighbour(of: "soma1", step: 1)?.id == "columbus-broadway")
        #expect(index.neighbour(of: "columbus-broadway", step: -1)?.id == "soma1")
    }

    @Test func aLocationNamesTheFileOrItsFolder() {
        let folder = URL(fileURLWithPath: "/Volumes/scenes")
        #expect(CitySiteIndex.indexURL(for: folder).path == "/Volumes/scenes/city/sites.json")
        let file = URL(fileURLWithPath: "/tmp/sites.json")
        #expect(CitySiteIndex.indexURL(for: file) == file)
        #expect(CityViewSession.url(for: " ") == nil)
        #expect(CityViewSession.url(for: "https://example.org/scenes")?.scheme == "https")
        #expect(CityViewSession.url(for: "/a/b")?.isFileURL == true)
    }

    @Test func loadsFromAFile() async throws {
        let index = try await CitySiteIndex.load(
            from: cityViewFixtures.appendingPathComponent("sites.json"))
        #expect(index.sites.count == 3)
    }

    @Test func formatsAnAzimuthForThePane() {
        #expect(AlignmentSession.formatAzimuth(35) == "35")
        #expect(AlignmentSession.formatAzimuth(36.46) == "36.46")
        #expect(AlignmentSession.formatAzimuth(101.78) == "101.78")
        #expect(AlignmentSession.formatAzimuth(12.5) == "12.5")
    }
}

struct SiteSourceResolverTests {
    private let clips = [
        AlignmentSite(
            clipID: "clip-columbus", l16Token: "808580f47", l13Token: "808580f4",
            description: "S2 archive: Columbus at Broadway", sensorID: "hesai-pandar40p",
            latitude: 37.7978, longitude: -122.4065),
        AlignmentSite(
            clipID: "clip-embarcadero", l16Token: "8085805e5", l13Token: "8085805c",
            description: "S2 archive: Embarcadero at Broadway", sensorID: "hesai-pandar40p",
            latitude: 0, longitude: 0),
    ]

    @Test func matchesByCaptureFileThenByCell() throws {
        let index = try fixtureSiteIndex()
        let columbus = try #require(index.site(id: "columbus-broadway"))
        let files = [
            "clip-columbus": [
                "s2/20260902T1300_80858-0f4_columbus-broadway.pcapng",
                "s2/20260902T1320_80858-0f4_columbus-broadway.pcapng",
            ]
        ]
        #expect(
            SiteSourceResolver.clip(for: columbus, clips: clips, filesByClip: files)?.clipID
                == "clip-columbus")
        // Only the captures the index names are replayed, in the clip's order.
        #expect(
            SiteSourceResolver.files(for: columbus, clipFiles: files["clip-columbus"]!) == [
                "s2/20260902T1320_80858-0f4_columbus-broadway.pcapng"
            ])
        let embarcadero = try #require(index.site(id: "embarcadero-broadway"))
        #expect(
            SiteSourceResolver.clip(for: embarcadero, clips: clips, filesByClip: [:])?.clipID
                == "clip-embarcadero")
        // A clip naming none of the index's captures replays all of them.
        #expect(SiteSourceResolver.files(for: embarcadero, clipFiles: ["a", "b"]) == ["a", "b"])
        let unplaced = try #require(index.site(id: "soma1"))
        #expect(SiteSourceResolver.clip(for: unplaced, clips: clips, filesByClip: [:]) == nil)
        // Two clips at one cell and no file to tell them apart: neither.
        var twice = clips
        twice.append(
            AlignmentSite(
                clipID: "clip-embarcadero-2", l16Token: "8085805e5", l13Token: "8085805c",
                description: "S2 archive: Embarcadero again", sensorID: "hesai-pandar40p",
                latitude: 0, longitude: 0))
        #expect(SiteSourceResolver.clip(for: embarcadero, clips: twice, filesByClip: [:]) == nil)
    }

    @Test func theIndexResolvesTheOpenCaptureBeforeItsName() throws {
        let index = try fixtureSiteIndex()
        let evidence = OpenCaptureEvidence(
            clipID: nil, sourcePaths: ["/pcaps/20260902T1522_80858-05c_embarcadero-broadway.pcapng"]
        )
        let found = AlignmentSiteResolver.resolve(
            evidence, sites: clips, filesByCase: [:], index: index)
        #expect(found?.site.clipID == "clip-embarcadero")
        #expect(found?.match == .siteIndex)
    }
}

final class FakeSiteReplayService: SiteReplayService, @unchecked Sendable {
    var clips: [AlignmentSite]
    var files: [String: [String]]
    private(set) var started: [(sensorID: String, files: [String])] = []
    var refuse = false

    init(clips: [AlignmentSite], files: [String: [String]]) {
        self.clips = clips
        self.files = files
    }

    func locatedSites() async throws -> [AlignmentSite] { clips }
    func captureFiles(clipID: String) async throws -> [String] { files[clipID] ?? [] }
    func startReplay(sensorID: String, files: [String]) async throws {
        if refuse { throw SiteSwitchError.replayRefused("busy") }
        started.append((sensorID, files))
    }
}

@MainActor struct SiteSwitchTests {
    private func service() -> FakeSiteReplayService {
        FakeSiteReplayService(
            clips: [
                AlignmentSite(
                    clipID: "clip-columbus", l16Token: "808580f47", l13Token: "808580f4",
                    description: "S2 archive: Columbus at Broadway", sensorID: "hesai-pandar40p",
                    latitude: 37.7978, longitude: -122.4065),
                AlignmentSite(
                    clipID: "clip-west", l16Token: "8085805e5", l13Token: "8085805c",
                    description: "S2 archive: Embarcadero at Broadway", sensorID: "hesai-pandar40p",
                    latitude: 37.7989, longitude: -122.3972),
            ],
            files: [
                "clip-columbus": ["s2/20260902T1320_80858-0f4_columbus-broadway.pcapng"],
                "clip-west": ["s2/20260902T1522_80858-05c_embarcadero-broadway.pcapng"],
            ])
    }

    private func track(_ id: String) -> Track {
        Track(
            trackID: id, sensorID: "hesai-pandar40p", state: .confirmed, hits: 10, misses: 0,
            observationCount: 10, firstSeenNanos: 1_000_000, lastSeenNanos: 2_000_000, speedMps: 8,
            maxSpeedMps: 9, classLabel: "car")
    }

    private func frame(_ id: UInt64, tracks: [Track]) -> FrameBundle {
        var frame = FrameBundle()
        frame.frameID = id
        frame.timestampNanos = Int64(id) * 100_000_000
        frame.tracks = TrackSet(
            frameID: id, timestampNanos: frame.timestampNanos, tracks: tracks, trails: [])
        return frame
    }

    @Test func switchingReplaysTheSitesCapturesAndTakesItsHeading() async throws {
        let index = try fixtureSiteIndex()
        let fake = service()
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        let source = try await state.switchSite(
            to: try #require(index.site(id: "columbus-broadway")), service: fake)
        #expect(
            source
                == .replay(
                    clipID: "clip-columbus", sensorID: "hesai-pandar40p",
                    files: ["s2/20260902T1320_80858-0f4_columbus-broadway.pcapng"]))
        #expect(fake.started.count == 1)
        #expect(fake.started.first?.sensorID == "hesai-pandar40p")
        #expect(state.siteContext.siteID == "columbus-broadway")
        #expect(state.alignment.site?.clipID == "clip-columbus")
        #expect(state.alignment.siteMatch == .siteIndex)
        #expect(state.alignment.northAzimuthText == "36.46")
        #expect(abs((state.alignment.hintHeadingDeg ?? 0) - 143.54) < 1e-9)
    }

    @Test func aRefusedReplayLeavesTheSiteAsItWas() async throws {
        let index = try fixtureSiteIndex()
        let fake = service()
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        try await state.switchSite(
            to: try #require(index.site(id: "columbus-broadway")), service: fake)
        state.onFrameReceived(frame(1, tracks: [track("trk_columbus")]))
        await Task.yield()
        fake.refuse = true
        await #expect(throws: SiteSwitchError.replayRefused("busy")) {
            try await state.switchSite(
                to: try #require(index.site(id: "embarcadero-broadway")), service: fake)
        }
        #expect(state.siteContext.siteID == "columbus-broadway")
        #expect(state.allSeenTracks.keys.sorted() == ["trk_columbus"])
        #expect(state.alignment.citySite?.id == "columbus-broadway")
    }

    @Test func aSiteTheServerCannotPlayIsEnteredButSaysSo() async throws {
        let index = try fixtureSiteIndex()
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        await #expect(throws: SiteSwitchError.nothingToPlay("SoMa 1")) {
            try await state.switchSite(
                to: try #require(index.site(id: "soma1")), service: service())
        }
        #expect(state.siteContext.source == .nothingToPlay)
        #expect(state.allSeenTracks.isEmpty)
    }

    @Test func aNamedLiveServerIsAttachedToInsteadOfReplayed() async throws {
        let defaults = try #require(UserDefaults(suiteName: "SiteSwitchTests.live"))
        defaults.removePersistentDomain(forName: "SiteSwitchTests.live")
        CityLiveServers.set("http://pi-west.local:8080", for: "embarcadero-broadway", in: defaults)
        #expect(
            CityLiveServers.server(for: "embarcadero-broadway", in: defaults)?.streamAddress
                == "pi-west.local:50051")
        let fake = service()
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        let source = try await state.switchSite(
            to: try #require(try fixtureSiteIndex().site(id: "embarcadero-broadway")),
            service: fake, defaults: defaults)
        #expect(source == .live(sensorID: "embarcadero-broadway"))
        #expect(fake.started.isEmpty)
        #expect(state.serverAddress == "pi-west.local:50051")
        CityLiveServers.set(" ", for: "embarcadero-broadway", in: defaults)
        #expect(CityLiveServers.server(for: "embarcadero-broadway", in: defaults) == nil)
        state.disconnect()
    }

    /// Away and back: nothing of either visit survives into the next.
    @Test func switchingAwayAndBackLeaksNoTracks() async throws {
        let index = try fixtureSiteIndex()
        let fake = service()
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        let columbus = try #require(index.site(id: "columbus-broadway"))
        let west = try #require(index.site(id: "embarcadero-broadway"))

        try await state.switchSite(to: columbus, service: fake)
        let firstVisit = state.siteContext
        state.onFrameReceived(frame(1, tracks: [track("trk_columbus")]))
        await Task.yield()
        #expect(state.allSeenTracks.keys.sorted() == ["trk_columbus"])
        #expect(state.trackHistory["trk_columbus"] != nil)

        try await state.switchSite(to: west, service: fake)
        #expect(state.allSeenTracks.isEmpty)
        #expect(state.trackHistory.isEmpty && state.trackMaxSpeed.isEmpty)
        #expect(state.inViewTrackIDs.isEmpty)
        #expect(state.selectedTrackID == nil)
        state.onFrameReceived(frame(2, tracks: [track("trk_west")]))
        await Task.yield()
        #expect(state.allSeenTracks.keys.sorted() == ["trk_west"])

        try await state.switchSite(to: columbus, service: fake)
        #expect(state.siteContext !== firstVisit, "a return visit starts a new context")
        #expect(state.allSeenTracks.isEmpty, "neither the west tracks nor the first visit's")
        // The site being left is emptied, and nothing writes to it again.
        #expect(firstVisit.tracks.isEmpty)
        #expect(state.siteContext.namespaced("trk_1") == "columbus-broadway/trk_1")
        #expect(SiteContext().namespaced("trk_1") == "trk_1")
    }

    @Test func switchingDropsTheRenderersBackgroundAndReference() async throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        guard let renderer = MetalRenderer(metalView: view) else { return }
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        state.registerRenderer(renderer)
        let index = try fixtureSiteIndex()
        let fake = service()
        try await state.switchSite(
            to: try #require(index.site(id: "columbus-broadway")), service: fake)
        #expect(renderer.siteID == "columbus-broadway")

        // A background snapshot, a reference and street marks at the first site.
        var background = FrameBundle()
        background.frameID = 1
        background.frameType = .background
        background.background = BackgroundSnapshot(
            sequenceNumber: 1, timestampNanos: 0, x: [1, 2], y: [1, 2], z: [0, 0],
            confidence: [1, 1])
        renderer.updateFrame(background)
        var g = ReferenceGeometry()
        for message in referenceMessages(.footprints, modelID: "sha256:m") { g.append(message) }
        renderer.setReferenceOverlay(ReferenceStyle.vertices(for: g, centre: SIMD3(0, 0, 0)))
        renderer.setReferenceModelMatrix(simd_float4x4(translation: SIMD3(5, 5, 0)))
        renderer.setStreetOverlay(
            StreetOverlayVertices(fills: [Float](repeating: 1, count: 21), lines: []))
        #expect(renderer.holdsSiteGeometry)

        try await state.switchSite(
            to: try #require(index.site(id: "embarcadero-broadway")), service: fake)
        #expect(!renderer.holdsSiteGeometry)
        #expect(renderer.referenceModelMatrix == matrix_identity_float4x4)
        #expect(renderer.siteID == "embarcadero-broadway")
        state.leaveCitySite()
        #expect(renderer.siteID == "")
        #expect(state.siteContext.site == nil)
    }

    @Test func leavingASiteDropsTheAlignmentReference() async throws {
        let gerardus = FakeAlignmentService()
        let sink = FakeOverlaySink()
        let alignment = AlignmentSession(makeClient: { _ in gerardus }, lookup: nil, defaults: nil)
        alignment.sink = sink
        await alignment.probe()
        let state = AppState(autoConnectOnStartup: false, alignment: alignment, serverDefaults: nil)
        let index = try fixtureSiteIndex()
        try await state.switchSite(
            to: try #require(index.site(id: "columbus-broadway")), service: service())
        await alignment.placeFromHint()
        #expect(alignment.reference != nil)
        #expect(alignment.pose != nil)

        try await state.switchSite(
            to: try #require(index.site(id: "embarcadero-broadway")), service: service())
        #expect(alignment.reference == nil)
        #expect(alignment.pose == nil)
        #expect(alignment.referenceGeometry == nil)
        #expect(alignment.site?.clipID == "clip-west")
        #expect(alignment.citySite?.id == "embarcadero-broadway")
        #expect(alignment.northAzimuthText == "101.78")
        #expect(sink.overlays.last.map { $0 == nil } == true)
        #expect(!state.streets.isPainting)

        // Choosing a site by hand in the pane leaves the city site.
        alignment.choose(
            AlignmentSite(
                clipID: "other", l16Token: "1", l13Token: "1", description: "Other", sensorID: "",
                latitude: 0, longitude: 0))
        #expect(alignment.citySite == nil)
    }

    @Test func theSessionSwitchesOneSiteAtATime() async throws {
        let city = CityViewSession(defaults: nil)
        city.setIndex(try fixtureSiteIndex(), error: nil)
        var asked = [String]()
        city.switcher = { site in
            asked.append(site.id)
            return .nothingToPlay
        }
        var current: String?
        city.currentSiteID = { current }
        await city.step(1)
        current = asked.last
        await city.step(1)
        #expect(asked == ["columbus-broadway", "embarcadero-broadway"])
        city.switcher = { _ in throw SiteSwitchError.replayRefused("busy") }
        await city.step(1)
        #expect(city.switchError?.contains("busy") == true)
        #expect(city.switchingTo == nil)
    }

    @Test func theAnnotationMapMarksLineTakesTheSiteID() throws {
        // mapMarksLine falls back to the site the main view is at.
        #expect(
            SiteContext(site: try fixtureSiteIndex().sites[0]).annotationSiteID
                == "columbus-broadway")
    }
}
