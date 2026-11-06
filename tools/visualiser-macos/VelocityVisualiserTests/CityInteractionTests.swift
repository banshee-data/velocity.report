// CityInteractionTests.swift
// The city view's camera and site picking: a drag pans the ground under
// the pointer at any distance, the operator's hand ends a flight, a site
// chosen during a switch is switched to after it, a failed switch says
// which site and why, and the overlay's per-frame changes do not redraw
// the views that observe the session (the toolbar's site menu).

import Combine
import Foundation
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

@MainActor struct CityCameraInputTests {
    private func renderer() throws -> MetalRenderer? {
        guard let device = MTLCreateSystemDefaultDevice() else { return nil }
        let view = MTKView(frame: CGRect(x: 0, y: 0, width: 800, height: 600))
        view.device = device
        return MetalRenderer(metalView: view)
    }

    /// Puts the camera `distance` from a target at the origin, looking down.
    private func place(_ renderer: MetalRenderer, distance: Float) {
        renderer.camera.target = SIMD3(0, 0, 0)
        renderer.camera.position = SIMD3(0, -distance * 0.5, distance * 0.866)
    }

    @Test func aDragPansTheGroundUnderThePointerAtAnyDistance() throws {
        guard let renderer = try renderer() else { return }
        var moved = [Float]()
        for distance: Float in [50, 5_000] {
            place(renderer, distance: distance)
            renderer.handleMouseDrag(
                deltaX: 100, deltaY: 0, isRightButton: true, shiftHeld: false, viewHeight: 800)
            moved.append(simd_length(renderer.camera.target))
        }
        // 100 points of an 800-point view is an eighth of the frustum's
        // height at the target: 2·d·tan(30°)/8.
        #expect(abs(moved[0] - 2 * 50 * tan(Float.pi / 6) / 8) < 0.01)
        #expect(abs(moved[1] / moved[0] - 100) < 0.01)
        // Over the city, a drag across the view crosses hundreds of metres.
        #expect(moved[1] > 500)
    }

    @Test func arrowKeysStepWithTheDistance() throws {
        guard let renderer = try renderer() else { return }
        place(renderer, distance: 4_000)
        #expect(renderer.handleKeyDown(keyCode: 123, modifiers: []))
        #expect(abs(simd_length(renderer.camera.target) - 200) < 0.01)
        place(renderer, distance: 10)
        #expect(renderer.handleKeyDown(keyCode: 123, modifiers: []))
        #expect(abs(simd_length(renderer.camera.target) - 2) < 0.01)
    }

    @Test func theOperatorsHandEndsAFlight() throws {
        guard let renderer = try renderer() else { return }
        renderer.setCityMode(true)
        renderer.fly(to: SIMD3(1_000, 1_000, 0))
        #expect(renderer.isFlying)
        renderer.handleMouseDrag(
            deltaX: 10, deltaY: 0, isRightButton: true, shiftHeld: false, viewHeight: 600)
        #expect(!renderer.isFlying)
        renderer.fly(to: SIMD3(1_000, 1_000, 0))
        renderer.handleZoom(delta: 1)
        #expect(!renderer.isFlying)
        renderer.fly(to: SIMD3(1_000, 1_000, 0))
        _ = renderer.handleKeyDown(keyCode: 126, modifiers: [])
        #expect(!renderer.isFlying)
    }
}

@MainActor struct CitySitePickingTests {
    @Test func aSiteChosenDuringASwitchIsSwitchedToAfterIt() async throws {
        let city = CityViewSession(defaults: nil, bundledIndex: nil)
        let index = try fixtureSiteIndex()
        city.setIndex(index, error: nil)
        let gate = CityTestBox<CheckedContinuation<Void, Never>?>(nil)
        let asked = CityTestBox<[String]>([])
        city.switcher = { site in
            asked.value.append(site.id)
            if asked.value.count == 1 { await withCheckedContinuation { gate.value = $0 } }
            return .nothingToPlay
        }
        let sites = index.sites
        let first = Task { await city.select(sites[0]) }
        while gate.value == nil { await Task.yield() }
        // Two more clicks while the first runs: the latest is the one kept.
        await city.select(sites[1])
        await city.select(sites[2])
        #expect(city.switchingTo == sites[0].id)
        gate.value?.resume()
        await first.value
        #expect(asked.value == [sites[0].id, sites[2].id])
        #expect(city.switchingTo == nil)
    }

    @Test func aFailedSwitchNamesTheSiteAndCanBeDismissed() async throws {
        let city = CityViewSession(defaults: nil, bundledIndex: nil)
        let index = try fixtureSiteIndex()
        city.setIndex(index, error: nil)
        city.switcher = { _ in throw SiteSwitchError.replayRefused("busy") }
        let site = index.sites[0]
        await city.select(site)
        #expect(city.switchError?.hasPrefix(site.title) == true)
        #expect(city.switchError?.contains("busy") == true)
        city.dismissSwitchError()
        #expect(city.switchError == nil)
    }

    @Test func theOverlaysChangesDoNotRedrawTheSiteMenu() {
        let city = CityViewSession(defaults: nil, bundledIndex: nil)
        var sessionChanges = 0
        var liveChanges = 0
        let a = city.objectWillChange.sink { sessionChanges += 1 }
        let b = city.live.objectWillChange.sink { liveChanges += 1 }
        for i in 0..<10 {
            city.live.overlay = CityOverlay(
                markers: [
                    CityMarker(
                        id: "s", title: "S", date: "", clock: nil, placed: true, focused: false,
                        screen: CGPoint(x: i, y: i))
                ], streets: [])
            city.live.siteClocks = ["s": "12:00:0\(i)"]
        }
        #expect(liveChanges == 20)
        #expect(sessionChanges == 0)
        a.cancel()
        b.cancel()
    }

    @Test func enteringASiteTellsTheSession() throws {
        let state = AppState(autoConnectOnStartup: false, serverDefaults: nil)
        let site = try #require(try fixtureSiteIndex().sites.first)
        state.enterSite(SiteContext(site: site, source: .nothingToPlay))
        #expect(state.city.currentSite?.id == site.id)
        state.city.leaveSite()
        #expect(state.city.currentSite == nil)
        #expect(state.siteContext.site == nil)
    }
}
