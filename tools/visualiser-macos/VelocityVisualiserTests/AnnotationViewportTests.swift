//
//  AnnotationViewportTests.swift
//  VelocityVisualiserTests
//
//  Tests for the screen/world mapping the lasso overlay depends on, and for
//  the orthographic camera the selection views use.
//
//  A sign error in the vertical flip here would select points the operator
//  did not draw around, and it would do so silently, so the mapping is tested
//  directly rather than through a mounted view.
//

import CoreGraphics
import Foundation
import Testing
import simd

@testable import VelocityVisualiser

struct OrthoViewportTests {
    private let viewport = OrthoViewport(
        halfHeight: 10, size: CGSize(width: 400, height: 200), centre: .zero)

    @Test func halfWidthFollowsTheAspectRatio() {
        // 400x200 at 10 m half-height is 20 m half-width.
        #expect(viewport.halfWidth == 20)
    }

    @Test func centreOfViewIsTheViewPlaneCentre() {
        let world = viewport.worldPoint(from: CGPoint(x: 200, y: 100))
        #expect(abs(world.x) < 1e-5)
        #expect(abs(world.y) < 1e-5)
    }

    @Test func screenYIsFlippedBecauseTheViewPlanePointsUp() {
        // Top of the view is +halfHeight, bottom is -halfHeight. Getting this
        // backwards mirrors every selection vertically.
        let top = viewport.worldPoint(from: CGPoint(x: 200, y: 0))
        let bottom = viewport.worldPoint(from: CGPoint(x: 200, y: 200))
        #expect(abs(top.y - 10) < 1e-5)
        #expect(abs(bottom.y + 10) < 1e-5)
    }

    @Test func screenXIsNotFlipped() {
        let left = viewport.worldPoint(from: CGPoint(x: 0, y: 100))
        let right = viewport.worldPoint(from: CGPoint(x: 400, y: 100))
        #expect(abs(left.x + 20) < 1e-5)
        #expect(abs(right.x - 20) < 1e-5)
    }

    @Test func roundTripsThroughScreenSpace() {
        for world in [simd_float2(0, 0), simd_float2(7, -3), simd_float2(-19.5, 9.5)] {
            let screen = viewport.screenPoint(from: world)
            let back = viewport.worldPoint(from: screen)
            #expect(abs(back.x - world.x) < 1e-4)
            #expect(abs(back.y - world.y) < 1e-4)
        }
    }

    @Test func centreOffsetShiftsTheWholeMapping() {
        let panned = OrthoViewport(
            halfHeight: 10, size: CGSize(width: 400, height: 200),
            centre: simd_float2(100, 50))
        let world = panned.worldPoint(from: CGPoint(x: 200, y: 100))
        #expect(abs(world.x - 100) < 1e-5)
        #expect(abs(world.y - 50) < 1e-5)
    }

    @Test func degenerateSizeDoesNotDivideByZero() {
        let empty = OrthoViewport(halfHeight: 10, size: .zero, centre: simd_float2(3, 4))
        #expect(empty.worldPoint(from: CGPoint(x: 5, y: 5)) == simd_float2(3, 4))
    }
}

struct OrthographicCameraTests {
    @Test func orthographicProjectionKeepsScaleIndependentOfDepth() {
        var camera = Camera()
        camera.aspectRatio = 1
        camera.projection = .orthographic(halfHeight: 10)
        let projection = camera.projectionMatrix

        // The same world offset must land at the same clip-space offset at
        // any depth. Under perspective it would not, which is exactly why
        // selection uses this projection.
        let near = projection * simd_float4(5, 0, -10, 1)
        let far = projection * simd_float4(5, 0, -100, 1)
        #expect(abs(near.x - far.x) < 1e-5)
        #expect(abs(near.w - 1) < 1e-5)
        #expect(abs(far.w - 1) < 1e-5)
    }

    @Test func orthographicHalfHeightMapsToClipSpaceEdge() {
        var camera = Camera()
        camera.aspectRatio = 2
        camera.projection = .orthographic(halfHeight: 10)
        let projection = camera.projectionMatrix

        // y = halfHeight is the top edge; x = halfHeight*aspect is the right.
        let topEdge = projection * simd_float4(0, 10, -50, 1)
        let rightEdge = projection * simd_float4(20, 0, -50, 1)
        #expect(abs(topEdge.y - 1) < 1e-5)
        #expect(abs(rightEdge.x - 1) < 1e-5)
    }

    @Test func defaultCameraStaysPerspective() {
        // Adding the mode must not change what every existing view renders.
        let camera = Camera()
        #expect(camera.projection == .perspective)
        // A perspective projection divides by w, so w tracks depth.
        let clip = camera.projectionMatrix * simd_float4(0, 0, -10, 1)
        #expect(clip.w != 1)
    }

    @Test func degenerateHalfHeightIsClampedNotDividedByZero() {
        var camera = Camera()
        camera.projection = .orthographic(halfHeight: 0)
        let projection = camera.projectionMatrix
        // Finite, not NaN or infinite.
        #expect(projection.columns.1.y.isFinite)
    }

    @Test func setOrthoViewPlacesTheEyeBackAlongTheViewAxis() {
        var camera = Camera()
        let centre = simd_float3(5, 10, 1)
        camera.setOrthoView(.top, centre: centre, halfHeight: 15, distance: 100)

        #expect(camera.target == centre)
        // Top view looks down, so the eye is above the centre.
        #expect(camera.position.z > centre.z)
        #expect(abs(camera.position.x - centre.x) < 1e-5)
        #expect(abs(camera.position.y - centre.y) < 1e-5)
        #expect(camera.projection == .orthographic(halfHeight: 15))
        // Far enough that nothing in the slab is clipped away behind the
        // centre.
        #expect(camera.farPlane >= 200)
    }

    @Test func setOrthoViewUsesTheBasisUpVectorForEachStandard() {
        for standard in OrthoViewBasis.Standard.allCases {
            var camera = Camera()
            camera.setOrthoView(standard, centre: .zero, halfHeight: 10)
            let basis = OrthoViewBasis(standard)
            #expect(simd_length(camera.up - basis.up) < 1e-5)
            // The eye sits opposite the depth axis, so the scene is in front.
            #expect(simd_dot(simd_normalize(camera.target - camera.position), basis.forward) > 0.99)
        }
    }
}
