import JobSearchHubCore
import SwiftUI

/// The companies the hub doesn't hold that the owner follows on LinkedIn, or
/// that startups.gallery lists as remote with a fitting job open, best first;
/// researching one runs Add company.
struct CompanySuggestionsSheet: View {
    let client: HubClient
    let onResearch: (CompanySuggestion) -> Void
    @Environment(CompanyResearch.self) private var research
    @Environment(\.dismiss) private var dismiss
    @State private var suggestions: [CompanySuggestion] = []
    @State private var loadError: HubFailure?
    @State private var isLoading = true

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text("Suggestions").font(.hubSection)
            List(suggestions) { suggestion in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(suggestion.organization).fontWeight(.medium)
                        if !suggestion.reason.isEmpty {
                            Text(suggestion.reason).font(.hubCaption)
                                .foregroundStyle(suggestion.fittingJobs > 0 ? AnyShapeStyle(Tone.positive.color) : AnyShapeStyle(.secondary))
                        }
                        Text(suggestion.origin).font(.hubCaption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button("Research") { onResearch(suggestion) }
                        .disabled(research.isRunning)
                }
                .padding(.vertical, 2)
            }
            .frame(height: 380)
            .overlay {
                if let loadError {
                    HubErrorView(loadError, style: .page, retry: { Task { await load() } })
                } else if suggestions.isEmpty && !isLoading {
                    ContentUnavailableView(
                        "No suggestions", systemImage: "sparkles",
                        description: Text("Import your LinkedIn archive in Settings › Accounts; the companies you follow show up here, beside the ones on startups.gallery's remote list with a job that passes your screen, read weekly.")
                    )
                }
            }
            HStack(alignment: .firstTextBaseline) {
                Text("Ranked by their open jobs that pass your screen and the people you know there. Researching one costs an agent run.")
                    .font(.hubCaption).foregroundStyle(.secondary)
                Spacer()
                Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
            }
        }
        .padding(Space.xl)
        .frame(width: 560)
        .task { await load() }
    }

    private func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            suggestions = try await client.get("v1/company-suggestions", as: CompanySuggestionsResponse.self).suggestions
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the suggestions", error)
        }
    }
}
