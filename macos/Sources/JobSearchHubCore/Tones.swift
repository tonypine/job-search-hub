import Foundation

/// What a color means. Every status color in the app comes from one of these,
/// so the same state looks the same wherever it shows.
public enum Tone: String, CaseIterable, Sendable {
    /// You and your actions: selection, the primary button, a Possible match, unseen.
    case accent
    /// Strong match, passes a screen, heard back.
    case positive
    /// Stretch match, unclear, due today, stale brief, waiting for you.
    case caution
    /// Fails a screen, overdue, errors.
    case negative
    /// Mismatch, skipped, closed, relations, metadata.
    case neutral

    /// A run's outcome as the hub records it: "succeeded", "running",
    /// "invalid" (the model answered, but not in the shape asked for) or
    /// "failed".
    public static func ofRunOutcome(_ outcome: String) -> Tone {
        switch outcome {
        case "succeeded": .positive
        case "running": .accent
        case "invalid": .caution
        default: .negative
        }
    }
}

public extension JobMatch {
    var tone: Tone {
        switch self {
        case .strong: .positive
        case .possible: .accent
        case .stretch: .caution
        case .mismatch: .neutral
        }
    }

    /// The match's symbol beside its word: fuller stars for better matches.
    var symbolName: String {
        switch self {
        case .strong: "star.fill"
        case .possible: "star.leadinghalf.filled"
        case .stretch: "star"
        case .mismatch: "minus.circle"
        }
    }
}

/// The screen: does a rule rule the job out? Good passes, poor fails.
public extension FitLevel {
    var tone: Tone {
        switch self {
        case .good: .positive
        case .unclear: .caution
        case .poor: .negative
        }
    }
}

public extension FitVerdict {
    var tone: Tone {
        switch self {
        case .yes: .positive
        case .unclear: .caution
        case .no: .negative
        }
    }

    var symbolName: String {
        switch self {
        case .yes: "checkmark.circle.fill"
        case .unclear: "questionmark.circle.fill"
        case .no: "xmark.circle.fill"
        }
    }
}

public extension FollowUpStatus {
    var tone: Tone {
        switch self {
        case .overdue: .negative
        case .dueToday: .caution
        case .dueIn: .neutral
        }
    }
}

/// A job or pipeline card out of the way: skipped as not for you, or closed
/// with an outcome. Each reads as neutral, told apart by its symbol.
public enum SetAside: CaseIterable, Sendable {
    case skipped, closed

    public var tone: Tone { .neutral }

    public var symbolName: String {
        switch self {
        case .skipped: "eye.slash"
        case .closed: "archivebox"
        }
    }
}

public extension SessionActivity {
    var tone: Tone {
        switch self {
        case .working: .accent
        case .blocked: .caution
        case .idle: .positive
        }
    }
}

public extension CVScreenVerdict {
    var tone: Tone {
        switch self {
        case .likelyPass: .positive
        case .borderline: .caution
        case .likelyReject: .negative
        }
    }
}

public extension ComparisonVerdictKind {
    var tone: Tone {
        switch self {
        case .right: .positive
        case .wrong: .negative
        }
    }
}

public extension ConnectionStatus {
    var tone: Tone {
        switch self {
        case .connected: .positive
        case .unchecked, .waitingForKeychain: .neutral
        case .missingToken, .serverUnreachable: .caution
        case .tokenRefused, .upgradeRequired, .failed: .negative
        }
    }
}

public extension ServerLaunchAgent.State {
    var tone: Tone {
        switch self {
        case .running: .positive
        case .waiting: .caution
        case .stopped: .neutral
        }
    }
}
