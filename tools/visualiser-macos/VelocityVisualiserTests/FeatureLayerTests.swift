//
//  FeatureLayerTests.swift
//  VelocityVisualiserTests
//
//  Gerardus's W4–W7 layers without a server: how BUILDINGS, LANE_MARKINGS,
//  STREET_FURNITURE and TREES decode, the vertices each kind draws (part
//  prisms, roof triangles, rings for poles and crowns, thick signs), their
//  colours, and that the session asks for each layer only when the server
//  lists it.
//

import Foundation
import Metal
import MetalKit
import Testing
import simd

@testable import VelocityVisualiser

// MARK: - Fixtures

private func feature(
    _ kind: Gerardus_Align_V1alpha1_FeatureKind, _ xyz: [Float], id: String = "",
    closed: Bool = false, tags: [String: String] = [:]
) -> Gerardus_Align_V1alpha1_LineFeature {
    var f = Gerardus_Align_V1alpha1_LineFeature()
    f.featureID = id
    f.kind = kind
    f.xyz = xyz
    f.closed = closed
    f.tags = tags
    return f
}

/// The main part: a 10 m square on the ground at 21 m, 12 m tall, gabled.
private let mainPart = feature(
    .buildingPart, [0, 0, 21, 10, 0, 21, 10, 10, 21, 0, 10, 21], id: "gms:part/main", closed: true,
    tags: ["height": "12", "roof:shape": "gabled", "roof:height": "2", "roof:direction": "180"])
/// A bay over a garage: its outline 3 m up, rising to 8 m.
private let bayPart = feature(
    .buildingPart, [2, -1, 24, 4, -1, 24, 4, 0, 24, 2, 0, 24], id: "gms:part/bay", closed: true,
    tags: ["height": "8", "min_height": "3", "roof:shape": "flat"])

/// The main part's two gabled faces, ridge along y = 5 at 33 m.
private let roofFaces = [
    feature(
        .roofPlane, [0, 0, 31, 10, 0, 31, 10, 5, 33, 0, 5, 33], closed: true,
        tags: ["part": "gms:part/main", "slope_deg": "21.8", "azimuth_deg": "180", "role": "roof"]),
    feature(
        .roofPlane, [0, 5, 33, 10, 5, 33, 10, 10, 31, 0, 10, 31], closed: true,
        tags: ["part": "gms:part/main", "slope_deg": "21.8", "azimuth_deg": "0", "role": "roof"]),
]

/// The W4–W7 layers as a server would send them, for FakeAlignmentService.
func featureLayerMessages(
    _ layer: Gerardus_Align_V1alpha1_ReferenceLayer
) -> [Gerardus_Align_V1alpha1_StreamReferenceResponse] {
    var m = Gerardus_Align_V1alpha1_StreamReferenceResponse()
    m.layer = layer
    m.layerComplete = true
    switch layer {
    case .buildings:
        m.lines.features = [mainPart, bayPart] + roofFaces
        m.modelID = "sha256:buildings"
    case .laneMarkings:
        m.lines.features = [
            feature(
                .laneMarking, [-20, 0, 21, 0, 0, 21, 20, 0, 21],
                tags: ["marking": "centre_line", "pattern": "double", "colour": "yellow"]),
            feature(
                .laneMarking, [-2, -5, 21, 2, -5, 21, 2, 5, 21, -2, 5, 21], closed: true,
                tags: ["marking": "crosswalk", "pattern": "solid", "colour": "white"]),
        ]
    case .streetFurniture:
        m.lines.features = [
            feature(
                .pole, [3, 3, 21, 3, 3, 27], id: "gms:pole/1",
                tags: ["pole_type": "streetlight", "diameter_m": "0.3"]),
            feature(
                .sign, [3.2, 3, 23.5, 3.2, 3, 24.3],
                tags: ["sign_type": "R1-1", "pole": "gms:pole/1"]),
        ]
    case .trees:
        m.lines.features = [
            feature(
                .tree, [5, -3, 21, 5, -3, 23.5],
                tags: [
                    "dbh_m": "0.4", "height_m": "9", "crown_base_m": "2.5", "crown_radius_m": "3",
                    "species": "Platanus x hispanica",
                ])
        ]
    default: return []
    }
    return [m]
}

