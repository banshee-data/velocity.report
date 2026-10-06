//
//  PackFolderAccessTests.swift
//  VelocityVisualiserTests
//
//  The sandbox itself cannot be exercised from a unit test: a security-scoped
//  bookmark needs the entitlement and a real operator grant. What can be
//  tested is everything around it — which folder covers which pack, when the
//  operator is asked, what is remembered, and that access is held for the
//  session and released after it. The bookmark is replaced by a plain path
//  behind FolderBookmarking, so these tests fail on the logic and never on
//  the machine they run on.
//

import Foundation
import Testing

@testable import VelocityVisualiser

private struct Unresolvable: Error {}

/// Bookmarks that are just the folder's path, with the failure modes a real
/// bookmark has: it can stop resolving, go stale, or be refused access.
private final class PathBookmarking: FolderBookmarking {
    var unresolvable: Set<String> = []
    var stale: Set<String> = []
    var refuseAccess = false
    private(set) var started: [String] = []
    private(set) var stopped: [String] = []

    func makeBookmark(for folder: URL) throws -> Data { Data(folder.path.utf8) }

    func resolve(_ bookmark: Data) throws -> (url: URL, isStale: Bool) {
        let path = String(decoding: bookmark, as: UTF8.self)
        if unresolvable.contains(path) { throw Unresolvable() }
        return (URL(fileURLWithPath: path), stale.contains(path))
    }

    func startAccess(_ url: URL) -> Bool {
        guard !refuseAccess else { return false }
        started.append(url.path)
        return true
    }

    func stopAccess(_ url: URL) { stopped.append(url.path) }
}

private final class MemoryStorage: FolderBookmarkStorage {
    var bookmarks: [Data] = []
    private(set) var saveCount = 0

    func load() -> [Data] { bookmarks }
    func save(_ bookmarks: [Data]) {
        self.bookmarks = bookmarks
        saveCount += 1
    }
}

private func makeAccess() -> (PackFolderAccess, PathBookmarking, MemoryStorage) {
    let bookmarking = PathBookmarking()
    let storage = MemoryStorage()
    return (PackFolderAccess(bookmarking: bookmarking, storage: storage), bookmarking, storage)
}

struct PackFolderContainmentTests {
    private let packs = URL(fileURLWithPath: "/data/lidar/annotation-packs")

    @Test func aPackBeneathTheFolderIsContained() {
        let pack = packs.appendingPathComponent("run-1-20260920")
        #expect(PackFolderAccess.folder(packs, contains: pack))
    }

    @Test func theFolderContainsItself() {
        #expect(PackFolderAccess.folder(packs, contains: packs))
    }

    /// The reason containment is by path component and not string prefix.
    @Test func aSiblingSharingTheFoldersNameAsAPrefixIsNotContained() {
        let sibling = URL(fileURLWithPath: "/data/lidar/annotation-packs-old/run-1")
        #expect(!PackFolderAccess.folder(packs, contains: sibling))
    }

    @Test func theFoldersParentIsNotContained() {
        #expect(!PackFolderAccess.folder(packs, contains: packs.deletingLastPathComponent()))
    }

    @Test func dotDotSegmentsCannotClimbOutOfTheFolder() {
        let escaping = URL(fileURLWithPath: "/data/lidar/annotation-packs/../secrets/run-1")
        #expect(!PackFolderAccess.folder(packs, contains: escaping))
    }

    /// The server reports canonical paths; the operator may have granted the
    /// folder through a symlink. Both sides are resolved before comparing.
    @Test func aPackReachedThroughASymlinkedFolderIsContained() throws {
        let base = try FileManager.default.url(
            for: .itemReplacementDirectory, in: .userDomainMask,
            appropriateFor: FileManager.default.temporaryDirectory, create: true)
        let real = base.appendingPathComponent("real-packs")
        try FileManager.default.createDirectory(
            at: real.appendingPathComponent("run-1"), withIntermediateDirectories: true)
        let link = base.appendingPathComponent("linked-packs")
        try FileManager.default.createSymbolicLink(at: link, withDestinationURL: real)

        #expect(PackFolderAccess.folder(link, contains: real.appendingPathComponent("run-1")))
        #expect(PackFolderAccess.folder(real, contains: link.appendingPathComponent("run-1")))
    }
}

