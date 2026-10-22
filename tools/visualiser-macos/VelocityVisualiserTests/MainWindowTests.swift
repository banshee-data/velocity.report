//
//  MainWindowTests.swift
//  VelocityVisualiserTests
//
//  The main window can always be brought back. The Window menu has an item
//  for it whether or not it is open; choosing the item reopens a closed main
//  window and brings an open one forward without opening a second. The About
//  and Annotation windows are not restored at launch, since a restored one
//  kept the main window from opening. The test host is the app, so these
//  drive its real menu and scenes.
//

import AppKit
import Testing

@testable import VelocityVisualiser

@MainActor @Suite(.serialized) struct MainWindowTests {
    private let title = VelocityVisualiserApp.mainWindowTitle

    private func windows(titled title: String) -> [NSWindow] {
        NSApp.windows.filter { $0.title == title && $0.isVisible }
    }

    private func choose(_ item: String) throws {
        let menu = try #require(NSApp.windowsMenu)
        let index = try #require(menu.items.firstIndex { $0.title == item })
        menu.performActionForItem(at: index)
    }

    /// Opening and closing a SwiftUI window completes on a later turn of the
    /// run loop, so this waits for the visible windows with the title to
    /// number `count`, and reports whether they did.
    private func settle(_ title: String, at count: Int) async -> Bool {
        for _ in 0..<150 where windows(titled: title).count != count {
            try? await Task.sleep(for: .milliseconds(20))
        }
        return windows(titled: title).count == count
    }

    @Test func windowMenuReopensAClosedMainWindow() async throws {
        try choose(title)
        #expect(await settle(title, at: 1))

        windows(titled: title).forEach { $0.performClose(nil) }
        #expect(await settle(title, at: 0))
        #expect(NSApp.windowsMenu?.items.contains { $0.title == title } == true)

        try choose(title)
        #expect(await settle(title, at: 1))
        #expect(windows(titled: title).first?.isRestorable == true)

        // Chosen again, the item brings the window forward: no second one.
        try choose(title)
        try await Task.sleep(for: .milliseconds(300))
        #expect(windows(titled: title).count == 1)
    }

    @Test(arguments: ["About VelocityVisualiser.app", "Annotation"])
    func auxiliaryWindowIsNotRestored(_ item: String) async throws {
        try choose(item)
        #expect(await settle(item, at: 1))
        let window = try #require(windows(titled: item).first)
        #expect(!window.isRestorable)
        window.close()
        #expect(await settle(item, at: 0))
    }
}