private func geometry(_ layers: [Gerardus_Align_V1alpha1_ReferenceLayer]) -> ReferenceGeometry {
    var g = ReferenceGeometry()
    for layer in layers { for message in featureLayerMessages(layer) { g.append(message) } }
    return g
}

private func distance(_ a: SIMD4<Float>, _ b: SIMD4<Float>) -> Float {
    simd_length(SIMD3(a.x, a.y, a.z) - SIMD3(b.x, b.y, b.z))
}

/// The vertices of a debug-line buffer.
private func points(_ floats: [Float]) -> [SIMD3<Float>] {
    stride(from: 0, to: floats.count, by: 7).map {
        SIMD3(floats[$0], floats[$0 + 1], floats[$0 + 2])
    }
}

/// How far a point lies from the pole at (3, 3), across the ground.
private func offAxis(_ p: SIMD3<Float>) -> Float { simd_length(SIMD2<Float>(p.x - 3, p.y - 3)) }

private func colours(_ floats: [Float]) -> [SIMD4<Float>] {
    stride(from: 0, to: floats.count, by: 7).map {
        SIMD4(floats[$0 + 3], floats[$0 + 4], floats[$0 + 5], floats[$0 + 6])
    }
}

/// The area of `ring`'s triangles, by the cross product.
private func area(_ ring: [SIMD3<Float>], _ triangles: [Int]) -> Float {
    stride(from: 0, to: triangles.count, by: 3).reduce(0) { total, t in
        let a = ring[triangles[t]]
        let b = ring[triangles[t + 1]]
        let c = ring[triangles[t + 2]]
        return total + simd_length(simd_cross(b - a, c - a)) / 2
    }
}

// MARK: - Decoding and colours

struct FeatureLayerDecodingTests {
    @Test func eachLayerKeepsItsOwnFeatures() {
        let g = geometry([.buildings, .laneMarkings, .streetFurniture, .trees])
        #expect(g.buildingParts.map(\.featureID) == ["gms:part/main", "gms:part/bay"])
        #expect(g.roofPlanes.count == 2)
        #expect(g.laneMarkings.count == 2)
        #expect(g.streetFurniture.map(\.kind) == [.pole, .sign])
        #expect(g.trees.count == 1)
        // None lands among the OSM ways.
        #expect(g.lines.isEmpty)
        #expect(g.completeLayers == [.buildings, .laneMarkings, .streetFurniture, .trees])
        // BUILDINGS names a model of its own, which is not the site model.
        #expect(g.modelID.isEmpty)
    }

    @Test func mergingCarriesTheNewLayers() {
        var g = ReferenceGeometry()
        g.merge(geometry([.buildings, .laneMarkings, .streetFurniture, .trees]))
        #expect(g.buildingParts.count == 2)
        #expect(g.roofPlanes.count == 2)
        #expect(g.laneMarkings.count == 2)
        #expect(g.streetFurniture.count == 2)
        #expect(g.trees.count == 1)
    }

    /// Each new kind has a colour of its own, apart from every other kind's
    /// and from the colour a kind the client does not know gets.
    @Test func eachNewKindHasItsOwnColour() {
        let new: [Gerardus_Align_V1alpha1_FeatureKind] = [
            .roofPlane, .buildingPart, .laneMarking, .sign, .tree,
        ]
        let old: [Gerardus_Align_V1alpha1_FeatureKind] = [
            .road, .path, .kerb, .rail, .building, .sensor, .correction, .facade, .pole, .island,
            .driveway, .parkingSpace, .travelLane, .surface, .other,
        ]
        let colour = { (k: Gerardus_Align_V1alpha1_FeatureKind) in ReferenceStyle.lineColour(k) }
        let all = (new + old).map(colour)
        #expect(Set(new.map(colour).map { [$0.x, $0.y, $0.z] }).count == new.count)
        for kind in new {
            let c = colour(kind)
            #expect(c != ReferenceStyle.otherColour, "\(kind)")
            #expect(all.filter { $0 == c }.count == 1, "\(kind)")
        }
        #expect(ReferenceStyle.treeTrunkColour != colour(.tree))
    }

