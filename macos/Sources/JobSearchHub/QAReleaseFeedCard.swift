import JobSearchHubCore
import SwiftUI

/// Settings › Version in QA mode: where the app reads its releases. QA
/// types its stub feed's URL here once it knows the port, since Symphony's
/// launcher passes the app no environment, then checks against it.
struct QAReleaseFeedCard: View {
    @Bindable var checker: NewVersionChecker

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            Text("Release feed (QA)").font(.hubSection)
            HStack(spacing: Space.s) {
                TextField("Release feed", text: $checker.qaFeedText, prompt: Text("http://localhost:8765/releases.json"))
                    .textFieldStyle(.roundedBorder)
                    .labelsHidden()
                    .onSubmit { checker.checkNow() }
                Button("Check now") { checker.checkNow() }
                    .disabled(checker.facts.isChecking)
            }
            Text(detail).font(.hubSecondary).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .hubCard()
    }

    private var detail: String {
        if ReleaseFeed.feedURL(checker.qaFeedText) != nil {
            return "Checks read the releases from this URL."
        }
        if checker.qaFeedText.trimmingCharacters(in: .whitespaces).isEmpty {
            return "Checks read GitHub's releases. Type a stub feed's http(s) URL to check against it instead."
        }
        return "Not an http(s) URL, so checks read GitHub's releases."
    }
}
