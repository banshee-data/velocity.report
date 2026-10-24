//
//  GerardusSettingsTests.swift
//  VelocityVisualiserTests
//
//  Settings ▸ Gerardus: the server's version and git sha from GetServerInfo,
//  whether it serves registered scans, a typed address validated, saved and
//  asked, and the street painter's author kept where the painter reads it.
//

import Foundation
import SwiftUI
import Testing

@testable import VelocityVisualiser

@MainActor struct GerardusSettingsTests {
    private func defaults() -> UserDefaults {
        let name = "GerardusSettingsTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: name)!
        defaults.removePersistentDomain(forName: name)
        return defaults
    }

    @Test func theServerVersionCarriesItsGitSha() {
        let v = GerardusSettings.version("0.1.0+3f2a9c1e0b7d")
        #expect(v.version == "0.1.0" && v.gitSHA == "3f2a9c1e0b7d")
        let dirty = GerardusSettings.version("0.2.0+3f2a9c1e0b7d-dirty")
        #expect(dirty.gitSHA == "3f2a9c1e0b7d-dirty")
        let bare = GerardusSettings.version("0.1.0")
        #expect(bare.version == "0.1.0" && bare.gitSHA == nil)
        #expect(GerardusSettings.version("0.1.0+").gitSHA == nil)
    }

    @Test func servingScansIsShownOnlyWhenReported() {
        var info = Gerardus_Align_V1alpha1_GetServerInfoResponse()
        #expect(GerardusSettings.servesScans(info) == nil)
        info.availableLayers = [.aerialPoints, .observed]
        #expect(GerardusSettings.servesScans(info) == false)
        info.availableLayers.append(.registeredScans)
        #expect(GerardusSettings.servesScans(info) == true)
    }

    @Test func aValidAddressIsSavedAndAskedAndABadOneIsRefused() async throws {
        let store = defaults()
        var asked: [GerardusAddress] = []
        let session = AlignmentSession(
            makeClient: { address in
                asked.append(address)
                return FakeAlignmentService()
            }, lookup: nil, defaults: store)
        #expect(session.serviceAddress == .local)

        let bad = GerardusSettings.apply("http://machine/", to: session)
        guard case .failure(.notAnAddress) = bad else {
            Issue.record("\(bad) is not a refusal")
            return
        }
        #expect(session.serviceAddress == .local)
        #expect(store.string(forKey: GerardusAddress.preferenceKey) == nil)
        guard case .failure(.invalidPort) = GerardusSettings.apply("machine:99999", to: session)
        else {
            Issue.record("a bad port was taken")
            return
        }

        let good = GerardusSettings.apply(" Machine.Tailnet.TS.net:50070 ", to: session)
        let address = try good.get()
        #expect(address.description == "machine.tailnet.ts.net:50070")
        #expect(session.serviceAddress == address)
        #expect(
            store.string(forKey: GerardusAddress.preferenceKey) == "machine.tailnet.ts.net:50070")
        // A new session, as at the next launch, finds it.
        let next = AlignmentSession(
            makeClient: { _ in FakeAlignmentService() }, lookup: nil, defaults: store)
        #expect(next.serviceAddress == address)
        // Changing it asks the new server at once.
        await Task.yield()
        for _ in 0..<100 where session.serverInfo == nil { await Task.yield() }
        #expect(asked.last == address)
        #expect(session.serverInfo?.serverVersion == "0.1.0")
    }

    @Test func theAuthorIsKeptUnderTheKeyThePainterReads() {
        let store = defaults()
        let author = AppStorage(wrappedValue: "", StreetPainterSession.authorKey, store: store)
        author.wrappedValue = "Ada"
        #expect(store.string(forKey: "streets.author") == "Ada")
        let again = AppStorage(wrappedValue: "", StreetPainterSession.authorKey, store: store)
        #expect(again.wrappedValue == "Ada")
    }
}