struct PackFolderAccessTests {
    private let packs = URL(fileURLWithPath: "/data/lidar/annotation-packs")
    private var pack: URL { packs.appendingPathComponent("run-1-20260920") }

    @Test func nothingRememberedMeansNoGrant() {
        let (access, bookmarking, _) = makeAccess()

        #expect(access.beginAccess(for: pack) == nil)
        #expect(bookmarking.started.isEmpty)
    }

    @Test func aRememberedFolderGrantsAccessToAPackInsideIt() throws {
        let (access, bookmarking, _) = makeAccess()
        try access.remember(packs)

        let grant = access.beginAccess(for: pack)

        #expect(grant?.folder.path == packs.path)
        #expect(bookmarking.started == [packs.path])
    }

    @Test func aRememberedFolderDoesNotGrantAPackOutsideIt() throws {
        let (access, bookmarking, _) = makeAccess()
        try access.remember(packs)

        let elsewhere = URL(fileURLWithPath: "/Volumes/lidar/annotation-packs/run-9")

        #expect(access.beginAccess(for: elsewhere) == nil)
        #expect(bookmarking.started.isEmpty, "access must not be started on a folder that does not cover the pack")
    }

    /// Packs will not always live under one root.
    @Test func anyOfSeveralRememberedFoldersCanGrant() throws {
        let (access, bookmarking, storage) = makeAccess()
        let volume = URL(fileURLWithPath: "/Volumes/lidar/annotation-packs")
        try access.remember(packs)
        try access.remember(volume)

        let grant = access.beginAccess(for: volume.appendingPathComponent("run-9"))

        #expect(storage.bookmarks.count == 2)
        #expect(grant?.folder.path == volume.path)
        #expect(bookmarking.started == [volume.path])
    }

    @Test func rememberingAFolderAlreadyCoveredAddsNothing() throws {
        let (access, _, storage) = makeAccess()
        try access.remember(packs)
        try access.remember(packs)
        try access.remember(packs.appendingPathComponent("nested"))

        #expect(storage.bookmarks.count == 1)
    }

    @Test func endingAGrantStopsAccessExactlyOnce() throws {
        let (access, bookmarking, _) = makeAccess()
        try access.remember(packs)
        let grant = try #require(access.beginAccess(for: pack))

        grant.end()
        grant.end()

        #expect(bookmarking.stopped == [packs.path], "start and stop must balance")
    }

    /// The folder was deleted or its volume is gone. Keeping the bookmark
    /// would fail the same way on every later launch.
    @Test func aBookmarkThatNoLongerResolvesIsForgotten() throws {
        let (access, bookmarking, storage) = makeAccess()
        let gone = URL(fileURLWithPath: "/Volumes/unplugged/packs")
        try access.remember(gone)
        try access.remember(packs)
        bookmarking.unresolvable = [gone.path]

        let grant = access.beginAccess(for: pack)

        #expect(grant != nil, "one dead bookmark must not block a live one")
        #expect(storage.bookmarks == [Data(packs.path.utf8)])
    }

    @Test func aStaleBookmarkIsRefreshedAndStillGrants() throws {
        let (access, bookmarking, storage) = makeAccess()
        try access.remember(packs)
        let savesBefore = storage.saveCount
        bookmarking.stale = [packs.path]

        let grant = access.beginAccess(for: pack)

        #expect(grant != nil)
        #expect(storage.saveCount == savesBefore + 1, "the refreshed bookmark must be written back")
    }

    @Test func beingRefusedAccessIsNotAGrant() throws {
        let (access, bookmarking, _) = makeAccess()
        try access.remember(packs)
        bookmarking.refuseAccess = true

        #expect(access.beginAccess(for: pack) == nil)
    }

    @Test func resolvingWithoutChangeDoesNotRewriteStorage() throws {
        let (access, _, storage) = makeAccess()
        try access.remember(packs)
        let savesBefore = storage.saveCount

        _ = access.beginAccess(for: pack)

        #expect(storage.saveCount == savesBefore)
    }
}

/// The route the operator actually takes: generate a pack, have it open.
@MainActor struct GeneratedPackOpeningTests {
    /// Counts and answers the "which folder?" question in place of a panel.
    private final class Asker {
        var answer: URL?
        private(set) var askedAbout: [URL] = []
        func ask(_ suggested: URL) -> URL? {
            askedAbout.append(suggested)
            return answer
        }
    }

