import Foundation
import JobSearchHubCore
import SwiftUI

/// One company being researched by the triage agent, through the `hub`
/// command bundled in the app. It lives as long as the app, so switching
/// pages doesn't lose a run.
@MainActor
@Observable
final class CompanyResearch {
    enum State: Equatable {
        case idle
        case running
        case succeeded(companyID: UUID?)
        case failed(String)
    }

    private(set) var state: State = .idle
    private(set) var company = ""
    private(set) var foundVia = ""
    private(set) var lines: [String] = []
    /// Bumps when a run finishes, so the companies list reads again.
    private(set) var revision = 0
    @ObservationIgnored private var process: Process?
    @ObservationIgnored private var unfinishedLine = ""

    var isRunning: Bool { state == .running }

    func start(company: String, foundVia: String, client: HubClient) {
        guard !isRunning else { return }
        self.company = company.trimmingCharacters(in: .whitespacesAndNewlines)
        self.foundVia = foundVia
        lines = []
        unfinishedLine = ""
        guard let command = Bundle.main.url(forResource: "hub", withExtension: nil) else {
            state = .failed("The app has no bundled hub command. Build it with Scripts/make-app.sh.")
            return
        }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else {
            state = .failed("Claude Code is not installed where its installers put it (~/.local/bin/claude, Homebrew).")
            return
        }

        let process = Process()
        process.executableURL = command
        process.arguments = CompanyResearchLaunch.makeArguments(company: self.company, foundVia: foundVia)
        process.environment = CompanyResearchLaunch.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let output = Pipe()
        process.standardOutput = output
        process.standardError = output
        output.fileHandleForReading.readabilityHandler = { handle in
            let data = handle.availableData
            guard !data.isEmpty else { return }
            let text = String(decoding: data, as: UTF8.self)
            Task { @MainActor in self.receive(text) }
        }
        process.terminationHandler = { finished in
            output.fileHandleForReading.readabilityHandler = nil
            let rest = String(decoding: output.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            let status = finished.terminationStatus
            Task { @MainActor in self.finish(rest: rest, status: status) }
        }
        do {
            try process.run()
            self.process = process
            state = .running
        } catch {
            state = .failed("Could not start the research: \(error.localizedDescription)")
        }
    }

    func stop() {
        process?.terminate()
    }

    /// Back to the form, keeping what was typed, for a retry or another company.
    func reset() {
        guard !isRunning else { return }
        state = .idle
    }

    private func receive(_ text: String) {
        let pieces = (unfinishedLine + text).components(separatedBy: "\n")
        unfinishedLine = pieces.last ?? ""
        lines.append(contentsOf: pieces.dropLast())
    }

    private func finish(rest: String, status: Int32) {
        receive(rest + "\n")
        process = nil
        revision += 1
        if status == 0 {
            state = .succeeded(companyID: lines.lazy.compactMap(CompanyResearchLaunch.parseCompanyID).last)
        } else {
            state = .failed(lines.last { !$0.isEmpty } ?? "The research stopped (exit status \(status)).")
        }
    }
}

/// The form that starts a company's research, then its progress.
struct AddCompanySheet: View {
    let client: HubClient
    /// The name of a company in the list, for saying which one was added.
    let getCompanyName: (UUID) -> String?
    let onShowCompany: (UUID) -> Void
    @Environment(CompanyResearch.self) private var research
    @Environment(\.dismiss) private var dismiss
    @State private var company = ""
    @State private var foundVia = ""
    @FocusState private var isCompanyFieldFocused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Add a company").font(.title3.weight(.semibold))
            if research.state == .idle {
                form
            } else {
                progress
            }
        }
        .padding(20)
        .frame(width: 640)
        .onAppear {
            if research.state != .idle {
                company = research.company
                foundVia = research.foundVia
            }
        }
    }

    private var form: some View {
        VStack(alignment: .leading, spacing: 12) {
            TextField("Company", text: $company, prompt: Text("A name, its site, or a careers or posting link"))
                .focused($isCompanyFieldFocused)
                .onAppear { isCompanyFieldFocused = true }
            TextField("Found via", text: $foundVia, prompt: Text("How you came across it, e.g. a referral (optional)"))
            Text("An agent researches it: what it does, where it lists its jobs, and who to reach there. It takes a few minutes, and its jobs follow.")
                .font(.callout)
                .foregroundStyle(.secondary)
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                Button("Research") { research.start(company: company, foundVia: foundVia, client: client) }
                    .keyboardShortcut(.defaultAction)
                    .disabled(company.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        }
    }

    private var progress: some View {
        VStack(alignment: .leading, spacing: 12) {
            status
            ScrollViewReader { scroller in
                ScrollView {
                    VStack(alignment: .leading, spacing: 2) {
                        ForEach(Array(research.lines.enumerated()), id: \.offset) { entry in
                            Text(entry.element).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                                .id(entry.offset)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(8)
                }
                .frame(height: 320)
                .background(.quinary, in: RoundedRectangle(cornerRadius: 8))
                .onChange(of: research.lines.count) { scroller.scrollTo(research.lines.count - 1, anchor: .bottom) }
            }
            HStack {
                Spacer()
                switch research.state {
                case .running:
                    Button("Stop") { research.stop() }
                    Button("Close") { dismiss() }
                        .help("The research goes on; the Companies page shows when it ends.")
                case let .succeeded(companyID):
                    Button("Add another") {
                        company = ""
                        foundVia = ""
                        research.reset()
                    }
                    if let companyID {
                        Button("Show company") {
                            onShowCompany(companyID)
                            research.reset()
                            dismiss()
                        }
                        .keyboardShortcut(.defaultAction)
                    }
                case .failed:
                    Button("Close") { dismiss() }
                    Button("Try again") { research.reset() }
                        .keyboardShortcut(.defaultAction)
                case .idle:
                    EmptyView()
                }
            }
        }
    }

    @ViewBuilder
    private var status: some View {
        switch research.state {
        case .running:
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text("Researching \(research.company)…")
            }
        case let .succeeded(companyID):
            let name = companyID.flatMap(getCompanyName) ?? research.company
            Label("Added \(name).", systemImage: "checkmark.circle.fill").foregroundStyle(.green)
        case let .failed(reason):
            Label(reason, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
        case .idle:
            EmptyView()
        }
    }
}
