//
//  IntensityInspectionTests.swift
//  VelocityVisualiserTests
//
//  Raw intensity inspection: exact readout of the stored byte, a display
//  range that clamps colour but never the value, one colour table for both
//  renderers, an explicit colour for "not measured", a picker that names the
//  return actually drawn on top, and display controls that change nothing
//  saved.
//

import Foundation
import Testing
import simd

@testable import VelocityVisualiser

struct IntensityDisplaySpecTests {
    @Test func offByDefaultOverTheFullRange() {
        let spec = IntensityDisplaySpec()
        #expect(!spec.enabled && spec.lower == 1 && spec.upper == 255 && spec.isDefaultRange)
        #expect(spec.gamma == 1 && spec.brightness == 1 && spec.ramp == .starburst)
    }

    @Test func zeroRemainsDistinctFromNonzeroAndMissingUnderEveryDisplayTransform() {
        for ramp in IntensityRamp.allCases {
            for brightness in [0.5, 1.0, 2.0] {
                var spec = IntensityDisplaySpec()
                spec.ramp = ramp
                spec.setBrightness(brightness)
                spec.setGamma(2)
                spec.setLower(50)
                spec.setUpper(200)
                #expect(spec.colour(of: 0) == IntensityDisplaySpec.zeroColour)
                #expect(spec.colour(of: 0) != IntensityDisplaySpec.missingColour)
                for code in 1...255 { #expect(spec.colour(of: UInt8(code)) != spec.colour(of: 0)) }
                #expect(spec.position(of: 0).clip == .none)
                #expect(spec.clipCounts([0, 0] as [UInt8]).below == 0)
            }
        }
    }

    @Test func limitsStayOrderedAndInRange() {
        var spec = IntensityDisplaySpec()
        spec.setLower(300)
        #expect(spec.lower == 254 && spec.upper == 255)
        spec.setUpper(-5)
        #expect(spec.upper == 2 && spec.lower == 1)
        spec.setLower(40)
        #expect(spec.lower == 40 && spec.upper == 41)
        spec.setUpper(200)
        spec.setLower(100)
        #expect(spec.lower == 100 && spec.upper == 200)
        spec.setGamma(99)
        #expect(spec.gamma == IntensityDisplaySpec.gammaRange.upperBound)
        spec.setBrightness(0)
        #expect(spec.brightness == IntensityDisplaySpec.brightnessRange.lowerBound)
    }

    @Test func rangeClampsColourAndSaysSo() {
        var spec = IntensityDisplaySpec()
        spec.setLower(100)
        spec.setUpper(200)
        #expect(spec.position(of: 99) == (0, .below))
        #expect(spec.position(of: 201) == (1, .above))
        #expect(spec.position(of: 100).clip == .none && spec.position(of: 100).t == 0)
        #expect(spec.position(of: 200).clip == .none && spec.position(of: 200).t == 1)
        #expect(abs(spec.position(of: 150).t - 0.5) < 1e-6)
        #expect(spec.colour(of: 0) == IntensityDisplaySpec.zeroColour)
        #expect(spec.colour(of: 255) == spec.colour(of: 200))
        let counts = spec.clipCounts([0, 99, 100, 150, 200, 201, 255] as [UInt8])
        #expect(counts.below == 1 && counts.above == 2)
    }

    @Test func gammaMovesTheLegendTicksNotTheValues() {
        var spec = IntensityDisplaySpec()
        let linear = spec.legendTicks()
        #expect(linear.map(\.raw) == [1, 64.5, 128, 191.5, 255])
        spec.setGamma(2)
        let curved = spec.legendTicks()
        #expect(curved.first?.raw == 1 && curved.last?.raw == 255)
        // Halfway along the bar is t = 0.5, which gamma 2 reaches at a
        // linear fraction of sqrt(0.5).
        #expect(abs(curved[2].raw - (1 + 254 * 0.5.squareRoot())) < 1e-9)
        #expect(abs(Double(spec.position(of: 180).t) - pow(179.0 / 254, 2)) < 1e-6)
    }

    @Test func theTableHasOneColourPerCodeAndMissingIsItsOwnColour() {
        var spec = IntensityDisplaySpec()
        spec.enabled = true
        let table = spec.table(available: true)
        #expect(table.count == 256)
        for code in [0, 1, 127, 128, 254, 255] {
            #expect(table[code] == SIMD4(spec.colour(of: UInt8(code)), 1))
        }
        // Zero is a measurement, drawn in the ramp's low colour, and not the
        // colour of "not measured".
        #expect(SIMD3(table[0].x, table[0].y, table[0].z) != IntensityDisplaySpec.missingColour)
        let absent = spec.table(available: false)
        #expect(absent.allSatisfy { $0 == SIMD4(IntensityDisplaySpec.missingColour, 1) })
        for ramp in IntensityRamp.allCases {
            spec.ramp = ramp
            for entry in spec.table(available: true) {
                #expect(SIMD3(entry.x, entry.y, entry.z) != IntensityDisplaySpec.missingColour)
                #expect(
                    entry.x >= 0 && entry.x <= 1 && entry.y >= 0 && entry.y <= 1 && entry.z >= 0
                        && entry.z <= 1)
            }
            // The low end is readable on the black views.
            let low = spec.colour(of: 0)
            #expect(max(low.x, low.y, low.z) >= 0.25, "\(ramp) starts too dark")
        }
    }

