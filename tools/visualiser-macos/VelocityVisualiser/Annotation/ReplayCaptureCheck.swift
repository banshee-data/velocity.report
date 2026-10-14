// ReplayCaptureCheck.swift
// Whether the main view is replaying the capture this pack's labels cite.
//
// Frame sync matches the main view's frame to a pack sample by time, and
// time cannot tell two captures apart: another file of the same street, or a
// recording whose clock happens to overlap, matches as readily as this one.
// Packets can. The stream lists, for each point frame, the packets the frame
// was decoded from, by the same key a label set's packet table uses. A frame
// of this capture has its packets in the table; a frame of another capture
// has none of them.
//
// The answer is information for the operator. Nothing is blocked by it.

import Foundation

/// What the main view's packets were compared with.
enum ReplayCheckSubject: Equatable {
    /// The packet table of the label set the pack is a view of.
    case labelSet
    /// A version 2 pack's own packets, before its first save makes a label set.
    case pack

    var noun: String {
        switch self {
        case .labelSet: return "this label set"
        case .pack: return "this pack"
        }
    }
}

/// How the main view's frame relates to the labels' capture, by packet key.
enum ReplayCaptureCheck: Equatable {
    /// At least nine in ten of the frame's packets are in the table.
    case sameCapture(found: Int, of: Int)
    /// Some of the frame's packets are in the table, but fewer than nine in ten.
    case partly(found: Int, of: Int)
    /// None of the frame's packets are in the table.
    case differentCapture(of: Int)
    /// The frame names no packets: an older recording, a live source, or an
    /// older server.
    case cannotCheck

    /// Nine in ten, not all: a frame of the same capture can still be cut a
    /// packet or two differently at its edges, by a replay that began part
    /// way through a rotation or a pack whose table ends at the excerpt's last
    /// frame.
    static let sameShareNumerator = 9
    static let sameShareDenominator = 10

    /// Compares one frame's packet keys with a packet table.
    static func evaluate(frameKeys: [UInt64], table: Set<UInt64>) -> ReplayCaptureCheck {
        guard !frameKeys.isEmpty else { return .cannotCheck }
        var found = 0
        for key in frameKeys where table.contains(key) { found += 1 }
        let total = frameKeys.count
        if found == 0 { return .differentCapture(of: total) }
        if found * sameShareDenominator >= total * sameShareNumerator {
            return .sameCapture(found: found, of: total)
        }
        return .partly(found: found, of: total)
    }

    /// True when some or all of the frame is not this capture: the line is
    /// drawn as a warning.
    var differs: Bool {
        switch self {
        case .partly, .differentCapture: return true
        case .sameCapture, .cannotCheck: return false
        }
    }

    /// One line for the window, beside the frame sync status.
    func label(_ subject: ReplayCheckSubject) -> String {
        switch self {
        case .sameCapture(let found, let total):
            return "Same capture: \(found) of \(total) packets of the main view's frame are in "
                + "\(subject.noun)."
        case .partly(let found, let total):
            return "Partly the same capture: \(found) of \(total) packets of the main view's frame "
                + "are in \(subject.noun)."
        case .differentCapture:
            return "Different capture: the main view is replaying packets \(subject.noun) does not "
                + "cover."
        case .cannotCheck:
            return "Cannot check the capture: the main view's frame has no packet keys. It is an "
                + "older recording, a live source, or an older server."
        }
    }
}
