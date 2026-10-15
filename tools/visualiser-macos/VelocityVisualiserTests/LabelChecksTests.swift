//
//  LabelChecksTests.swift
//  VelocityVisualiserTests
//
//  The window's label checks against Go's: the fixture Go's
//  TestLabelChecksFixture writes holds a membership that trips every rule and
//  the warnings Go gives for it, and the port must give the same, message for
//  message.
//

import Foundation
import Testing

@testable import VelocityVisualiser

/// The repository root, from this file's own path.
private let repositoryRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

private struct LabelChecksFixture: Decodable {
    struct Input: Decodable {
        var samples: [LabelCheckSample]
        var objects: [AnnotationObject]
        var masks: [FrameMask]
    }
    var input: Input
    var warnings: [LabelWarning]
}

struct LabelChecksTests {
    private func fixture() throws -> LabelChecksFixture {
        try JSONDecoder().decode(
            LabelChecksFixture.self,
            from: Data(
                contentsOf: repositoryRoot.appendingPathComponent(
                    "internal/lidar/annotation/testdata/label_checks_fixture.json")))
    }

    @Test func theChecksGiveGosWarnings() throws {
        let f = try fixture()
        let got = LabelChecks.run(samples: f.input.samples, objects: f.input.objects, masks: f.input.masks)
        #expect(got == f.warnings)
        #expect(Set(got.map(\.rule)).count == 6, "every rule is exercised")
    }

    @Test func aCleanMembershipWarnsOfNothing() throws {
        let f = try fixture()
        let clean = f.input.masks.filter { $0.objectID == "car_f" }
        #expect(
            LabelChecks.run(samples: f.input.samples, objects: f.input.objects, masks: clean).isEmpty)
    }
}
