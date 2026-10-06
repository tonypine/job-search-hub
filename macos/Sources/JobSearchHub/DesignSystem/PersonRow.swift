import JobSearchHubCore
import SwiftUI

/// Someone who can get you in: their name, how they relate to you, their
/// role, and the actions that fit them.
struct PersonRow<Actions: View>: View {
    let name: String
    /// The relation chip: "Connection", or "Agency recruiter".
    let relationTitle: String
    var role: String?
    /// A second line: how close you are, how they can help.
    var detail: String?
    /// They wrote and you never answered.
    var isUnanswered = false
    /// Off inside a row that opens on a click, so the click isn't a selection.
    var isNameSelectable = true
    @ViewBuilder let actions: Actions

    init(
        _ name: String, relation: PersonRelation, role: String? = nil, detail: String? = nil,
        @ViewBuilder actions: () -> Actions
    ) {
        self.name = name
        relationTitle = relation.title
        self.role = role
        self.detail = detail
        self.actions = actions()
    }

    /// A person from the people list, with the line that says how they can
    /// help.
    init(_ person: RelatedPerson, detail: String? = nil, @ViewBuilder actions: () -> Actions) {
        name = person.name
        relationTitle = person.relationTitle
        role = person.role
        self.detail = detail
        isUnanswered = person.isUnanswered
        self.actions = actions()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                if isNameSelectable {
                    Text(name).fontWeight(.semibold).textSelection(.enabled)
                } else {
                    Text(name).fontWeight(.semibold)
                }
                ToneChip(relationTitle, tone: .neutral)
                if isUnanswered {
                    ToneChip("Unanswered", tone: .caution)
                }
            }
            if let role, !role.isEmpty {
                Text(role).font(.hubSecondary).foregroundStyle(.secondary)
            }
            if let detail, !detail.isEmpty {
                Text(detail).font(.hubSecondary).foregroundStyle(.secondary)
            }
            if Actions.self != EmptyView.self {
                HStack(spacing: Space.m) { actions }
                    .font(.hubCaption)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

extension PersonRow where Actions == EmptyView {
    init(_ name: String, relation: PersonRelation, role: String? = nil, detail: String? = nil) {
        self.init(name, relation: relation, role: role, detail: detail) { EmptyView() }
    }

    init(_ person: RelatedPerson, detail: String? = nil) {
        self.init(person, detail: detail) { EmptyView() }
    }
}

/// A person from the people list as a row that opens them in the
/// inspector.
struct PersonLinkRow: View {
    let person: RelatedPerson
    @Environment(DetailsInspector.self) private var inspector

    var body: some View {
        Button {
            inspector.open(.person(person.reference))
        } label: {
            HStack(alignment: .center, spacing: Space.s) {
                row
                Spacer(minLength: 0)
                Image(systemName: "chevron.forward").foregroundStyle(.tertiary)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private var row: some View {
        var row = PersonRow<EmptyView>(person, detail: describe(person))
        row.isNameSelectable = false
        return row
    }

    /// How they can help: an introducer's note, a connection's closeness, a
    /// contact's relevance, or when a recruiter last wrote.
    private func describe(_ person: RelatedPerson) -> String? {
        switch person.relation {
        case .introducer: [person.note, person.preferredChannel.map { "prefers \($0)" }].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
        case .connection: person.closeness
        case .contact: person.relevanceTitle
        case .recruiter: person.lastContactAt.map { "wrote \($0.formatted(.relative(presentation: .named)))" }
        }
    }
}