    @Test func signsTreesAndRoofsAreDrawnWhereTheyAre() {
        for kind: Gerardus_Align_V1alpha1_FeatureKind in [.sign, .tree, .roofPlane, .pole] {
            #expect(ReferenceStyle.lift(kind) == 0)
        }
        #expect(ReferenceStyle.lift(.laneMarking) == ReferenceStyle.lineLift)
    }

    @Test func tagNumbersReadTheirLeadingNumber() {
        #expect(ReferenceStyle.number(["height": "12"], "height") == 12)
        #expect(ReferenceStyle.number(["height": " 12.5 m"], "height") == 12.5)
        #expect(ReferenceStyle.number(["height": "tall"], "height") == nil)
        #expect(ReferenceStyle.number([:], "height") == nil)
    }
}

// MARK: - Buildings

struct BuildingStyleTests {
    @Test func aPartIsExtrudedFromMinHeightToHeight() {
        #expect(ReferenceStyle.partRise(mainPart) == 12)
        // The bay's outline is already 3 m up: it rises 8 − 3.
        #expect(ReferenceStyle.partRise(bayPart) == 5)
        let v = ReferenceStyle.buildingPartVertices([bayPart])
        // The base ring, the top ring and four uprights.
        #expect(v.count / 7 == 24)
        let z = Set(points(v).map(\.z))
        #expect(z == [24, 29])
        #expect(colours(v).allSatisfy { $0 == ReferenceStyle.lineColour(.buildingPart) })
    }

    @Test func aPartWithoutAHeightIsItsOutline() {
        var flat = bayPart
        flat.tags = [:]
        #expect(ReferenceStyle.partRise(flat) == 0)
        #expect(ReferenceStyle.buildingPartVertices([flat]).count / 7 == 8)
        // height_m stands in when the tags carry no height.
        flat.heightM = 4
        #expect(ReferenceStyle.partRise(flat) == 4)
    }

    @Test func aRoofPlaneIsTriangulatedAtItsZ() {
        let (fills, lines) = ReferenceStyle.roofPlaneVertices(
            roofFaces, parts: [mainPart, bayPart], colouring: .slope)
        // Two quads, two triangles each; each outlined by four segments.
        #expect(fills.count / 7 == 12)
        #expect(lines.count / 7 == 16)
        #expect(Set(points(fills).map(\.z)) == [31, 33])
        #expect(colours(fills).allSatisfy { $0.w == ReferenceStyle.roofFillAlpha })
        #expect(colours(lines).allSatisfy { $0.w == 1 })
    }

    @Test func earClippingFillsAConcaveRingExactly() {
        // An L, anticlockwise, on a tilted plane z = x.
        let l: [SIMD3<Float>] = [
            SIMD3(0, 0, 0), SIMD3(4, 0, 4), SIMD3(4, 2, 4), SIMD3(2, 2, 2), SIMD3(2, 4, 2),
            SIMD3(0, 4, 0),
        ]
        let triangles = ReferenceStyle.triangulate(l)
        let tilted: Float = 12 * Float(2).squareRoot()
        #expect(triangles.count == 12)
        // The L's plan area is 12 m²; tilted 45°, √2 times that.
        #expect(abs(area(l, triangles) - tilted) < 1e-3)
        // Clockwise, and closed by a repeated first vertex: the same fill.
        let reversed = Array(l.reversed()) + [l.last!]
        let again = ReferenceStyle.triangulate(reversed)
        #expect(again.count == 12)
        #expect(abs(area(reversed, again) - tilted) < 1e-3)
        // A vertical face, as a parapet's top might be.
        let wall: [SIMD3<Float>] = [SIMD3(0, 0, 0), SIMD3(3, 0, 0), SIMD3(3, 0, 1), SIMD3(0, 0, 1)]
        #expect(abs(area(wall, ReferenceStyle.triangulate(wall)) - 3) < 1e-4)
        // Too few vertices, or none apart.
        #expect(ReferenceStyle.triangulate([SIMD3(0, 0, 0), SIMD3(1, 0, 0)]).isEmpty)
        #expect(
            ReferenceStyle.triangulate([SIMD3(0, 0, 0), SIMD3(1, 0, 0), SIMD3(2, 0, 0)]).isEmpty)
    }

    @Test func roofsAreColouredBySlopeOrByTheirPartsShape() {
        let flat = ReferenceStyle.roofSlopeColour(0)
        let steep = ReferenceStyle.roofSlopeColour(60)
        #expect(flat.z > flat.x)  // blue
        #expect(steep.x > steep.z && steep.x > steep.y)  // red
        #expect(ReferenceStyle.roofSlopeColour(45) == steep)
        #expect(ReferenceStyle.roofSlopeColour(nil) == ReferenceStyle.lineColour(.roofPlane))
        let parts = ["gms:part/main": mainPart.tags]
        let face = roofFaces[0]
        #expect(
            ReferenceStyle.roofColour(face, parts: parts, colouring: .shape)
                == ReferenceStyle.roofShapeColours["gabled"])
        #expect(
            ReferenceStyle.roofColour(face, parts: parts, colouring: .slope)
                == ReferenceStyle.roofSlopeColour(21.8))
        // A face whose part is not drawn, or has no known shape, is grey.
        let grey = ReferenceStyle.roofShapeColour(nil)
        #expect(ReferenceStyle.roofColour(face, parts: [:], colouring: .shape) == grey)
        #expect(ReferenceStyle.roofShapeColour("unknown") == grey)
        let shapes = Array(ReferenceStyle.roofShapeColours.values) + [grey]
        #expect(Set(shapes.map { [$0.x, $0.y, $0.z] }).count == shapes.count)
    }
}

