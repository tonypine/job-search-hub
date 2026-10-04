import JobSearchHubCore
import SwiftUI

/// A pursued job's interview pack: each likely question with why the posting
/// raises it, the confirmed cases to tell and how to answer, then honest
/// answers for the role's market gaps.
struct InterviewPackSection: View {
    let jobID: UUID
    let client: HubClient
    @State private var pack: InterviewPack?
    @State private var isLoaded = false

    var body: some View {
        HubSection("Interview pack") {
            if let pack {
                ForEach(Array(pack.pack.questions.enumerated()), id: \.offset) { _, question in
                    VStack(alignment: .leading, spacing: Space.xs) {
                        Text(question.question).fontWeight(.medium)
                        Text(question.reason).font(.hubCaption).foregroundStyle(.secondary)
                        ForEach(question.stories, id: \.entryID) { story in
                            Label(story.title, systemImage: "text.book.closed").font(.hubSecondary)
                        }
                        Text(question.talkingPoints).font(.hubSecondary)
                    }
                }
                if !pack.pack.roleGaps.isEmpty {
                    Text("Gaps this role asks about").font(.hubSecondary.weight(.semibold)).foregroundStyle(.secondary).padding(.top, Space.xs)
                    ForEach(Array(pack.pack.roleGaps.enumerated()), id: \.offset) { _, gap in
                        Text("\(Text("\(gap.gap): ").fontWeight(.medium))\(gap.honestAnswer)").font(.hubSecondary)
                    }
                }
                Text("Prepared \(pack.createdAt.formatted(.relative(presentation: .named))). It's prepared again when the knowledge base changes.")
                    .font(.hubCaption).foregroundStyle(.secondary)
            } else if isLoaded {
                Text("The local model prepares the pack within a few minutes of pursuing the job.").foregroundStyle(.secondary)
            } else {
                ProgressView().controlSize(.small)
            }
        }
        .task(id: jobID) {
            pack = try? await client.getInterviewPack(jobID)
            isLoaded = true
        }
    }
}
