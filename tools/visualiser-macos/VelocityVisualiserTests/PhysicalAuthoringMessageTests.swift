import Foundation
import Testing

@testable import VelocityVisualiser

struct PhysicalAuthoringMessageTests {
    private var objects: [PhysicalObject] {
        [
            PhysicalObject(
                objectID: "obj_car", body: PhysicalBody(bodyID: "body_1"),
                keyframes: [PhysicalKeyframe(keyframeID: "pose_1", sampleID: 295, timestampNs: 123)]
            )
        ]
    }
    private func describe(_ raw: String) -> String {
        PhysicalAuthoringMessage.describe(
            raw, objects: objects, name: { _ in "car 5" }, frameNumber: { $0 == 295 ? 297 : nil })
    }
    @Test func objectSizeErrorsNameTheFieldAndKeepTheOriginalUntouched() {
        let raw =
            "object \"obj_car\": body \"body_1\": length: a known dimension needs a finite, non-negative lower_m"
        #expect(
            describe(raw)
                == "car 5: object size: length: enter a finite lower bound of at least 0 m, or choose Unknown"
        )
        #expect(raw.contains("lower_m"))
        #expect(describe("body/body_1") == "car 5 · object size")
    }
    @Test func poseLabelsKeepFrameAndSampleDistinctAndNeverGuessUnknownRecords() {
        let raw =
            "object \"obj_car\": keyframe \"pose_1\": position: a known position needs a finite, non-negative bound_m"
        #expect(
            describe(raw)
                == "car 5: pose at frame 297 (sample 295): position: enter a finite position uncertainty of at least 0 m"
        )
        #expect(describe("keyframe/pose_1") == "car 5 · pose at frame 297 (sample 295)")
        let unknown = "object \"obj_other\": keyframe \"pose_other\": future constraint failed"
        #expect(describe(unknown) == unknown)
        #expect(describe("body/body_10") == "body/body_10", "matched an ID prefix")
        let fallback = PhysicalAuthoringMessage.describe(
            "keyframe/pose_1", objects: objects, name: { _ in "car 5" }, frameNumber: { _ in nil })
        #expect(fallback == "car 5 · pose at sample 295")
    }
    @Test func partialSpansAndObservedSupportOfferOnlyKnownRecoveryInstructions() {
        #expect(
            describe(
                "width: a partial span supports only a lower bound: it cannot carry upper_m or value_m"
            )
                == "width: a partial view states only an at-least size; remove the upper bound and representative value"
        )
        #expect(
            describe("yaw: an observed component names no supporting frames")
                == "yaw: cite the frame where this component was observed")
        #expect(
            describe("length: a prior-only component must name its prior as an external reference")
                == "length: name the class or external size prior; this remains prior-only and unscored"
        )
        #expect(describe("new_component: bound_m invalid") == "new_component: bound_m invalid")
    }
}