// MARK: - Markings, furniture and trees

struct StreetscapeStyleTests {
    @Test func markingsAreBrightLinesOnTheRoad() {
        let g = geometry([.laneMarkings])
        let v = ReferenceStyle.laneMarkingVertices(g.laneMarkings)
        // The centre line's two segments and the crosswalk's four, closed.
        #expect(v.count / 7 == 12)
        #expect(points(v).allSatisfy { $0.z == 21 + ReferenceStyle.lineLift })
        let yellow = ReferenceStyle.laneMarkingColour(g.laneMarkings[0])
        let white = ReferenceStyle.laneMarkingColour(g.laneMarkings[1])
        #expect(yellow != white)
        #expect(yellow.x == 1 && yellow.z == 0)
        #expect(white == SIMD4(1, 1, 1, 1))
        // Bright and opaque, whatever the colour.
        for c in [yellow, white, ReferenceStyle.lineColour(.laneMarking)] {
            #expect(c.w == 1 && max(c.x, c.y, c.z) == 1)
        }
    }

    @Test func aPoleIsALineWithARingOfItsDiameterAtEachEnd() {
        let pole = geometry([.streetFurniture]).streetFurniture[0]
        let v = ReferenceStyle.streetFurnitureVertices([pole])
        let n = ReferenceStyle.ringSegments
        let ringed: Int = 2 + 4 * n
        #expect(v.count / 7 == ringed)
        let rings = Array(points(v).dropFirst(2))
        let radii = rings.map { offAxis($0) }
        #expect(radii.allSatisfy { abs($0 - 0.15) < 1e-5 })
        let heights = Set(rings.map(\.z))
        #expect(heights == [21, 27])
        // Without a diameter: 0.2 m.
        var bare = pole
        bare.tags = [:]
        let r = points(ReferenceStyle.streetFurnitureVertices([bare]))[2]
        let radius = offAxis(r)
        #expect(abs(radius - 0.1) < 1e-5)
    }

    @Test func aSignIsAShortThickSegment() {
        let sign = geometry([.streetFurniture]).streetFurniture[1]
        let v = ReferenceStyle.streetFurnitureVertices([sign])
        // The face and four copies set off around it.
        #expect(v.count / 7 == 10)
        #expect(Set(points(v).map(\.z)) == [23.5, 24.3])
        #expect(colours(v).allSatisfy { $0 == ReferenceStyle.lineColour(.sign) })
        let xs = points(v).map(\.x)
        let spread: Float = (xs.max() ?? 0) - (xs.min() ?? 0)
        let thick: Float = 2 * ReferenceStyle.signThicknessM
        #expect(abs(spread - thick) < 1e-5)
    }

