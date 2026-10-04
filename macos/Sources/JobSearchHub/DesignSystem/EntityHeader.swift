import SwiftUI

/// The top of a job's, company's or person's details: an eyebrow naming the
/// kind, the title, one line of facts, and up to three chips.
struct EntityHeader<Facts: View, Chips: View>: View {
    /// The kind, and its parent when it has one: "Job · Northwind".
    let eyebrow: String
    let title: String
    @ViewBuilder let facts: Facts
    @ViewBuilder let chips: Chips

    init(eyebrow: String, title: String, @ViewBuilder facts: () -> Facts, @ViewBuilder chips: () -> Chips) {
        self.eyebrow = eyebrow
        self.title = title
        self.facts = facts()
        self.chips = chips()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            Text(eyebrow.uppercased())
                .font(.hubCaption.weight(.semibold))
                .foregroundStyle(.secondary)
                .lineLimit(1)
            Text(title).font(.hubEntity).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
            facts
                .font(.hubSecondary)
                .foregroundStyle(.secondary)
            if Chips.self != EmptyView.self {
                HStack(spacing: Space.xs) { chips }
                    .padding(.top, Space.xs)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

extension EntityHeader where Chips == EmptyView {
    init(eyebrow: String, title: String, @ViewBuilder facts: () -> Facts) {
        self.init(eyebrow: eyebrow, title: title, facts: facts) { EmptyView() }
    }
}

extension EntityHeader where Facts == Text {
    /// A header whose facts are one line of text, left out when empty.
    init(eyebrow: String, title: String, facts: [String?], @ViewBuilder chips: () -> Chips) {
        let line = facts.compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
        self.init(eyebrow: eyebrow, title: title, facts: { Text(line) }, chips: chips)
    }
}
