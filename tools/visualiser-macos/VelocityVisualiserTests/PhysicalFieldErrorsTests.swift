//
//  PhysicalFieldErrorsTests.swift
//  VelocityVisualiserTests
//
//  A service refusal lands beside the control that sets the field it names,
//  read from the location Go writes, never from the message text; anything
//  without a control on screen stays at Save in full. The cases are Go's own
//  refusals (TestPhysicalLocationSwiftFixture), so a renamed location fails
//  here rather than silently falling back to the bottom of the pane.
//

import Foundation
import Testing

@testable import VelocityVisualiser

private let repositoryRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

private struct LocationCase: Decodable {
    var name: String
    var objectID: String
    var sampleID: Int
    var result: PhysicalEditResult

    enum CodingKeys: String, CodingKey {
        case name
        case objectID = "object_id"
        case sampleID = "sample_id"
        case result
    }
}

private func locationCases() throws -> [String: LocationCase] {
    let data = try Data(
        contentsOf: repositoryRoot.appendingPathComponent(
            "internal/lidar/annotation/testdata/physical_location_fixture.json"))
    let cases = try JSONDecoder().decode([LocationCase].self, from: data)
    return Dictionary(uniqueKeysWithValues: cases.map { ($0.name, $0) })
}

struct PhysicalFieldErrorsTests {
    /// Where each of Go's refusals lands with its object and frame on screen.
    @Test func everyGoRefusalLandsAtItsControl() throws {
        let cases = try locationCases()
        let expected: [String: PhysicalPaneField] = [
            // The pilot's two stops.
            "position bound": .position, "pose assumptions": .poseReview,
            "size assumptions": .sizeReview,
            // The rest of the controls.
            "length": .length, "width": .width, "height": .height, "axis convention": .size,
            "heading": .heading, "anchor": .anchor, "front": .front, "rear": .rear,
            "shared errors": .sharedErrors, "capture time": .pose,
            "length beyond the returns": .length,
        ]
        for (name, field) in expected {
            let c = try #require(cases[name], "Go's fixture has no case \(name)")
            let errors = PhysicalFieldErrors.place(
                c.result, objectID: c.objectID, sampleID: c.sampleID)
            #expect(errors.placedFields == [field], "\(name)")
            #expect(errors.elsewhere.isEmpty, "\(name) is also listed at Save")
            let refusal = try #require(errors.at(field).first)
            // Beside the field, the location prefix is dropped; the whole
            // message is kept for the service detail.
            #expect(!refusal.detail.contains("object \""), "\(name): \(refusal.detail)")
            #expect(refusal.raw.hasSuffix(refusal.detail), "\(name)")
        }
        #expect(
            Set(expected.keys).union(["pose at another frame", "following", "document"])
                == Set(cases.keys), "every Go case is checked")
    }

    /// The pilot's second stop: an empty ± after a position, which shows
    /// only "m". The refusal now says what is missing, at that row, and
    /// holds Adjust open so the row is in view.
    @Test func aMissingPositionBoundOpensAdjustAndNamesTheField() throws {
        let c = try #require(try locationCases()["position bound"])
        let errors = PhysicalFieldErrors.place(c.result, objectID: "car-1", sampleID: 0)
        #expect(errors.needsAdjust)
        #expect(errors.at(.position).first?.location?.field == "position.bound_m")
        let human = PhysicalAuthoringMessage.describe(
            errors.at(.position)[0].detail, objects: [], name: { $0 })
        #expect(human == "enter a finite position uncertainty of at least 0 m")
    }

    /// A refusal with no control on screen stays at Save, whole.
    @Test func whatHasNoControlOnScreenStaysAtSave() throws {
        let cases = try locationCases()
        for name in ["following", "document"] {
            let c = try #require(cases[name])
            let errors = PhysicalFieldErrors.place(c.result, objectID: "car-1", sampleID: 0)
            #expect(errors.placedFields.isEmpty, "\(name)")
            #expect(errors.elsewhere.map(\.raw) == [c.result.invalid], "\(name)")
        }
        // A pose at another frame is not on screen; at its own frame it is.
        let other = try #require(cases["pose at another frame"])
        #expect(
            PhysicalFieldErrors.place(other.result, objectID: "car-1", sampleID: 3).elsewhere.count
                == 1)
        #expect(
            PhysicalFieldErrors.place(other.result, objectID: "car-1", sampleID: 0).placedFields
                == [.position])
        // Another object's size is not on screen either.
        let size = try #require(cases["length"])
        let elsewhere = PhysicalFieldErrors.place(size.result, objectID: "car-2", sampleID: 0)
        #expect(elsewhere.placedFields.isEmpty && elsewhere.elsewhere.count == 1)
        #expect(
            PhysicalFieldErrors.place(size.result, objectID: nil, sampleID: nil).elsewhere.count
                == 1)
    }

    /// A service that predates locations still reports every refusal, at
    /// Save, as the pane did before.
    @Test func anUnlocatedRefusalFallsBackToSave() throws {
        let result = PhysicalEditResult(
            valid: false,
            invalid:
                "object \"car-1\": keyframe \"k\": position: a known position needs a finite, non-negative bound_m",
            linkProblems: [PhysicalLinkProblem(record: "object \"car-1\"", problem: "rejected")],
            resetReviews: [])
        let errors = PhysicalFieldErrors.place(result, objectID: "car-1", sampleID: 0)
        #expect(errors.placedFields.isEmpty)
        #expect(errors.elsewhere.map(\.raw) == [result.invalid!, "object \"car-1\": rejected"])
        #expect(!errors.needsAdjust)
    }

    /// A valid draft, or none, places nothing.
    @Test func aValidDraftPlacesNothing() {
        #expect(PhysicalFieldErrors.place(nil, objectID: "car-1", sampleID: 0).isEmpty)
        let valid = PhysicalEditResult(
            valid: true, invalid: nil, linkProblems: [], resetReviews: [])
        #expect(PhysicalFieldErrors.place(valid, objectID: "car-1", sampleID: 0).isEmpty)
    }

    /// The mapping reads the field's first component and the record kind,
    /// and nothing else.
    @Test func theMappingReadsOnlyTheLocation() {
        func at(_ record: String?, _ field: String?) -> PhysicalPaneField? {
            PhysicalPaneField.field(
                for: PhysicalLocation(objectID: "o", record: record, field: field, detail: ""))
        }
        #expect(at("body", "length.support") == .length)
        #expect(at("body", "review.provenance.author") == .sizeReview)
        #expect(at("body", nil) == .size)
        #expect(at("keyframe", "yaw.support") == .heading)
        #expect(at("keyframe", "shared_errors.0.components") == .sharedErrors)
        #expect(at("keyframe", "keyframe_id") == .pose)
        #expect(at("following", "review.method") == nil)
        #expect(at(nil, nil) == nil)
        #expect(PhysicalPaneField.position.isUnderAdjust && !PhysicalPaneField.front.isUnderAdjust)
    }
}
