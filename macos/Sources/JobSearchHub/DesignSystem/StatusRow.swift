import JobSearchHubCore
import SwiftUI

/// An account, a service or the hub (P12): a tile, the name with ⓘ for what
/// it is, its state as a dot and a word with a few words after it, and one
/// action. What used to be a section's footer paragraph is behind ⓘ.
struct StatusRow<Action: View>: View {
    let title: String
    let symbol: String
    var tileTone = Tone.accent
    /// The state in a word or two, in its tone: "Connected", "Stopped".
    let state: String
    let stateTone: Tone
    /// A few words after the state: "Reads Gmail and Calendar".
    var detail: String?
    /// What it is and does, behind ⓘ.
    let help: String
    @ViewBuilder let action: () -> Action
    @State private var isShowingHelp = false

    init(
        _ title: String, symbol: String, tileTone: Tone = .accent, state: String, stateTone: Tone, detail: String? = nil, help: String,
        @ViewBuilder action: @escaping () -> Action
    ) {
        self.title = title
        self.symbol = symbol
        self.tileTone = tileTone
        self.state = state
        self.stateTone = stateTone
        self.detail = detail
        self.help = help
        self.action = action
    }

    var body: some View {
        HStack(spacing: Space.m) {
            Image(systemName: symbol)
                .font(.system(size: 14, weight: .semibold))
                .foregroundStyle(.white)
                .frame(width: 28, height: 28)
                .background(tileTone.color, in: RoundedRectangle(cornerRadius: Radius.control))
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: Space.xs) {
                    Text(title).fontWeight(.semibold)
                    Button("About \(title)", systemImage: "info.circle") { isShowingHelp.toggle() }
                        .labelStyle(.iconOnly)
                        .buttonStyle(.borderless)
                        .foregroundStyle(.secondary)
                        .help("What \(title) is")
                        .popover(isPresented: $isShowingHelp, arrowEdge: .bottom) {
                            Text(help)
                                .fixedSize(horizontal: false, vertical: true)
                                .frame(width: 320, alignment: .leading)
                                .padding(Space.l)
                        }
                }
                HStack(spacing: Space.xs) {
                    Circle().fill(stateTone.color).frame(width: 7, height: 7)
                        .accessibilityHidden(true)
                    Text(state).foregroundStyle(stateTone == .neutral ? AnyShapeStyle(.secondary) : AnyShapeStyle(stateTone.color))
                    if let detail {
                        Text("· \(detail)").foregroundStyle(.secondary)
                    }
                }
                .font(.hubCaption)
                .lineLimit(1)
                .truncationMode(.middle)
                .accessibilityElement(children: .combine)
                .accessibilityLabel(detail.map { "\(state), \($0)" } ?? state)
            }
            Spacer(minLength: Space.s)
            action()
        }
        .padding(.vertical, 2)
    }
}
