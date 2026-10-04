import SwiftUI

/// The top of a job's, company's or person's details: an eyebrow naming the
/// kind and linking to its parent, the title, one line of facts, and up to
/// three chips.
struct EntityHeader<Facts: View, Chips: View>: View {
    /// The kind: "Job", "Company", "Person".
    let kind: String
    /// What it belongs to, after the kind: "Job · Northwind".
    var parent: String?
    /// Opens the parent; without it the parent is plain text.
    var openParent: (() -> Void)?
    let title: String
    @ViewBuilder let facts: Facts
    @ViewBuilder let chips: Chips

    init(
        kind: String, parent: String? = nil, openParent: (() -> Void)? = nil, title: String,
        @ViewBuilder facts: () -> Facts, @ViewBuilder chips: () -> Chips
    ) {
        self.kind = kind
        self.parent = parent
        self.openParent = openParent
        self.title = title
        self.facts = facts()
        self.chips = chips()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            eyebrow
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

    private var eyebrow: some View {
        HStack(spacing: Space.xs) {
            Text(kind.uppercased()).foregroundStyle(.secondary)
            if let parent, !parent.isEmpty {
                Text("·").foregroundStyle(.secondary)
                if let openParent {
                    Button(parent.uppercased(), action: openParent)
                        .buttonStyle(.link)
                        .help("Open \(parent)")
                } else {
                    Text(parent.uppercased()).foregroundStyle(.secondary)
                }
            }
        }
        .font(.hubCaption.weight(.semibold))
        .lineLimit(1)
    }
}

extension EntityHeader where Chips == EmptyView {
    init(
        kind: String, parent: String? = nil, openParent: (() -> Void)? = nil, title: String,
        @ViewBuilder facts: () -> Facts
    ) {
        self.init(kind: kind, parent: parent, openParent: openParent, title: title, facts: facts) { EmptyView() }
    }
}

extension EntityHeader where Facts == Text {
    /// A header whose facts are one line of text, left out when empty.
    init(
        kind: String, parent: String? = nil, openParent: (() -> Void)? = nil, title: String, facts: [String?],
        @ViewBuilder chips: () -> Chips
    ) {
        let line = facts.compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
        self.init(kind: kind, parent: parent, openParent: openParent, title: title, facts: { Text(line) }, chips: chips)
    }
}
