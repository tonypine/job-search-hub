import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class ApplicationAnswersModel {
    private(set) var answers: [ApplicationAnswer] = []
    var failure: HubFailure?
    private(set) var isWorking = false

    var missingCommonQuestions: [String] { CommonApplicationQuestions.findMissing(in: answers) }

    func load(with client: HubClient) async {
        do {
            answers = try await client.get("v1/application-answers", as: ApplicationAnswersResponse.self).answers
            failure = nil
        } catch {
            failure = HubFailure("Couldn't load the answers", error)
        }
    }

    /// Saves an answer: the one with id, or a new one.
    func save(id: UUID?, question: String, answer: String, with client: HubClient) async -> Bool {
        isWorking = true
        defer { isWorking = false }
        do {
            let request = SaveApplicationAnswerRequest(question: question, answer: answer)
            if let id {
                _ = try await client.send("PUT", "v1/application-answers/\(id)", body: request, as: ApplicationAnswer.self)
            } else {
                _ = try await client.send("POST", "v1/application-answers", body: request, as: ApplicationAnswer.self)
            }
            await load(with: client)
            return true
        } catch {
            failure = HubFailure("Couldn't save the answer", error)
            return false
        }
    }

    func delete(_ id: UUID, with client: HubClient) async {
        do {
            try await client.delete("v1/application-answers/\(id)")
            await load(with: client)
        } catch {
            failure = HubFailure("Couldn't delete the answer", error)
        }
    }

    /// Adds the common questions the library lacks, unanswered, to fill in.
    func addCommonQuestions(with client: HubClient) async {
        isWorking = true
        defer { isWorking = false }
        for question in missingCommonQuestions {
            _ = try? await client.send("POST", "v1/application-answers", body: SaveApplicationAnswerRequest(question: question, answer: ""), as: ApplicationAnswer.self)
        }
        await load(with: client)
    }
}

/// The owner's answers to application form questions, which agents answer
/// forms from.
struct ApplicationAnswersSection: View {
    let client: HubClient
    @State private var model = ApplicationAnswersModel()
    @State private var editing: EditedAnswer?

    struct EditedAnswer: Identifiable {
        let id = UUID()
        var answerID: UUID?
        var question: String
        var answer: String
    }

    var body: some View {
        HubSection("Answers for application forms") {
            Text("Agents helping with an application answer from these, the same way every time.")
                .foregroundStyle(.secondary)
            if model.failure != nil {
                HubErrorView($model.failure)
            }
            if model.answers.isEmpty {
                Text("No answers yet. Import your LinkedIn export's saved answers from the Settings page, or add common questions to fill in.")
                    .foregroundStyle(.secondary)
            }
            ForEach(model.answers) { answer in
                Button {
                    editing = EditedAnswer(answerID: answer.id, question: answer.question, answer: answer.answer)
                } label: {
                    VStack(alignment: .leading, spacing: Space.xs) {
                        Text(answer.question).fontWeight(.medium)
                        if answer.answer.isEmpty {
                            ToneChip("Not answered yet", tone: .caution)
                        } else {
                            Text(answer.answer).foregroundStyle(.secondary).lineLimit(3)
                        }
                    }
                    .hubWell()
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .contextMenu {
                    Button("Delete", role: .destructive) { Task { await model.delete(answer.id, with: client) } }
                }
            }
        } trailing: {
            HStack {
                if !model.missingCommonQuestions.isEmpty {
                    AsyncButton("Add common questions (\(model.missingCommonQuestions.count))", busyTitle: "Adding…", isBusy: model.isWorking) {
                        await model.addCommonQuestions(with: client)
                    }
                }
                Button("Add answer", systemImage: "plus") { editing = EditedAnswer(question: "", answer: "") }
            }
        }
        .task { await model.load(with: client) }
        .sheet(item: $editing) { edited in
            AnswerEditor(edited: edited, isSaving: model.isWorking) { question, answer in
                await model.save(id: edited.answerID, question: question, answer: answer, with: client)
            }
        }
    }
}

struct AnswerEditor: View {
    let edited: ApplicationAnswersSection.EditedAnswer
    let isSaving: Bool
    let onSave: (String, String) async -> Bool
    @Environment(\.dismiss) private var dismiss
    @State private var question = ""
    @State private var answer = ""

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text(edited.answerID == nil ? "Add an answer" : "Edit the answer").font(.hubSection)
            TextField("Question", text: $question, prompt: Text("As forms ask it, e.g. Notice period"))
            TextEditor(text: $answer)
                .font(.body)
                .frame(minHeight: 120)
                .padding(Space.xs)
                .background(.quinary, in: RoundedRectangle(cornerRadius: Radius.control))
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                AsyncButton("Save", busyTitle: "Saving…", isBusy: isSaving) {
                    if await onSave(question, answer) { dismiss() }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(question.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        }
        .padding(Space.xl)
        .frame(width: 520)
        .onAppear {
            question = edited.question
            answer = edited.answer
        }
    }
}
