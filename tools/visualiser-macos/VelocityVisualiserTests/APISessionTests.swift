//
//  APISessionTests.swift
//  VelocityVisualiserTests
//
//  The API clients must not use Foundation's on-disk URL cache.
//

import Foundation
import Testing

@testable import VelocityVisualiser

// MARK: - APISession

struct APISessionTests {

    /// URLSession.shared writes an on-disk cache to
    /// Library/Caches/<bundle-id>/Cache.db. That database is the one behind the
    /// `execSQLStatement ... disk I/O error` messages in the app log: the
    /// failures are Foundation's cache, not the visualiser's own storage.
    @Test func sessionKeepsNoOnDiskCache() throws {
        let session = APISession.makeUncached()
        #expect(session.configuration.urlCache == nil)
    }

    /// Even if a cache were attached by the environment, responses must not be
    /// served from it. A cached GET against the labelling API can return a label
    /// set that has already been edited.
    @Test func sessionAlwaysRevalidates() throws {
        let session = APISession.makeUncached()
        #expect(session.configuration.requestCachePolicy == .reloadIgnoringLocalCacheData)
    }

    /// Ephemeral configurations keep credentials in memory rather than in the
    /// shared on-disk store, which is what makes this safe as the app-wide
    /// default. (The storage is non-nil — it is an in-memory instance — so the
    /// assertion is about which store it is, not whether one exists.)
    @Test func sessionKeepsCredentialsOutOfSharedStorage() throws {
        let session = APISession.makeUncached()
        #expect(session.configuration.urlCredentialStorage !== URLCredentialStorage.shared)
    }

    @Test func sharedSessionUsesTheSamePolicy() throws {
        #expect(APISession.shared.configuration.urlCache == nil)
        #expect(APISession.shared.configuration.requestCachePolicy == .reloadIgnoringLocalCacheData)
    }

    /// The default must not be URLSession.shared, whose cache is the whole point
    /// of this type.
    @Test func sharedSessionIsNotTheFoundationShared() throws {
        #expect(APISession.shared !== URLSession.shared)
        #expect(URLSession.shared.configuration.urlCache != nil)
    }
}

// MARK: - Client defaults

struct APIClientCacheDefaultsTests {

    /// Both clients default to the uncached session. Constructed with the
    /// default argument they must not be reaching Foundation's shared cache.
    @Test func labelClientDefaultsToUncachedSession() throws {
        let client = LabelAPIClient()
        #expect(client.sessionForTesting.configuration.urlCache == nil)
    }

    @Test func runTrackClientDefaultsToUncachedSession() throws {
        let client = RunTrackLabelAPIClient()
        #expect(client.sessionForTesting.configuration.urlCache == nil)
    }

    /// An injected session is still honoured, so tests and callers can supply
    /// their own transport.
    @Test func injectedSessionIsUsed() throws {
        let custom = URLSession(configuration: .ephemeral)
        let client = LabelAPIClient(session: custom)
        #expect(client.sessionForTesting === custom)
    }
}