    @Test func aTreeIsATrunkAndTwoCrownRings() throws {
        let tree = geometry([.trees]).trees[0]
        let crown = try #require(ReferenceStyle.crown(tree))
        #expect(crown.centre == SIMD2(5, -3))
        #expect(crown.radius == 3)
        #expect(crown.bottom == 23.5)
        #expect(crown.top == 30)
        let v = ReferenceStyle.treeVertices([tree])
        let n = ReferenceStyle.ringSegments
        let ringed: Int = 2 + 4 * n
        #expect(v.count / 7 == ringed)
        #expect(colours(v).prefix(2).allSatisfy { $0 == ReferenceStyle.treeTrunkColour })
        #expect(Set(points(v).dropFirst(2).map(\.z)) == [23.5, 30])
        // Without tags: the crown starts at the trunk's top, 2 m wide each
        // way and as tall as it is wide.
        var bare = tree
        bare.tags = [:]
        let guess = try #require(ReferenceStyle.crown(bare))
        #expect(guess.radius == ReferenceStyle.defaultCrownRadiusM)
        #expect(guess.bottom == 23.5)
        let guessedTop: Float = 23.5 + 2 * ReferenceStyle.defaultCrownRadiusM
        #expect(guess.top == guessedTop)
    }

    @Test func everyLayerReachesTheOverlay() {
        let g = geometry([.buildings, .laneMarkings, .streetFurniture, .trees])
        let v = ReferenceStyle.vertices(for: g)
        // Two prisms of 24 vertices, and the two faces' outlines.
        #expect(v.buildingVertexCount == 64)
        #expect(v.roofFillVertexCount == 12)
        #expect(v.laneMarkingVertexCount == 12)
        let n = ReferenceStyle.ringSegments
        let ringed: Int = 2 + 4 * n
        #expect(v.streetFurnitureVertexCount == ringed + 10)
        #expect(v.treeVertexCount == ringed)
        // Shape colouring changes the roofs' colours and nothing else.
        let shaped = ReferenceStyle.vertices(for: g, roofColouring: .shape)
        #expect(shaped.roofFills != v.roofFills)
        #expect(shaped.roofFillVertexCount == v.roofFillVertexCount)
        #expect(shaped.treeLines == v.treeLines)
    }
}

// MARK: - The renderer

@MainActor struct FeatureLayerRendererTests {
    @Test func eachNewLayerHasABufferAndAToggle() {
        guard let device = MTLCreateSystemDefaultDevice() else { return }
        let view = MTKView()
        view.device = device
        guard let renderer = MetalRenderer(metalView: view) else { return }
        let g = geometry([.buildings, .laneMarkings, .streetFurniture, .trees])
        renderer.setReferenceOverlay(ReferenceStyle.vertices(for: g))
        #expect(renderer.referenceBuildingVertexCount == 64)
        #expect(renderer.referenceRoofFillVertexCount == 12)
        #expect(renderer.referenceLaneMarkingVertexCount == 12)
        #expect(renderer.referenceStreetFurnitureVertexCount > 0)
        #expect(renderer.referenceTreeVertexCount > 0)
        var shown = ReferenceVisibility()
        #expect(shown.buildings && shown.laneMarkings && shown.streetFurniture && shown.trees)
        shown.buildings = false
        shown.trees = false
        renderer.setReferenceVisibility(shown)
        #expect(renderer.referenceVisibility == shown)
        renderer.setReferenceOverlay(nil)
        #expect(renderer.referenceBuildingVertexCount == 0)
        #expect(renderer.referenceRoofFillVertexCount == 0)
        #expect(renderer.referenceLaneMarkingVertexCount == 0)
        #expect(renderer.referenceStreetFurnitureVertexCount == 0)
        #expect(renderer.referenceTreeVertexCount == 0)
    }
}

// MARK: - The session

@MainActor struct FeatureLayerSessionTests {
    private let site = AlignmentSite(
        clipID: "6451411d-81c3-47f3-a5ff-5ce37b3ab768", l16Token: "808580f47", l13Token: "808580f4",
        description: "Columbus at Broadway", sensorID: "hesai-pandar40p", latitude: 37.7978,
        longitude: -122.4065)

