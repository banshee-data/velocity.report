// RoadReviewAutopilot.swift
// A scripted first-pass review of the road surface, for taking screenshots and
// a timed traverse on the machine in front of you without clicking through it.
// Off unless VV_ROAD_AUTOPILOT names an output folder; it changes nothing in a
// normal launch.
//
//   VV_ROAD_AUTOPILOT=<dir>         run, write screenshots and notes to <dir>, quit
//   VV_GERARDUS_ADDRESS=host:port   the Gerardus server (default: the saved one)
//   VV_ROAD_SITE=<l16 token>        the site to open (default: the first listed)
//   VV_ROAD_TRAVERSE=<seconds>      traverse length (default 60)
//
// Each step writes `view-*.png` (the 3D view, read back from the drawable) and
// `window-*.png` (the whole window), and `review.md` records what was seen.

import AppKit
import Foundation

@available(macOS 15.0, *) @MainActor enum RoadReviewAutopilot {
    static func runIfRequested(_ app: AppState) async {
        let env = ProcessInfo.processInfo.environment
        guard let path = env["VV_ROAD_AUTOPILOT"] else { return }
        var dir = URL(fileURLWithPath: path)
        // The app is sandboxed: outside its container a write fails quietly, so
        // fall back to the container's temporary folder and say where it is.
        let probe = dir.appendingPathComponent(".probe")
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        if (try? Data().write(to: probe)) == nil {
            dir = FileManager.default.temporaryDirectory.appendingPathComponent("road-review")
            try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        } else {
            try? FileManager.default.removeItem(at: probe)
        }
        print("AUTOPILOT output \(dir.path)")
        var log: [String] = []
        func say(_ line: String) {
            log.append(line)
            print("AUTOPILOT", line)
        }
        func wait(_ what: String, seconds: Double = 30, until: () -> Bool) async -> Bool {
            let end = Date().addingTimeInterval(seconds)
            while Date() < end {
                if until() { return true }
                try? await Task.sleep(for: .milliseconds(100))
            }
            say("timed out waiting for \(what)")
            return false
        }
        func shoot(_ name: String) async {
            try? await Task.sleep(for: .milliseconds(600))
            if let renderer = app.renderer {
                await withCheckedContinuation { (k: CheckedContinuation<Void, Never>) in
                    renderer.captureNextFrame { image in
                        if let image {
                            write(image, dir.appendingPathComponent("view-\(name).png"))
                        }
                        k.resume()
                    }
                }
            }
            if let view = NSApp.windows.first(where: {
                $0.isVisible && $0.title == "VelocityVisualiser"
            })?.contentView, let rep = view.bitmapImageRepForCachingDisplay(in: view.bounds) {
                view.cacheDisplay(in: view.bounds, to: rep)
                if let png = rep.representation(using: .png, properties: [:]) {
                    try? png.write(to: dir.appendingPathComponent("window-\(name).png"))
                }
            }
            say("screenshot \(name)")
        }

        if let text = env["VV_GERARDUS_ADDRESS"], let address = try? GerardusAddress.parse(text) {
            app.alignment.useServiceAddress(address)
        }
        app.alignment.startProbing()
        guard await wait("Gerardus", until: { app.alignment.isAvailable }) else {
            finish(dir, log, quit: true)
            return
        }
        say(
            "server \(app.alignment.serverInfo?.serverVersion ?? "?"), road_surface_version \(app.alignment.serverInfo?.roadSurfaceVersion ?? 0)"
        )
        app.showRoadPane = true
        guard await wait("sites", until: { !app.road.sites.isEmpty }) else {
            finish(dir, log, quit: true)
            return
        }
        let token = env["VV_ROAD_SITE"] ?? app.road.sites[0].id
        app.road.open(siteToken: token)
        _ = await wait(
            "load", seconds: 60,
            until: { !app.road.loader.state.isLoading && app.road.loader.state != .idle })
        try? await Task.sleep(for: .seconds(1))
        say("state \(app.road.statusLine); blocker \(app.road.blocker ?? "none")")
        say(
            "patches \(app.road.patches.count), duplicates \(app.road.loader.store.duplicates), conflicts \(app.road.loader.store.conflicts.count), rejected \(app.road.loader.store.rejected.count)"
        )
        for c in app.road.coverage {
            say(
                "corridor \(c.corridorID): \(c.patchCount) patches, inventory \(c.inventoryLengthM) m, surfaced \(c.surfacedLengthM) m, inferred \(c.inferredLengthM) m, missing \(c.missingLengthM) m, gaps \(c.gaps.count), build \(c.buildHash)"
            )
        }
        guard !app.road.patches.isEmpty else {
            finish(dir, log, quit: true)
            return
        }
        try? await Task.sleep(for: .seconds(1))
        if let a = app.road.meshAudit {
            say(
                "mesh audit: \(a.invertedTotal) inverted, \(a.degenerateTotal) degenerate, \(a.overlapTotal) overlapping, \(a.seamPairs) seam pairs, worst step \(a.seamStepMaxM * 1000) mm, \(a.seamViolations.count) over 5 mm"
            )
        }
        app.road.frameCorridor()
        await shoot("01-overhead-support")
        app.road.options.shading = .plain
        await shoot("02-overhead-plain")
        app.road.options.shading = .support
        app.road.options.wireframe = true
        await shoot("03-wireframe")
        app.road.options.wireframe = false
        if let pick = app.road.patches.first(where: { $0.profile.count > 20 })
            ?? app.road.patches.first
        {
            app.road.selectedPatchID = pick.patchID
            if let i = app.road.inspector {
                say(
                    "inspector \(i.patchID): cnn \(i.cnn), boundary \(i.boundarySource.label), observed \(i.observed), area \(i.areaM2) (obs \(i.observedAreaM2), int \(i.interpolatedAreaM2), miss \(i.missingAreaM2)), rider \(i.riderLine), held-out \(i.heldOutPoints), rms \(i.residualRmsM), p95 \(i.residualP95AbsM), safety not assessed \(i.notAssessed)"
                )
            }
            await shoot("04-selected-patch")
            app.road.selectedPatchID = nil
        }
        // Drive: the first start option, forwards; screenshots along and at the stop.
        guard let start = app.drive.startOptions.first else {
            say("no profile to drive")
            finish(dir, log, quit: true)
            return
        }
        app.drive.begin(on: start.patchID)
        await shoot("05-drive-start")
        app.drive.scrub(to: app.drive.routeLength * 0.4)
        await shoot("06-drive-middle")
        app.drive.scrub(to: app.drive.routeLength)
        say(
            "end of scrub: \(app.drive.message ?? "no stop") at \(app.drive.distanceAlong) of \(app.drive.routeLength) m"
        )
        await shoot("07-drive-stop")
        app.drive.overhead()
        await shoot("08-back-overhead")
        // Traverse.
        app.drive.begin(on: start.patchID)
        let seconds = Double(env["VV_ROAD_TRAVERSE"] ?? "") ?? 60
        app.drive.startTraverse(seconds: seconds)
        _ = await wait("traverse", seconds: seconds + 30, until: { app.drive.traverseUntil == nil })
        if let summary = app.drive.lastTraverse {
            let note = TraverseReport.markdown(
                summary, app.drive.provenance(), date: ISO8601DateFormatter().string(from: Date()),
                label: "autopilot, \(Int(seconds)) s")
            try? note.write(
                to: dir.appendingPathComponent("traverse.md"), atomically: true, encoding: .utf8)
            say(
                "traverse p95 \(summary.intervalP95Ms) ms over \(summary.frames) frames, \(summary.meetsTarget ? "met" : "not met")"
            )
        }
        await shoot("09-after-traverse")
        finish(dir, log, quit: env["VV_ROAD_AUTOPILOT_KEEP"] == nil)
    }

    private static func write(_ image: CGImage, _ url: URL) {
        let rep = NSBitmapImageRep(cgImage: image)
        if let png = rep.representation(using: .png, properties: [:]) { try? png.write(to: url) }
    }

    private static func finish(_ dir: URL, _ log: [String], quit: Bool) {
        try? log.joined(separator: "\n").write(
            to: dir.appendingPathComponent("review.md"), atomically: true, encoding: .utf8)
        if quit { NSApp.terminate(nil) }
    }
}
