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
    @State private var loadError: String?
    @State private var isLoading = true

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Suggestions").font(.title3.weight(.semibold))
            Text("Companies you follow on LinkedIn, and remote ones on startups.gallery with a fitting job open, ranked by the fitting jobs they have open and the people you know there. Researching one costs an agent run, so pick the ones worth watching.")
                .font(.callout).foregroundStyle(.secondary)
            List(suggestions) { suggestion in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(suggestion.organization).fontWeight(.medium)
                        if !suggestion.reason.isEmpty {
                            Text(suggestion.reason).font(.caption).foregroundStyle(suggestion.fittingJobs > 0 ? .green : .secondary)
                        }
                        Text(suggestion.origin).font(.caption).foregroundStyle(.secondary)
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
                    ContentUnavailableView("Could not load suggestions", systemImage: "exclamationmark.triangle", description: Text(loadError))
                } else if suggestions.isEmpty && !isLoading {
                    ContentUnavailableView(
                        "No suggestions", systemImage: "sparkles",
                        description: Text("Import your LinkedIn archive in Settings › Network; the companies you follow show up here, beside the ones on startups.gallery's remote list with a fitting job, read weekly.")
                    )
                }
            }
            HStack {
                Spacer()
                Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
            }
        }
        .padding(20)
        .frame(width: 560)
        .task {
            defer { isLoading = false }
            do {
                suggestions = try await client.get("v1/company-suggestions", as: CompanySuggestionsResponse.self).suggestions
            } catch {
                loadError = String(describing: error)
            }
        }
    }
}
