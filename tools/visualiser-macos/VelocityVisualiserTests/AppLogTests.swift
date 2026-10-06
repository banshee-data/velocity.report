//
//  AppLogTests.swift
//  VelocityVisualiserTests
//
//  On 2026-08-28 `make run-mac` captured the app to a file for the first time.
//  The log held 241 AttributeGraph lines from stderr and almost none of the
//  app's own stdout, ending mid-word: redirected stdout is block-buffered, so
//  the diagnostics written to investigate a stall never reached the file.
//

import XCTest

@testable import VelocityVisualiser

@available(macOS 15.0, *) final class AppLogTests: XCTestCase {

    /// Timestamps must match the server's format, so an app event and a server
    /// event can be lined up when both logs describe the same second.
    func testTimestampMatchesTheServerFormat() {
        let line = captureStdout { AppLog.write("hello") }

        // yyyy/MM/dd HH:mm:ss.SSS
        let pattern = #"^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{3} hello$"#
        XCTAssertNotNil(
            line.range(of: pattern, options: .regularExpression),
            "log line \(line.debugDescription) does not carry a server-format timestamp")
    }

    func testMessageIsPreservedVerbatim() {
        let line = captureStdout { AppLog.write("frame 42 took 1234.5ms") }

        XCTAssertTrue(
            line.hasSuffix("frame 42 took 1234.5ms"),
            "the message was altered: \(line.debugDescription)")
    }

    func testVlogWritesThroughAppLog() {
        let line = captureStdout { vlog("via vlog") }

        XCTAssertTrue(line.hasSuffix("via vlog"))
    }

    /// Redirect stdout to a pipe for the duration of the block and return what
    /// was written, trimmed of its newline.
    private func captureStdout(_ body: () -> Void) -> String {
        let pipe = Pipe()
        let original = dup(STDOUT_FILENO)
        dup2(pipe.fileHandleForWriting.fileDescriptor, STDOUT_FILENO)

        body()
        fflush(stdout)

        dup2(original, STDOUT_FILENO)
        close(original)
        try? pipe.fileHandleForWriting.close()

        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        return String(data: data, encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }
}