    private func placed(
        _ configure: (FakeAlignmentService) -> Void = { _ in }
    ) async -> (AlignmentSession, FakeAlignmentService, FakeOverlaySink) {
        let service = FakeAlignmentService()
        configure(service)
        let sink = FakeOverlaySink()
        let session = AlignmentSession(makeClient: { _ in service }, lookup: nil, defaults: nil)
        session.sink = sink
        await session.probe()
        session.choose(site)
        session.northAzimuthText = "35"
        await session.placeFromHint()
        return (session, service, sink)
    }

    @Test func listedLayersAreStreamedEachOnItsOwnCall() async throws {
        let (session, service, sink) = await placed {
            $0.availableLayers += AlignmentSession.featureLayers
        }
        let asked = service.references.map(\.layers).filter {
            $0.contains(where: AlignmentSession.featureLayers.contains)
        }
        #expect(asked == AlignmentSession.featureLayers.map { [$0] })
        #expect(service.references.allSatisfy { $0.radiusM == 150 })
        // The bundle's call does not carry them.
        let bundle = service.references.first?.layers ?? []
        let carried = bundle.filter { AlignmentSession.featureLayers.contains($0) }
        #expect(!bundle.isEmpty)
        #expect(carried.isEmpty)

        let summary = try #require(session.reference)
        #expect(summary.featureLayersServed == Set(AlignmentSession.featureLayers))
        #expect(summary.buildingParts == 2)
        #expect(summary.roofPlanes == 2)
        #expect(summary.laneMarkings == 2)
        #expect(summary.poles == 1)
        #expect(summary.signs == 1)
        #expect(summary.trees == 1)
        // The site model's id is still the registered layers'.
        #expect(summary.modelID == "sha256:model-a")
        #expect(session.featureLayersNote == nil)

        let overlay = try #require(sink.overlays.last ?? nil)
        #expect(overlay.buildingVertexCount == 64)
        #expect(overlay.roofFillVertexCount == 12)

        // Each layer has its own toggle.
        session.showBuildings = false
        #expect(sink.visibility?.buildings == false)
        #expect(sink.visibility?.laneMarkings == true)
        session.showLaneMarkings = false
        session.showStreetFurniture = false
        session.showTrees = false
        #expect(sink.visibility?.laneMarkings == false)
        #expect(sink.visibility?.streetFurniture == false)
        #expect(sink.visibility?.trees == false)

        // Colouring roofs by shape rebuilds the overlay.
        let before = sink.overlays.count
        session.roofColouring = .shape
        await session.pendingStyling?.value
        #expect(sink.overlays.count == before + 1)
        #expect(sink.overlays.last??.roofFills != overlay.roofFills)
    }

    @Test func onlyTheListedLayersAreAskedFor() async {
        let (session, service, _) = await placed { $0.availableLayers += [.trees] }
        let asked = service.references.map(\.layers).filter {
            $0.contains(where: AlignmentSession.featureLayers.contains)
        }
        #expect(asked == [[.trees]])
        #expect(session.reference?.featureLayersServed == [.trees])
        #expect(session.reference?.trees == 1)
        #expect(session.reference?.buildingParts == 0)
    }

    @Test func aServerThatListsNoneIsNotAskedForThem() async {
        let (session, service, _) = await placed()
        #expect(
            !service.references.contains {
                $0.layers.contains(where: AlignmentSession.featureLayers.contains)
            })
        #expect(session.reference?.featureLayersServed.isEmpty == true)
        // A server that lists no layers at all is asked for none of them
        // either: they are never assumed.
        let bare = FakeAlignmentService()
        bare.availableLayers = []
        let quiet = AlignmentSession(makeClient: { _ in bare }, lookup: nil, defaults: nil)
        await quiet.probe()
        #expect(quiet.featureReferenceRequests(l16Token: "808580f47", centre: .zero).isEmpty)
    }

    @Test func aFailingLayerLeavesTheOthersDrawnAndIsNamed() async {
        let (session, _, _) = await placed {
            $0.availableLayers += AlignmentSession.featureLayers
            $0.refusedLayers = [.trees]
        }
        #expect(session.reference?.trees == 0)
        #expect(session.reference?.buildingParts == 2)
        #expect(session.reference?.laneMarkings == 2)
        #expect(session.featureLayersNote?.hasPrefix("Trees not drawn") == true)
        #expect(session.errorMessage == nil)
    }
}
