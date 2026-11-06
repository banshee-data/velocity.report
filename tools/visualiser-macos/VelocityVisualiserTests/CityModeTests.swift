// CityModeTests.swift
// City mode (lidar-city-view-plan Item 7): the flight, the markers and
// street names, the stream placed by its sensor-to-city, and the
// separation cues that keep each site to itself.

import Foundation
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

struct SeparationCueTests {
    @Test func boxesFadeAtTheCoverageEdgeAndVanishBeyondIt() {
        let coverage = SiteCoverage(radius: 100)
        #expect(coverage.opacity(atDistance: 10) == 1)
        #expect(coverage.opacity(atDistance: 85) == 1)
        #expect(abs(coverage.opacity(atDistance: 92.5) - 0.5) < 1e-6)
        #expect(coverage.opacity(atDistance: 100) == 0)
        #expect(coverage.opacity(atDistance: 300) == 0)
    }

    @Test func theRingSitsOnTheEdge() {
        let ring = SiteCoverage(radius: 50).ring(groundZ: -2.3, segments: 8)
        #expect(ring.count == 8 * 2 * 7)
        for v in stride(from: 0, to: ring.count, by: 7) {
            #expect(abs(hypot(ring[v], ring[v + 1]) - 50) < 1e-3)
            #expect(ring[v + 2] == -2.3)
        }
    }

    @Test func colourIsByClassNeverByTrack() {
        #expect(ClassColour.colour(for: "car") == ClassColour.colour(for: "truck"))
        #expect(ClassColour.colour(for: "pedestrian") != ClassColour.colour(for: "car"))
        #expect(ClassColour.colour(for: "bicycle") == ClassColour.colour(for: "cyclist"))
        #expect(ClassColour.colour(for: "") == ClassColour.colour(for: "noise"))
    }

    @Test func theCopySaysVehiclesAreNotFollowed() {
        #expect(
            SeparationCopy.line
                == "Each intersection is recorded on its own. Vehicles are not followed from one intersection to the next."
        )
    }

    @Test func eachSiteKeepsItsOwnClock() {
        // 2026-09-02 20:20:00 UTC is 13:20 in San Francisco.
        #expect(CityOverlayLayout.clock(timestampNanos: 1_788_380_400_000_000_000) == "13:20:00")
        #expect(CityOverlayLayout.clock(timestampNanos: 0) == nil)
    }
}

struct CityFlightTests {
    private let from = SIMD3<Double>(2947, 4192, 21)
    private let to = SIMD3<Double>(3733, 4336, 2.5)

    @Test func startsWhereTheCameraIsAndEndsOverTheSite() {
        let flight = CityFlight(
            fromTarget: from, fromEye: from + SIMD3(0, -30, 20), toTarget: to,
            toEyeOffset: CityFlight.arrivalOffset)
        let start = flight.at(0)
        #expect(start.target == from)
        #expect(simd_length(start.eye - (from + SIMD3(0, -30, 20))) < 1e-9)
        let end = flight.at(1)
        #expect(end.target == to)
        #expect(simd_length(end.eye - (to + CityFlight.arrivalOffset)) < 1e-6)
        #expect(flight.at(2) == end)
    }

    @Test func risesInTheMiddle() {
        let flight = CityFlight(
            fromTarget: from, fromEye: from + CityFlight.arrivalOffset, toTarget: to,
            toEyeOffset: CityFlight.arrivalOffset)
        let middle = flight.at(0.5)
        let ends = max(flight.at(0).eye.z, flight.at(1).eye.z)
        #expect(middle.eye.z > ends + 300)
        #expect(flight.duration > 1.2 && flight.duration <= 4)
    }
}

