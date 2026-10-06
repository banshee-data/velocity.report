//
//  InertModifier.swift
//  VelocityVisualiser
//
//  An alternative to `.disabled()` for controls whose availability changes
//  while the view graph is updating.
//
//  Traced on 2026-08-28: 17 of 24 AttributeGraph cycles were rooted at
//  `-[NSCell setEnabled:]`, which calls `nextValidKeyView`, which makes AppKit
//  recompute the window's key-view loop, which re-enters SwiftUI's view graph
//  through NSHostingView.responderNode — inside the update that set it. The
//  cycle is AppKit and SwiftUI re-entering each other, and it starts with the
//  enabled state changing.
//

import SwiftUI

extension View {

    /// Makes a control unavailable without changing AppKit's enabled state.
    ///
    /// Prefer plain `.disabled()` where availability changes rarely: it is the
    /// semantically correct API and carries its meaning to assistive
    /// technologies for free. Reach for this only where the state flips during
    /// view updates, and pass `hint` so the reason survives the substitution.
    func inert(_ isInert: Bool, hint: String) -> some View {
        self.opacity(isInert ? 0.45 : 1).allowsHitTesting(!isInert).accessibilityHint(
            isInert ? hint : "")
    }
}
