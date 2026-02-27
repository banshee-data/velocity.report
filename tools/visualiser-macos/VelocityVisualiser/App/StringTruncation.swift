// StringTruncation.swift
// Shared string truncation utility used across the application.

import Foundation

extension String {
    /// Truncate string with a unicode ellipsis (…).
    /// E.g. "abc123def456".truncated(8) → "abc123de…"
    func truncated(_ maxLength: Int) -> String {
        if count <= maxLength { return self }
        return String(prefix(maxLength)) + "\u{2026}"
    }
}