struct CityOverlayLayoutTests {
    @Test func projectsOntoTheViewTopLeft() {
        var camera = Camera()
        camera.position = SIMD3(0, 0, 100)
        camera.target = .zero
        camera.up = SIMD3(0, 1, 0)
        let size = CGSize(width: 800, height: 600)
        camera.aspectRatio = 800 / 600
        let vp = camera.projectionMatrix * camera.viewMatrix
        let centre = CityOverlayLayout.project(.zero, viewProjection: vp, size: size)
        #expect(abs(centre!.x - 400) < 0.01 && abs(centre!.y - 300) < 0.01)
        // North is up the screen: a smaller y.
        let north = CityOverlayLayout.project(SIMD3(0, 10, 0), viewProjection: vp, size: size)
        #expect(north!.y < 300)
        #expect(CityOverlayLayout.project(SIMD3(0, 0, 200), viewProjection: vp, size: size) == nil)
    }

    @Test func keepsLabelsUpright() {
        #expect(CityOverlayLayout.angle(from: .zero, to: CGPoint(x: -10, y: 0)) == 0)
        #expect(abs(CityOverlayLayout.angle(from: .zero, to: CGPoint(x: -10, y: -10)) - 45) < 1e-9)
        #expect(abs(CityOverlayLayout.angle(from: .zero, to: CGPoint(x: 0, y: 10)) - 90) < 1e-9)
    }

    @Test func thinsOverlappingLabelsArterialsFirst() {
        func label(
            _ id: String, _ name: String, _ x: CGFloat, arterial: Bool = false
        ) -> ScreenStreetLabel {
            ScreenStreetLabel(
                id: id, name: name, screen: CGPoint(x: x, y: 100), angleDeg: 0, arterial: arterial)
        }
        let kept = CityOverlayLayout.thin([
            label("a", "GREEN ST", 100), label("b", "COLUMBUS AVE", 110, arterial: true),
            label("c", "VALLEJO ST", 600), label("d", "VALLEJO ST", 700),
        ])
        #expect(kept.map(\.id) == ["b", "c"])
    }
}

@MainActor struct CityModeRendererTests {
    private func renderer() throws -> MetalRenderer? {
        guard let device = MTLCreateSystemDefaultDevice() else { return nil }
        let view = MTKView(frame: CGRect(x: 0, y: 0, width: 800, height: 600))
        view.device = device
        return MetalRenderer(metalView: view)
    }

    private func track(_ id: String, x: Float, y: Float, label: String = "car") -> Track {
        var t = Track(
            trackID: id, sensorID: "s", state: .confirmed, hits: 10, misses: 0,
            observationCount: 10, firstSeenNanos: 1, lastSeenNanos: 2, speedMps: 8, maxSpeedMps: 9,
            classLabel: label)
        t.x = x
        t.y = y
        t.alpha = 1
        return t
    }

    @Test func cityModeDrawsTheStreamByItsSensorToCity() throws {
        guard let renderer = try renderer() else { return }
        let index = try fixtureSiteIndex()
        let columbus = try #require(index.site(id: "columbus-broadway"))
        renderer.citySites = index.sites
        renderer.leaveSite(for: SiteContext(site: columbus))
        #expect(renderer.streamWorldFromSensor == matrix_identity_double4x4)

        renderer.setCityMode(true)
        #expect(renderer.isCityMode && renderer.recentresOrigin)
        #expect(renderer.streamWorldFromSensor == columbus.sensorToCity)
        #expect(renderer.cityTiles.worldFromCity == matrix_identity_double4x4)
        #expect(renderer.streamCoverage == .standard)
        // The origin sits at the sensor, so the stream's matrix is small.
        let model = renderer.streamModel
        #expect(simd_length(SIMD3(model.columns.3.x, model.columns.3.y, model.columns.3.z)) < 1)
        // The camera looks at the sensor from the arrival view.
        let target = renderer.origin.world(renderer.camera.target)
        #expect(simd_length(target - columbus.sensorCityPosition!) < 1)

        // A site without a pose is never drawn as though placed.
        renderer.leaveSite(for: SiteContext(site: index.site(id: "soma1")))
        #expect(renderer.streamCoverage == .hidden)

        renderer.setCityMode(false)
        #expect(!renderer.isCityMode && renderer.origin.anchor == .zero)
        #expect(renderer.camera == Camera(), "the view the operator had before city mode")
        #expect(renderer.streamCoverage == nil)
        #expect(!renderer.cityTilesPlaced)
    }

