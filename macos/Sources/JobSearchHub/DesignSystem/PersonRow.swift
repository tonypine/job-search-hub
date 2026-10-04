import JobSearchHubCore
import SwiftUI

/// Someone who can get you in: their name, how they relate to you, their
/// role, and the actions that fit them.
struct PersonRow<Actions: View>: View {
    enum Relation: String {
        /// Found at the company by an agent.
        case contact = "Contact"
        /// A LinkedIn connection.
        case connection = "Connection"
        /// Someone elsewhere who can introduce you.
        case introducer = "Introducer"
        case recruiter = "Recruiter"
    }

    let name: String
    let relation: Relation
    var role: String?
    /// A second line: how close you are, how they can help.
    var detail: String?
    @ViewBuilder let actions: Actions

    init(
        _ name: String, relation: Relation, role: String? = nil, detail: String? = nil,
        @ViewBuilder actions: () -> Actions
    ) {
        self.name = name
        self.relation = relation
        self.role = role
        self.detail = detail
        self.actions = actions()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Text(name).fontWeight(.semibold).textSelection(.enabled)
                ToneChip(relation.rawValue, tone: .neutral)
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
    init(_ name: String, relation: Relation, role: String? = nil, detail: String? = nil) {
        self.init(name, relation: relation, role: role, detail: detail) { EmptyView() }
    }
}
