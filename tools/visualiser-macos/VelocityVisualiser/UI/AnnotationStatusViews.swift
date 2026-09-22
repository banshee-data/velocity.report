// AnnotationStatusViews.swift
// The two strips across the top of the annotation window: how far through the
// labelling is, and what went wrong or what to do next.

import SwiftUI

/// How much of this frame, and of every frame, is labelled. Two bars across
/// the top of the views, because the question an operator keeps asking is "how
/// far through am I", and a ring in a sidebar answers "where is what is left".
struct AnnotationProgressHeader: View {
    @ObservedObject var session: AnnotationSession

    var body: some View {
        HStack(spacing: 16) {
            bar(
                "Frame \(session.sampleIndex + 1) of \(session.samples.count)",
                session.completeness.whole)
            bar("All \(session.samples.count) frames", session.packTally)
        }.padding(.horizontal, 12).padding(.vertical, 6)
    }

    private func bar(_ title: String, _ tally: LabelTally) -> some View {
        let labelled = tally.fractionAgreed + tally.fractionInQuestion
        return VStack(alignment: .leading, spacing: 3) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Text(title).font(.caption).foregroundStyle(.secondary)
                Text(String(format: "%.0f%% labelled", labelled * 100)).font(.callout.bold())
                    .monospacedDigit()
                Text(
                    String(
                        format: "%.0f%% agreed · %.0f%% in question · %d of %d points",
                        tally.fractionAgreed * 100, tally.fractionInQuestion * 100,
                        tally.agreed + tally.inQuestion, tally.total)
                ).font(.caption2).foregroundStyle(.secondary).monospacedDigit().lineLimit(1)
            }
            GeometryReader { geometry in
                HStack(spacing: 0) {
                    Rectangle().fill(AnnotationFrameStrip.agreedColour).frame(
                        width: geometry.size.width * CGFloat(tally.fractionAgreed))
                    Rectangle().fill(AnnotationFrameStrip.inQuestionColour).frame(
                        width: geometry.size.width * CGFloat(tally.fractionInQuestion))
                    Rectangle().fill(Color.white.opacity(0.12))
                }
            }.frame(height: 9).clipShape(RoundedRectangle(cornerRadius: 2))
        }.frame(maxWidth: .infinity, alignment: .leading).help(
            "Of the foreground the tracker could use. Agreed is labelled by a person or reviewed; "
                + "in question is proposed by an algorithm, or changed and not saved.")
    }
}

// MARK: - Status

/// What went wrong, or failing that what to do next. One line, always on
/// screen, across the top of the views: a refused save used to report itself
/// at the foot of a scrolling column, where a save that failed looked like one
/// that had worked.
struct AnnotationStatusStrip: View {
    @ObservedObject var session: AnnotationSession

    var body: some View {
        HStack(alignment: .top, spacing: 6) {
            if let error = session.lastError {
                Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.red)
                Text(error).foregroundStyle(.red)
            } else if let next = session.nextStep {
                Image(systemName: "arrow.right.circle").foregroundStyle(.secondary)
                Text(next).foregroundStyle(.secondary)
            } else {
                Image(systemName: "checkmark.circle").foregroundStyle(.green)
                Text("Saved.").foregroundStyle(.secondary)
            }
        }.font(.caption).fixedSize(horizontal: false, vertical: true).frame(
            maxWidth: .infinity, alignment: .leading
        ).padding(.horizontal, 12).padding(.vertical, 6)
    }
}