    /// A reference with every building layer and some street layers.
    private func siteReference() -> ReferenceOverlayVertices {
        let line = [Float](repeating: 0, count: 14)
        var reference = ReferenceOverlayVertices()
        reference.buildingLines = line
        reference.roofFills = [Float](repeating: 0, count: 21)
        reference.footprintLines = line
        reference.boxLines = line
        reference.cityStreetLines = line
        reference.laneMarkingLines = line
        reference.kerbLines = line
        return reference
    }

    @Test func inCityModeTheTilesAreTheBuildings() throws {
        guard let renderer = try renderer() else { return }
        let columbus = try #require(try fixtureSiteIndex().site(id: "columbus-broadway"))
        renderer.leaveSite(for: SiteContext(site: columbus))
        renderer.setReferenceOverlay(siteReference())
        // The site's own view: its buildings have arrived, so the city's fade.
        #expect(renderer.cityBuildingFade?.centre == columbus.sensorCityPosition)
        #expect(renderer.drawnReferenceVisibility == renderer.referenceVisibility)

        renderer.setCityMode(true)
        // The tiles are drawn in full, and the site's buildings not over them.
        #expect(!renderer.siteDrawsOwnSurroundings)
        #expect(renderer.cityBuildingFade == nil)
        let shown = renderer.drawnReferenceVisibility
        #expect(!shown.buildings && !shown.footprints && !shown.boxes)
        #expect(shown.cityStreets && shown.laneMarkings && shown.kerbs && shown.aerial)
        // The operator's own toggles are left as they were.
        #expect(renderer.referenceVisibility == ReferenceVisibility())

        renderer.setCityMode(false)
        #expect(renderer.cityBuildingFade?.centre == columbus.sensorCityPosition)
        #expect(renderer.drawnReferenceVisibility.buildings)
    }

    @Test func inCityModeASiteWithoutAPoseFadesNothing() throws {
        guard let renderer = try renderer() else { return }
        let index = try fixtureSiteIndex()
        renderer.leaveSite(for: SiteContext(site: index.site(id: "soma1")))
        renderer.setReferenceOverlay(siteReference())
        renderer.setCityMode(true)
        #expect(renderer.cityBuildingFade == nil)
        #expect(!renderer.drawnReferenceVisibility.buildings)
    }

