import Foundation

/// Presentation only. Never use this text to decide whether a record is valid,
/// identify a repair target, or change the service's original diagnostic.
enum PhysicalAuthoringMessage {
    static func describe(
        _ raw: String, objects: [PhysicalObject], name: (String) -> String,
        frameNumber: (Int) -> Int?
    ) -> String {
        var result = raw
        for object in objects {
            result = result.replacingOccurrences(
                of: "object \"\(object.objectID)\"", with: name(object.objectID))
            if let body = object.body {
                result = result.replacingOccurrences(
                    of: "body \"\(body.bodyID)\"", with: "object size")
                if result == "body/" + body.bodyID {
                    result = name(object.objectID) + " · object size"
                }
            }
            for pose in object.keyframes {
                let location =
                    frameNumber(pose.sampleID).map { "frame \($0) (sample \(pose.sampleID))" }
                    ?? "sample \(pose.sampleID)"
                result = result.replacingOccurrences(
                    of: "keyframe \"\(pose.keyframeID)\"", with: "pose at " + location)
                if result == "keyframe/" + pose.keyframeID {
                    result = name(object.objectID) + " · pose at " + location
                }
            }
        }
        // Translate only known exact clauses. New service diagnostics remain
        // available verbatim and do not acquire an invented recovery action.
        let clauses: [(String, String)] = [
            (
                "a known dimension needs a finite, non-negative lower_m",
                "enter a finite lower bound of at least 0 m, or choose Unknown"
            ),
            (
                "a full span needs a finite upper_m no less than lower_m",
                "enter a finite upper bound at least as large as the lower bound"
            ),
            (
                "value_m must lie within [lower_m, upper_m]",
                "the representative size must lie between its lower and upper bounds"
            ),
            (
                "a partial span supports only a lower bound: it cannot carry upper_m or value_m",
                "a partial view states only an at-least size; remove the upper bound and representative value"
            ),
            (
                "a known position needs a finite, non-negative bound_m",
                "enter a finite position uncertainty of at least 0 m"
            ),
            (
                "an observed component names no supporting frames",
                "cite the frame where this component was observed"
            ),
            (
                "an unknown dimension carries no span or value",
                "choose Unknown without a size, span or bounds"
            ),
            (
                "an inferred component names neither supporting frames nor an external reference",
                "cite the frames used for this inference or name an external reference"
            ),
            (
                "a prior-only component must name its prior as an external reference",
                "name the class or external size prior; this remains prior-only and unscored"
            ),
            (
                "a known position needs finite x_m and y_m",
                "enter finite X and Y coordinates, or choose Unknown"
            ), ("review names no method", "name the measurement or authoring method before saving"),
            ("review names no author", "enter Labelled by before saving"),
        ]
        for (service, human) in clauses {
            result = result.replacingOccurrences(of: service, with: human)
        }
        return result
    }
}