    private func makeController(answer: URL?) -> (
        AnnotationController, Asker, PathBookmarking, MemoryStorage
    ) {
        let (access, bookmarking, storage) = makeAccess()
        let asker = Asker()
        asker.answer = answer
        let controller = AnnotationController(folderAccess: access, askForFolder: asker.ask)
        return (controller, asker, bookmarking, storage)
    }

    @Test func theFirstGeneratedPackAsksOnceThenOpensAndRemembers() throws {
        let pack = try PackFixture.write()
        let folder = pack.deletingLastPathComponent()
        let (controller, asker, bookmarking, storage) = makeController(answer: folder)

        controller.openGeneratedPack(at: pack)

        #expect(controller.session != nil)
        #expect(controller.lastError == nil)
        #expect(asker.askedAbout.map(\.path) == [folder.path], "the panel opens on the pack's folder")
        #expect(storage.bookmarks.count == 1)
        #expect(bookmarking.started.count == 1, "access is held for the session, for saves")
        #expect(bookmarking.stopped.isEmpty)
    }

    @Test func aLaterPackInARememberedFolderOpensWithoutAsking() throws {
        let first = try PackFixture.write()
        let (controller, asker, _, _) = makeController(answer: first.deletingLastPathComponent())
        controller.openGeneratedPack(at: first)

        let second = try PackFixture.write()
        controller.openGeneratedPack(at: second)

        #expect(controller.packName == second.lastPathComponent)
        #expect(asker.askedAbout.count == 1, "the operator is asked once per folder, not once per pack")
    }

    @Test func replacingTheSessionReleasesThePreviousGrant() throws {
        let first = try PackFixture.write()
        let (controller, _, bookmarking, _) = makeController(
            answer: first.deletingLastPathComponent())
        controller.openGeneratedPack(at: first)

        controller.openGeneratedPack(at: try PackFixture.write())

        #expect(bookmarking.started.count == 2)
        #expect(bookmarking.stopped.count == 1)
    }

    @Test func closingReleasesTheGrant() throws {
        let pack = try PackFixture.write()
        let (controller, _, bookmarking, _) = makeController(
            answer: pack.deletingLastPathComponent())
        controller.openGeneratedPack(at: pack)

        controller.close()

        #expect(controller.session == nil)
        #expect(bookmarking.stopped.count == bookmarking.started.count)
    }

    @Test func decliningToGrantLeavesAnExplanationAndNoSession() throws {
        let pack = try PackFixture.write()
        let (controller, _, bookmarking, storage) = makeController(answer: nil)

        controller.openGeneratedPack(at: pack)

        #expect(controller.session == nil)
        #expect(controller.lastError?.contains(pack.path) == true, "the operator needs to know where the pack went")
        #expect(storage.bookmarks.isEmpty)
        #expect(bookmarking.started.isEmpty)
    }

    @Test func grantingAFolderThatDoesNotHoldThePackIsRefused() throws {
        let pack = try PackFixture.write()
        let unrelated = URL(fileURLWithPath: "/Volumes/lidar/somewhere-else")
        let (controller, _, _, storage) = makeController(answer: unrelated)

        controller.openGeneratedPack(at: pack)

        #expect(controller.session == nil)
        #expect(controller.lastError?.contains("does not contain") == true)
        #expect(storage.bookmarks.isEmpty, "a folder that does not help must not be remembered")
    }

    @Test func aPackThatFailsToOpenDoesNotKeepItsGrant() throws {
        let broken = try PackFixture.write { files in files["manifest.json"] = Data("{".utf8) }
        let (controller, _, bookmarking, _) = makeController(
            answer: broken.deletingLastPathComponent())

        controller.openGeneratedPack(at: broken)

        #expect(controller.session == nil)
        #expect(controller.lastError != nil)
        #expect(bookmarking.stopped.count == bookmarking.started.count)
    }

    /// An operator-chosen pack needs no folder grant: the panel is the grant.
    @Test func anOperatorChosenPackNeitherAsksNorStartsAccess() throws {
        let pack = try PackFixture.write()
        let (controller, asker, bookmarking, _) = makeController(answer: nil)

        controller.openPack(at: pack)

        #expect(controller.session != nil)
        #expect(asker.askedAbout.isEmpty)
        #expect(bookmarking.started.isEmpty)
    }
}