    @Test func resetKeepsTheToggleAndRestoresEverythingElse() {
        var spec = IntensityDisplaySpec()
        spec.enabled = true
        spec.setLower(20)
        spec.setGamma(2)
        spec.ramp = .heat
        spec.setBrightness(1.5)
        spec.reset()
        var expected = IntensityDisplaySpec()
        expected.enabled = true
        #expect(spec == expected)
    }

    @Test func availabilityComesFromTheManifestNotTheBytes() {
        #expect(IntensityAvailability(hasIntensity: true) == .packFlag)
        #expect(IntensityAvailability(hasIntensity: false) == .absent)
        #expect(!IntensityAvailability.absent.available)
        #expect(IntensityAvailability.packFlag.caveat.contains("per frame"))
    }

    /// The shader indexes the same table by the exact code: intensity / 255
    /// rounded back. Held here, as the palette test holds the palette.
    @Test func theShaderLooksUpTheRawCodeInTheSharedTable() throws {
        let shader = try String(
            contentsOf: URL(fileURLWithPath: #filePath).deletingLastPathComponent()
                .deletingLastPathComponent().appendingPathComponent(
                    "VelocityVisualiser/Rendering/Shaders/PointCloud.metal"), encoding: .utf8)
        #expect(shader.contains("constant float4 *intensityTable [[buffer(0)]]"))
        #expect(shader.contains("int code = clamp(int(in.intensity * 255.0 + 0.5), 0, 255);"))
        #expect(shader.contains("colour = intensityTable[code].rgb;"))
        // Every byte survives the renderer's normalisation and comes back.
        for code in 0...255 {
            let normalised = Float(UInt8(code)) / 255.0
            #expect(Int(normalised * 255.0 + 0.5) == code)
        }
        #expect(MetalRenderer.IntensityColouring.modulated.rawValue == 0)
        #expect(MetalRenderer.IntensityColouring.flat.rawValue == 1)
        #expect(MetalRenderer.IntensityColouring.table.rawValue == 2)
    }
}

@MainActor struct IntensityInspectionStateTests {
    private func readout(_ index: Int, raw: UInt8?) -> IntensityReadout {
        IntensityReadout(
            sampleID: 0, sourceOrdinal: 1, pointIndex: index, raw: raw, position: .zero,
            displayClass: 1, depth: Float(index))
    }

    @Test func pinKeepsAReturnAndNextStepsThroughTheOverlap() {
        let state = IntensityInspectionState()
        state.setHovered([readout(3, raw: 10), readout(7, raw: 200)])
        #expect(state.current?.pointIndex == 3)
        state.next()
        #expect(state.current?.pointIndex == 7)
        state.pin()
        #expect(state.pinned?.pointIndex == 7)
        state.setHovered([])
        #expect(state.pinned?.pointIndex == 7, "a pinned readout outlives the cursor")
        state.next()
        #expect(state.pinned?.pointIndex == 3)
        state.clear()
        #expect(state.pinned == nil && state.current == nil)
    }
}

@MainActor struct IntensitySessionTests {
    private let codes: [UInt8] = [0, 1, 127, 128, 254, 255]

    private func session(hasIntensity: Bool = true) throws -> AnnotationSession {
        let points: [SyntheticPack.Point] = (0..<6).map { (Float($0) * 2, 5, 1, 1) }
        let dir = try SyntheticPack.write(
            [points, points], intensities: [codes, codes], hasIntensity: hasIntensity)
        return try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
    }

