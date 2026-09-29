import Foundation

/// The owner's answer to a question application forms ask; an empty answer
/// is a question still to answer.
public struct ApplicationAnswer: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var question: String
    public var answer: String
    public var source: String
    public var updatedAt: Date
}

public struct ApplicationAnswersResponse: Decodable, Sendable {
    public var answers: [ApplicationAnswer]
}

public struct SaveApplicationAnswerRequest: Encodable, Sendable {
    public var question: String
    public var answer: String

    public init(question: String, answer: String) {
        self.question = question
        self.answer = answer
    }
}

public enum CommonApplicationQuestions {
    /// Questions most application forms ask, offered to start the library.
    public static let all = [
        "Notice period", "Earliest start date", "Salary expectation", "Current or last salary", "Work authorization",
        "Do you need visa sponsorship?", "Willing to relocate?", "Preferred work arrangement (remote, hybrid, on-site)",
        "Timezone and working hours", "Years of experience", "English level", "Why are you looking for a new role?",
        "LinkedIn profile URL", "GitHub or portfolio URL", "How did you hear about us?",
    ]

    /// The common questions the library doesn't hold yet, by their words.
    public static func findMissing(in answers: [ApplicationAnswer]) -> [String] {
        let held = Set(answers.map { normalize($0.question) })
        return all.filter { !held.contains(normalize($0)) }
    }

    private static func normalize(_ question: String) -> String {
        question.lowercased().filter { $0.isLetter || $0.isNumber }
    }
}
