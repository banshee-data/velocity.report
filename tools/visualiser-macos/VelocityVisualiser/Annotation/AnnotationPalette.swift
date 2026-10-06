// AnnotationPalette.swift
// The object classes an operator can label, and the colour each is drawn in.
//
// One table, read by the SwiftUI canvases directly and mirrored by the point
// shader for the 3D view, so that a car is the same blue in every view. A
// test holds the shader's copy to this one.

import SwiftUI
import simd

/// A class an annotation object can be given.
struct AnnotationClass: Equatable, Identifiable {
    enum Kind: Equatable {
        /// Moves between samples: its mask is carried forward and nudged.
        case moving
        /// Stays put: its mask can be applied to every sample at once.
        case fixed
    }

    let name: String
    let kind: Kind
    /// Index into `AnnotationPalette.colours`.
    let paletteIndex: Int
    /// The main view's track label that covers this class, or nil where it
    /// has none. The two lists differ on purpose: a dataset wants a van told
    /// from a car, and a track label does not. Shown so that an operator who
    /// knows one list can find their way in the other.
    let mainViewLabel: String?

    var id: String { name }
}

enum AnnotationPalette {
    // MARK: Edit states

    /// In the membership but not yet saved.
    static let unsavedIndex = 0
    /// What the stroke or brush in progress would take.
    static let candidateIndex = 1
    /// Saved, and taken out of the membership since.
    static let removedIndex = 2
    /// A class this client does not know.
    static let unknownClassIndex = 13
    /// Settled background that the latest snapshot added or moved.
    static let backgroundChangedIndex = 16

    /// Linear RGB. The order is the shader's order: see PointCloud.metal.
    static let colours: [simd_float3] = [
        simd_float3(1.00, 0.55, 0.10),  // 0 unsaved: orange
        simd_float3(1.00, 0.90, 0.20),  // 1 candidate: yellow
        simd_float3(1.00, 0.20, 0.20),  // 2 removed: red
        simd_float3(0.25, 0.60, 1.00),  // 3 car: blue
        simd_float3(0.20, 0.80, 0.80),  // 4 van: teal
        simd_float3(0.45, 0.45, 1.00),  // 5 truck: indigo
        simd_float3(0.70, 0.45, 1.00),  // 6 bus: purple
        simd_float3(0.40, 0.90, 1.00),  // 7 motorcycle: cyan
        simd_float3(1.00, 0.35, 0.75),  // 8 pedestrian: pink
        simd_float3(0.75, 0.95, 0.20),  // 9 cyclist: lime
        simd_float3(0.85, 0.45, 0.25),  // 10 building: terracotta
        simd_float3(0.90, 0.90, 0.95),  // 11 sign: silver
        simd_float3(0.55, 0.60, 0.15),  // 12 vegetation: olive
        simd_float3(0.60, 0.65, 0.80),  // 13 unknown class: slate
        simd_float3(0.65, 0.40, 0.30),  // 14 ground: umber
        simd_float3(0.50, 0.50, 0.60),  // 15 noise: grey
        simd_float3(1.00, 1.00, 1.00),  // 16 background changed: white
    ]

    /// The first seven are the production labels. The fixed classes are for
    /// reference annotation only: they describe the street, not a road user,
    /// and no production enum value is enabled by choosing one.
    static let classes: [AnnotationClass] = [
        AnnotationClass(name: "car", kind: .moving, paletteIndex: 3, mainViewLabel: "car"),
        AnnotationClass(name: "van", kind: .moving, paletteIndex: 4, mainViewLabel: "car"),
        AnnotationClass(name: "truck", kind: .moving, paletteIndex: 5, mainViewLabel: "car"),
        AnnotationClass(name: "bus", kind: .moving, paletteIndex: 6, mainViewLabel: "bus"),
        AnnotationClass(
            name: "motorcycle", kind: .moving, paletteIndex: 7, mainViewLabel: "cyclist"),
        AnnotationClass(
            name: "pedestrian", kind: .moving, paletteIndex: 8, mainViewLabel: "pedestrian"),
        AnnotationClass(name: "cyclist", kind: .moving, paletteIndex: 9, mainViewLabel: "cyclist"),
        // Foreground that is not a road user at all: returns the background
        // model failed to settle on. Labelling them is how a misclassified
        // road or wall is told from a vehicle that was missed.
        AnnotationClass(name: "ground", kind: .fixed, paletteIndex: 14, mainViewLabel: "noise"),
        AnnotationClass(name: "building", kind: .fixed, paletteIndex: 10, mainViewLabel: "noise"),
        AnnotationClass(name: "sign", kind: .fixed, paletteIndex: 11, mainViewLabel: "noise"),
        AnnotationClass(name: "vegetation", kind: .fixed, paletteIndex: 12, mainViewLabel: "noise"),
        AnnotationClass(name: "noise", kind: .moving, paletteIndex: 15, mainViewLabel: "noise"),
    ]

    static func annotationClass(named name: String) -> AnnotationClass? {
        classes.first { $0.name == name }
    }

    static func paletteIndex(forClass name: String) -> Int {
        annotationClass(named: name)?.paletteIndex ?? unknownClassIndex
    }

    static func isFixed(_ name: String) -> Bool { annotationClass(named: name)?.kind == .fixed }

    static func colour(_ index: Int) -> Color {
        let c = colours[min(max(index, 0), colours.count - 1)]
        return Color(red: Double(c.x), green: Double(c.y), blue: Double(c.z))
    }

    static func colour(forClass name: String) -> Color { colour(paletteIndex(forClass: name)) }

    /// The point class the shader draws as palette entry `index`. Kept clear
    /// of the recorder's own classes (0 to 2) with room to spare.
    static let shaderClassBase: UInt8 = 16
    static func shaderClass(_ index: Int) -> UInt8 { shaderClassBase + UInt8(index) }
}
