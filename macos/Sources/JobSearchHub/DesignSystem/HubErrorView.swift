import JobSearchHubCore
import SwiftUI

/// What failed and what to do about it, a retry where one makes sense, and
/// the raw error folded away under Details.
struct HubErrorView: View {
    enum Style {
        /// A tinted box among a view's content, or at the bottom of a page.
        case inline
        /// The whole of a page or panel that couldn't load.
        case page
    }

    /// What failed: "Couldn't load the jobs".
    let title: String
    let report: ErrorReport
    var style = Style.inline
    var retry: (() -> Void)?
    /// Clears the error, for an inline one the owner has read.
    var dismiss: (() -> Void)?
    @State private var showsDetails = false

    var body: some View {
        switch style {
        case .inline: inline
        case .page: page
        }
    }

    private var inline: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(Tone.negative.color)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: Space.xs) {
                Text(title).fontWeight(.semibold)
                Text(report.advice).font(.hubSecondary).foregroundStyle(.secondary)
                details
            }
            .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: Space.s)
            if let retry {
                Button("Try again", action: retry)
                    .buttonBorderShape(.capsule)
            }
            if let dismiss {
                Button("Hide", systemImage: "xmark", action: dismiss)
                    .labelStyle(.iconOnly)
                    .buttonStyle(.borderless)
                    .help("Hide")
            }
        }
        .padding(Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Tone.negative.fill, in: RoundedRectangle(cornerRadius: Radius.card))
        .accessibilityElement(children: .contain)
    }

    private var page: some View {
        ContentUnavailableView {
            Label(title, systemImage: "exclamationmark.triangle")
        } description: {
            Text(report.advice)
        } actions: {
            if let retry {
                Button("Try again", action: retry)
            }
            details.frame(maxWidth: 420)
        }
    }

    @ViewBuilder
    private var details: some View {
        if let details = report.details {
            DisclosureGroup("Details", isExpanded: $showsDetails) {
                Text(details)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .font(.hubCaption)
            .foregroundStyle(.secondary)
        }
    }
}

/// Something that failed: what it was, and what to do about it.
struct HubFailure: Equatable {
    /// What failed: "Couldn't load the jobs".
    let title: String
    let report: ErrorReport

    init(_ title: String, _ error: any Error) {
        self.title = title
        report = ErrorReport(error)
    }

    init(_ title: String, advice: String, details: String? = nil) {
        self.title = title
        report = ErrorReport(advice: advice, details: details)
    }
}

extension HubErrorView {
    init(_ failure: HubFailure, style: Style = .inline, retry: (() -> Void)? = nil) {
        self.init(title: failure.title, report: failure.report, style: style, retry: retry)
    }

    /// A failure the owner reads and dismisses, which clears it.
    init(_ failure: Binding<HubFailure?>, retry: (() -> Void)? = nil) {
        let shown = failure.wrappedValue ?? HubFailure("", advice: "")
        self.init(title: shown.title, report: shown.report, retry: retry, dismiss: { failure.wrappedValue = nil })
    }
}