    private func viewport(_ session: AnnotationSession) -> OrthoViewport {
        OrthoViewport(
            halfHeight: 10, size: CGSize(width: 400, height: 400), centre: simd_float2(5, 5))
    }

    @Test func theReadoutIsTheStoredByteExactly() throws {
        let session = try session()
        session.inspectIntensity = true
        session.intensityDisplay.enabled = true
        session.intensityDisplay.setLower(100)
        session.intensityDisplay.setGamma(3)
        let vp = viewport(session)
        for (index, code) in codes.enumerated() {
            let screen = vp.screenPoint(
                from: session.basis(.top).project(simd_float3(Float(index) * 2, 5, 1)))
            session.inspectReturns(in: .top, viewport: vp, at: screen)
            #expect(session.inspection.current?.pointIndex == index)
            #expect(session.inspection.current?.raw == code, "clamped or normalised the readout")
        }
        // Turned: the same return, the same byte.
        session.gridAzimuthDeg = 37
        let screen = vp.screenPoint(from: session.basis(.top).project(simd_float3(10, 5, 1)))
        session.inspectReturns(in: .top, viewport: vp, at: screen)
        #expect(session.inspection.current?.raw == 255)
    }

    @Test func absentIntensityReadsUnavailableNotZero() throws {
        let session = try session(hasIntensity: false)
        #expect(!session.intensityAvailability.available)
        session.inspectIntensity = true
        let vp = viewport(session)
        let screen = vp.screenPoint(from: session.basis(.top).project(simd_float3(0, 5, 1)))
        session.inspectReturns(in: .top, viewport: vp, at: screen)
        #expect(session.inspection.current != nil)
        #expect(session.inspection.current?.raw == nil)
    }

    @Test func theTopReturnIsPickedFirstAndHiddenReturnsAreNot() throws {
        // Two returns at one place in the top view, at different heights.
        let points: [SyntheticPack.Point] = [(0, 0, 0.5, 1), (0, 0, 2.0, 1), (0, 0.02, 1.0, 2)]
        let dir = try SyntheticPack.write([points], intensities: [[10, 20, 30]])
        let session = try AnnotationSession(pack: try AnnotationPack.open(directory: dir))
        session.inspectIntensity = true
        let vp = OrthoViewport(halfHeight: 5, size: CGSize(width: 200, height: 200), centre: .zero)
        let screen = vp.screenPoint(from: .zero)
        session.inspectReturns(in: .top, viewport: vp, at: screen)
        // Top looks down: the highest return is nearest the viewer.
        #expect(session.inspection.hovered.map(\.pointIndex).first == 1)
        #expect(Set(session.inspection.hovered.map(\.pointIndex)) == [0, 1, 2])
        session.visibility.ground = false
        session.inspectReturns(in: .top, viewport: vp, at: screen)
        #expect(
            !session.inspection.hovered.contains { $0.pointIndex == 2 }, "picked a hidden return")
        // Stepping frames forgets a point index: it named a return in one scan.
        session.inspection.pin()
        #expect(session.inspection.pinned != nil)
        session.inspectIntensity = false
        #expect(session.inspection.pinned == nil)
    }

    @Test func displayControlsChangeNothingSavedOrDirty() throws {
        let session = try session()
        let sidecar = session.sidecar
        let digest = session.membershipDigest
        let bytes = session.currentPoints.intensity
        session.intensityDisplay.enabled = true
        session.intensityDisplay.setLower(50)
        session.intensityDisplay.ramp = .grey
        session.intensityDisplay.enabled = false
        session.inspectIntensity = true
        #expect(session.sidecar == sidecar && session.membershipDigest == digest)
        #expect(session.currentPoints.intensity == bytes)
        #expect(session.navigationGuard() == nil)
    }

    @Test func steppingClearsTheReadout() throws {
        let session = try session()
        session.inspectIntensity = true
        let vp = viewport(session)
        session.inspectReturns(
            in: .top, viewport: vp,
            at: vp.screenPoint(from: session.basis(.top).project(simd_float3(0, 5, 1))))
        session.inspection.pin()
        #expect(session.inspection.pinned != nil)
        #expect(session.stepForward() == nil)
        #expect(session.inspection.pinned == nil && session.inspection.hovered.isEmpty)
    }
}
