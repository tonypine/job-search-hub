import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class ApplicationAnswersModel {
    private(set) var answers: [ApplicationAnswer] = []
    private(set) var errorMessage: String?
    private(set) var isWorking = false

    var missingCommonQuestions: [String] { CommonApplicationQuestions.findMissing(in: answers) }

    func load(with client: HubClient) async {
        do {
            answers = try await client.get("v1/application-answers", as: ApplicationAnswersResponse.self).answers
            errorMessage = nil
        } catch {
            errorMessage = "Could not load the answers: \(error)"
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
            errorMessage = "Could not save the answer: \(error)"
            return false
        }
    }

    func delete(_ id: UUID, with client: HubClient) async {
        do {
            try await client.delete("v1/application-answers/\(id)")
            await load(with: client)
        } catch {
            errorMessage = "Could not delete the answer: \(error)"
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
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Answers for application forms").font(.title3.weight(.semibold))
                    Text("Agents helping with an application answer from these, the same way every time.")
                        .foregroundStyle(.secondary)
                }
                Spacer()
                if !model.missingCommonQuestions.isEmpty {
                    Button("Add common questions (\(model.missingCommonQuestions.count))") { Task { await model.addCommonQuestions(with: client) } }
                        .disabled(model.isWorking)
                }
                Button("Add answer", systemImage: "plus") { editing = EditedAnswer(question: "", answer: "") }
            }
            if let errorMessage = model.errorMessage {
                Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
            }
            if model.answers.isEmpty {
                Text("No answers yet. Import your LinkedIn export's saved answers from the Settings page, or add common questions to fill in.")
                    .foregroundStyle(.secondary)
            }
            ForEach(model.answers) { answer in
                Button {
                    editing = EditedAnswer(answerID: answer.id, question: answer.question, answer: answer.answer)
                } label: {
                    VStack(alignment: .leading, spacing: 3) {
                        Text(answer.question).fontWeight(.medium)
                        if answer.answer.isEmpty {
                            Text("Not answered yet").italic().foregroundStyle(.orange)
                        } else {
                            Text(answer.answer).foregroundStyle(.secondary).lineLimit(3)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(10)
                    .background(.quinary, in: RoundedRectangle(cornerRadius: 8))
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .contextMenu {
                    Button("Delete", role: .destructive) { Task { await model.delete(answer.id, with: client) } }
                }
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
        VStack(alignment: .leading, spacing: 12) {
            Text(edited.answerID == nil ? "Add an answer" : "Edit the answer").font(.title3.weight(.semibold))
            TextField("Question", text: $question, prompt: Text("As forms ask it, e.g. Notice period"))
            TextEditor(text: $answer)
                .font(.body)
                .frame(minHeight: 120)
                .padding(4)
                .background(.quinary, in: RoundedRectangle(cornerRadius: 6))
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                Button("Save") {
                    Task {
                        if await onSave(question, answer) { dismiss() }
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(question.trimmingCharacters(in: .whitespaces).isEmpty || isSaving)
            }
        }
        .padding(20)
        .frame(width: 520)
        .onAppear {
            question = edited.question
            answer = edited.answer
        }
    }
}
