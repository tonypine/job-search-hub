import AppKit
import JobSearchHubCore
import SwiftUI

/// Going back to an earlier version: what it would lose, and the advice to
/// stay; or, from *Install an older version…*, the versions the hub can go
/// back to. It reads the going back live; the snapshots draw the same
/// content from values.
struct GoBackSheet: View {
    private let goingBack = GoingBack.shared

    var body: some View {
        let failure = Binding(get: { goingBack.failure }, set: { goingBack.failure = $0 })
        switch goingBack.page {
        case .olderVersions where goingBack.offer == nil:
            OlderVersionsContent(
                versions: goingBack.olderVersions, isLoading: goingBack.isLoadingOlderVersions, downloading: goingBack.downloading, failure: failure,
                choose: { older in Task { await goingBack.offerOlder(older) } },
                retry: { Task { await goingBack.loadOlderVersions() } }, cancel: goingBack.dismiss
            )
        default:
            GoBackSheetContent(
                offer: goingBack.offer, isWeighing: goingBack.isWeighing, isStarting: goingBack.isStarting, now: .now, failure: failure,
                goBack: goingBack.confirm, cancel: goingBack.dismiss,
                back: goingBack.page == .olderVersions ? { goingBack.showList() } : nil
            )
        }
    }
}

/// The sheet's offer, from values: "Go back to 0.1.247?", or, after a crash,
/// "Job Search Hub 0.1.252 quit unexpectedly".
struct GoBackSheetContent: View {
    let offer: GoingBack.Offer?
    var isWeighing = false
    var isStarting = false
    let now: Date
    @Binding var failure: HubFailure?
    let goBack: () -> Void
    let cancel: () -> Void
    /// Back to the list of older versions, when the offer came from it.
    var back: (() -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            if let offer {
                Text(title(offer)).font(.title3.weight(.semibold))
                if offer.reason == .crash, let installedAt = offer.installedAt {
                    Text("It's the version installed \(GoingBackCost.describeTime(installedAt, now: now)).")
                        .foregroundStyle(.secondary)
                }
                Text(offer.cost.describeDatabase(running: offer.running, target: offer.target, now: now))
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if case let .loses(_, lost) = offer.cost, !lost.isEmpty {
                    LostTiles(lost: lost)
                    Text("These would be lost. Or stay on \(offer.running.description) and wait for a fix: new versions arrive most days.")
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                } else if let sentence = offer.cost.lostSentence {
                    Text(offer.cost.losesWrites ? "\(sentence) Or stay on \(offer.running.description) and wait for a fix: new versions arrive most days." : sentence)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if offer.cost.isPossible {
                    Text("Job Search Hub quits, and reopens on \(offer.target.description) once the hub is back. \(offer.running.description) isn't offered again.")
                        .font(.hubSecondary)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            } else if isWeighing {
                HStack(spacing: Space.s) {
                    ProgressView().controlSize(.small)
                    Text("Counting what going back would lose…").foregroundStyle(.secondary)
                }
            }
            if failure != nil {
                HubErrorView($failure)
            }
            buttons
        }
        .padding(Space.l)
        .frame(width: 500, alignment: .leading)
    }

    private func title(_ offer: GoingBack.Offer) -> String {
        offer.reason == .crash ? "Job Search Hub \(offer.running.description) quit unexpectedly" : "Go back to \(offer.target.description)?"
    }

    @ViewBuilder
    private var buttons: some View {
        HStack(spacing: Space.s) {
            if let back {
                Button("Other versions", action: back)
            }
            Spacer()
            if isStarting {
                ProgressView().controlSize(.small)
                Text("Going back…").foregroundStyle(.secondary)
            } else if let offer, offer.cost.isPossible {
                Button(offer.reason == .crash ? "Stay on \(offer.running.description)" : "Cancel", action: cancel)
                    .keyboardShortcut(.cancelAction)
                if offer.cost.losesWrites && offer.reason != .crash {
                    Button(role: .destructive, action: goBack) {
                        Text(isLosingCounted(offer) ? "Go back and lose these" : "Go back and lose it")
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(Tone.negative.color)
                } else {
                    Button("Go back to \(offer.target.description)", action: goBack)
                        .keyboardShortcut(.defaultAction)
                        .buttonStyle(.borderedProminent)
                        .tint(offer.reason == .crash ? Tone.caution.color : nil)
                }
            } else {
                Button(offer == nil && isWeighing ? "Cancel" : "Close", action: cancel)
                    .keyboardShortcut(.cancelAction)
            }
        }
        .controlSize(.large)
    }

    private func isLosingCounted(_ offer: GoingBack.Offer) -> Bool {
        if case .loses = offer.cost { true } else { false }
    }
}

/// What going back would lose, a tile per kind of write: "3 jobs found".
private struct LostTiles: View {
    let lost: [LostCount]

    var body: some View {
        HStack(alignment: .top, spacing: Space.l) {
            ForEach(lost, id: \.self) { count in
                VStack(alignment: .leading, spacing: 2) {
                    Text("\(count.rows)").font(.title2.weight(.semibold)).monospacedDigit()
                    Text(count.words).font(.hubCaption).foregroundStyle(.secondary)
                }
                .accessibilityElement(children: .combine)
            }
            Spacer(minLength: 0)
        }
        .padding(Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(.quaternary))
    }
}

/// *Install an older version…*: the older releases the hub can go back to,
/// each with what it would lose.
struct OlderVersionsContent: View {
    let versions: [OlderVersion]?
    let isLoading: Bool
    let downloading: HubVersion?
    @Binding var failure: HubFailure?
    let choose: (OlderVersion) -> Void
    let retry: () -> Void
    let cancel: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text("Install an older version").font(.title3.weight(.semibold))
            Text("Older versions download again from GitHub. Only those that can run on the database are listed: those from before its last change, while its copy from then is kept, and those it hasn't changed since.")
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if let versions, !versions.isEmpty {
                VStack(spacing: 0) {
                    ForEach(Array(versions.enumerated()), id: \.element.version) { index, older in
                        if index > 0 { Divider() }
                        row(older)
                    }
                }
                .background(.background, in: RoundedRectangle(cornerRadius: Radius.card))
                .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(.quaternary))
            } else if versions != nil {
                Text("No older version can run on the database as it is now.").foregroundStyle(.secondary)
            } else if isLoading {
                HStack(spacing: Space.s) {
                    ProgressView().controlSize(.small)
                    Text("Reading the releases…").foregroundStyle(.secondary)
                }
            }
            if failure != nil {
                HubErrorView($failure, retry: versions == nil ? retry : nil)
            }
            HStack {
                Spacer()
                Button("Cancel", action: cancel)
                    .keyboardShortcut(.cancelAction)
            }
            .controlSize(.large)
        }
        .padding(Space.l)
        .frame(width: 500, alignment: .leading)
    }

    private func row(_ older: OlderVersion) -> some View {
        HStack(spacing: Space.s) {
            VStack(alignment: .leading, spacing: 2) {
                Text(older.version.description).fontWeight(.medium)
                Text(older.cost.summarize(target: older.version))
                    .font(.hubCaption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: Space.s)
            if downloading == older.version {
                ProgressView().controlSize(.small)
                Text("Downloading…").font(.hubCaption).foregroundStyle(.secondary)
            } else {
                Button("Go back…") { choose(older) }
                    .disabled(downloading != nil)
                    .help("Download \(older.version.description), check it, and see what going back to it would lose")
            }
        }
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
    }
}

/// Where the connection banner goes, while the running version is
/// withdrawn: "0.1.252 was withdrawn. Going back to 0.1.247 loses nothing."
struct WithdrawnBanner: View {
    let running: HubVersion
    let summary: String?
    let goBack: (() -> Void)?
    let dismiss: () -> Void

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Tone.caution.color).accessibilityHidden(true)
            Text([running.description + " was withdrawn.", summary].compactMap(\.self).joined(separator: " "))
                .fontWeight(.medium)
                .fixedSize(horizontal: false, vertical: true)
            if let goBack {
                Button("Go back…", action: goBack).buttonStyle(.link).fontWeight(.medium)
            }
            Spacer(minLength: Space.s)
            Button(action: dismiss) {
                Image(systemName: "xmark").font(.caption.weight(.semibold))
            }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
            .help("Dismiss")
            .accessibilityLabel("Dismiss")
        }
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
        .background(Tone.caution.fill, in: RoundedRectangle(cornerRadius: Radius.card))
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
        .accessibilityElement(children: .contain)
    }
}
