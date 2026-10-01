//
//  AnnotationSaveKeyTests.swift
//  VelocityVisualiserTests
//
//  S saves and X saves and steps, in both modes; the keys are free app-wide;
//  the buttons underline the letter; and the intensity pin is on a key the
//  app's menus do not already own.
//

import AppKit
import Foundation
import Testing
import simd

@testable import VelocityVisualiser

/// A source file with its whitespace removed, so a check survives the
/// formatter wrapping a call.
private func source(_ relative: String) throws -> String {
    try String(
        contentsOf: URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().appendingPathComponent("VelocityVisualiser/\(relative)"),
        encoding: .utf8
    ).filter { !$0.isWhitespace }
}

struct AnnotationSaveKeyBindingTests {
    @Test func sAndXAreBoundInTheWindowAndNowhereElse() throws {
        let window = try source("UI/AnnotationWindow.swift")
        #expect(window.contains(#".keyboardShortcut("s",modifiers:[])"#))
        #expect(window.contains(#".keyboardShortcut("x",modifiers:[])"#))
        let app = try source("App/VelocityVisualiserApp.swift")
        for key in ["s", "x", "m", "n"] {
            #expect(
                !app.contains(#"keyboardShortcut("\#(key)",modifiers:[])"#),
                "the app menu owns \(key)")
        }
    }

    @Test func theButtonsUnderlineTheirKey() throws {
        #expect(shortcutSplit("Save points", key: "s")! == ("", "S", "ave points"))
        #expect(shortcutSplit("Save and next", key: "x")! == ("Save and ne", "x", "t"))
        #expect(shortcutSplit("Save proposal", key: "s")! == ("", "S", "ave proposal"))
        #expect(shortcutSplit("Review", key: "x") == nil)
        let pane = try source("UI/AnnotationPane.swift")
        let physical = try source("UI/PhysicalReferencePane.swift")
        #expect(pane.contains(#"shortcutLabel("Savepoints",key:"s")"#))
        #expect(pane.contains(#"shortcutLabel("Saveandnext",key:"x")"#))
        #expect(physical.contains(#"shortcutLabel("Saveproposal",key:"s")"#))
        #expect(physical.contains(#"shortcutLabel("Saveandnext",key:"x")"#))
    }

    @Test func thePinIsMBesideNextNotTheOverlaysMenusP() throws {
        func key(_ code: UInt16, _ flags: NSEvent.ModifierFlags = []) -> ViewportKey? {
            NSEvent.keyEvent(
                with: .keyDown, location: .zero, modifierFlags: flags, timestamp: 0,
                windowNumber: 0, context: nil, characters: "", charactersIgnoringModifiers: "",
                isARepeat: false, keyCode: code
            ).flatMap { ViewportInputView.viewportKey(for: $0) }
        }
        #expect(key(46) == .inspectPin)
        #expect(key(45) == .inspectNext)
        #expect(key(35) == nil, "P belongs to the Overlays menu")
        #expect(key(46, .command) == nil)
        #expect(ViewportKey.inspectPin.meaning(carrying: false) == .pass)
        #expect(ViewportKey.inspectNext.meaning(carrying: true) == .pass)
    }
}

@MainActor struct AnnotationSaveKeyTests {
    private func open() throws -> (AnnotationSession, FakePhysicalService) {
        let dir = try SyntheticPack.write([
            SyntheticPack.car(at: simd_float2(5, 0)), SyntheticPack.car(at: simd_float2(6, 0)),
            SyntheticPack.car(at: simd_float2(7, 0)),
        ])
        let (urlSession, baseURL, register) = AnnotationMockURLProtocol.makeSession()
        let fake = FakePhysicalService(packDir: dir.resolvingSymlinksInPath().path)
        register { try fake.handle($0) }
        let session = try AnnotationSession(
            pack: try AnnotationPack.open(directory: dir),
            physicalClient: PhysicalReferenceAPIClient(baseURL: baseURL, session: urlSession))
        session.operatorName = "op"
        return (session, fake)
    }

    @Test func inPointsModeSSavesAndXSavesAndSteps() async throws {
        let (session, _) = try open()
        _ = session.createObject(objectClass: "car")
        #expect(await session.saveCurrent(advance: false))
        #expect(session.sampleIndex == 0)
        #expect(await session.saveCurrent(advance: true))
        #expect(session.sampleIndex == 1)
        #expect(session.sidecar.masks.contains { $0.sampleID == 0 })
    }

    @Test func aRefusedSaveDoesNotStep() async throws {
        let (session, _) = try open()
        // No object: nothing to save the points to.
        #expect(!(await session.saveCurrent(advance: true)))
        #expect(session.sampleIndex == 0)
    }

    @Test func xOnTheLastFrameSavesAndSaysItCouldNotStep() async throws {
        let (session, _) = try open()
        _ = session.createObject(objectClass: "car")
        #expect(session.step(to: 2) == nil)
        #expect(!(await session.saveCurrent(advance: true)))
        #expect(session.sidecar.masks.contains { $0.sampleID == session.samples[2].sampleID })
    }

    @Test func inPhysicalModeXSavesTheDraftThenSteps() async throws {
        let (session, fake) = try open()
        session.workMode = .physical
        for _ in 0..<200 where session.physical.availability != .ready {
            try? await Task.sleep(for: .milliseconds(10))
        }
        let object = session.createObject(objectClass: "car")
        // Nothing unsaved: X is only a step, and asks the service nothing.
        #expect(await session.saveCurrent(advance: true))
        #expect(session.sampleIndex == 1)
        #expect(fake.last("/api/annotations/physical/save") == nil)

        session.physical.edit {
            PhysicalDraft.addBody(objectID: object.objectID, author: "op", session: "s", to: &$0)
        }
        #expect(await session.saveCurrent(advance: true))
        #expect(fake.last("/api/annotations/physical/save") != nil)
        #expect(!session.physical.isDirty && session.sampleIndex == 2)

        // A refused save keeps the frame and the draft.
        session.physical.edit { PhysicalDraft.removeBody(objectID: object.objectID, from: &$0) }
        fake.override["/api/annotations/physical/save"] = (
            409, Data(#"{"error":"changed","code":"conflict"}"#.utf8)
        )
        #expect(session.step(to: 0) != nil, "a dirty draft let the frame change")
        #expect(!(await session.saveCurrent(advance: false)))
        #expect(session.physical.isDirty && session.sampleIndex == 2)
    }
}