    @Test func boxesAreClassColouredFadedAndKeptToTheirCoverage() throws {
        guard let renderer = try renderer() else { return }
        let columbus = try #require(try fixtureSiteIndex().site(id: "columbus-broadway"))
        renderer.leaveSite(for: SiteContext(site: columbus))
        renderer.setCityMode(true)
        var frame = FrameBundle()
        frame.tracks = TrackSet(
            frameID: 1, timestampNanos: 1,
            tracks: [
                track("near", x: 10, y: 0), track("edge", x: 110, y: 0, label: "pedestrian"),
                track("beyond", x: 150, y: 0),
            ], trails: [])
        renderer.updateFrame(frame)
        #expect(renderer.boxInstanceCount == 2)
        let floats = try #require(renderer.boxInstances).contents().bindMemory(
            to: Float.self, capacity: 40)
        let near = SIMD4(floats[16], floats[17], floats[18], floats[19])
        let edge = SIMD4(floats[36], floats[37], floats[38], floats[39])
        #expect(SIMD3(near.x, near.y, near.z) == ClassColour.colour(for: "car"))
        #expect(near.w == 1)
        #expect(SIMD3(edge.x, edge.y, edge.z) == ClassColour.colour(for: "pedestrian"))
        #expect(edge.w > 0 && edge.w < 1)
    }

    @Test func oneMarkerPerSiteWithAPlace() throws {
        guard let renderer = try renderer() else { return }
        let index = try fixtureSiteIndex()
        renderer.citySites = index.sites
        renderer.leaveSite(for: SiteContext(site: index.site(id: "columbus-broadway")))
        renderer.setCityMode(true)
        // Up high over the middle of the two sites.
        renderer.setOrigin(anchor: SIMD3(3340, 4260, 0))
        renderer.camera.target = .zero
        renderer.camera.position = SIMD3(0, -1500, 3000)
        renderer.camera.aspectRatio = 800 / 600
        let overlay = renderer.cityOverlay(size: CGSize(width: 800, height: 600))
        // soma1 has no cell and no marker: it is not drawn in the city at all.
        #expect(
            overlay.markers.map(\.id).sorted() == ["columbus-broadway", "embarcadero-broadway"])
        let focused = overlay.markers.first(where: { $0.focused })
        let embarcadero = overlay.markers.first(where: { $0.id == "embarcadero-broadway" })
        #expect(focused?.id == "columbus-broadway")
        #expect(overlay.markers.allSatisfy { $0.placed })
        #expect(embarcadero?.date == "2026-09-02")
    }

    @Test func streetNamesComeFromTheDrawnTiles() async throws {
        guard let renderer = try renderer() else { return }
        renderer.cityTiles.setTileset(
            try fixtureTileset(), url: cityTilesFixture.appendingPathComponent("tileset.json"))
        let columbus = try #require(try fixtureSiteIndex().site(id: "columbus-broadway"))
        renderer.leaveSite(for: SiteContext(site: columbus))
        renderer.setCityMode(true)
        renderer.setOrigin(anchor: SIMD3(3400, 4500, 0))
        renderer.camera.target = .zero
        renderer.camera.position = SIMD3(0, -1, 2500)
        renderer.camera.aspectRatio = 800 / 600
        for _ in 0..<8 {
            _ = renderer.cityTiles.select(view: renderer.cityTileView(viewportHeight: 600))
            await renderer.cityTiles.settle()
        }
        _ = renderer.cityTiles.select(view: renderer.cityTileView(viewportHeight: 600))
        let overlay = renderer.cityOverlay(size: CGSize(width: 800, height: 600))
        #expect(!overlay.streets.isEmpty)
        let names = overlay.streets.map(\.name)
        #expect(names.contains { $0.contains("COLUMBUS") })
    }

    @Test func aFlightEndsOverTheSite() throws {
        guard let renderer = try renderer() else { return }
        let index = try fixtureSiteIndex()
        renderer.leaveSite(for: SiteContext(site: index.site(id: "columbus-broadway")))
        renderer.setCityMode(true)
        let embarcadero = try #require(index.site(id: "embarcadero-broadway")?.sensorCityPosition)
        renderer.fly(to: embarcadero)
        #expect(renderer.isFlying)
        renderer.advanceFlight(now: CACurrentMediaTime() + 10)
        #expect(!renderer.isFlying)
        #expect(simd_length(renderer.origin.world(renderer.camera.target) - embarcadero) < 1e-3)
        // The origin keeps up with the camera: its target stays near it.
        #expect(simd_length(renderer.camera.target) <= renderer.origin.recentreDistance)
    }
}

@MainActor struct CityModeAppStateTests {
    @Test func switchingInCityModeFliesToTheSite() async throws {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        let renderer = try #require(MetalRenderer(metalView: view))
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        state.registerRenderer(renderer)
        state.city.setIndex(try fixtureSiteIndex(), error: nil)
        state.city.setCityMode(true)
        #expect(state.city.isCityMode && renderer.isCityMode)
        let site = try #require(state.city.index?.site(id: "embarcadero-broadway"))
        let service = FakeSiteReplayService(
            clips: [
                AlignmentSite(
                    clipID: "clip-e", l16Token: "8085805e5", l13Token: "8085805c",
                    description: "Embarcadero", sensorID: "hesai-pandar40p", latitude: 0,
                    longitude: 0)
            ], files: ["clip-e": ["20260902T1522_80858-05c_embarcadero-broadway.pcapng"]])
        try await state.switchSite(to: site, service: service)
        #expect(renderer.isFlying)
        #expect(renderer.streamWorldFromSensor == site.sensorToCity)
        state.city.setCityMode(false)
        #expect(!renderer.isCityMode)
        #expect(!renderer.isFlying)
        renderer.cancelFlight()
    }

    @Test func cityModeNeedsTilesOrSites() {
        let city = CityViewSession(defaults: nil)
        city.setCityMode(true)
        #expect(!city.isCityMode)
    }
}
