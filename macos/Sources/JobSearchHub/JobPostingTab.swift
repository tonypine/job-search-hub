import JobSearchHubCore
import SwiftUI

/// A job's Posting tab: the key facts on one card, with who read them and
/// every fact behind its ⋯; then where the text came from, the posting's
/// outline and the marks' legend with Find (⌘F); then the posting to read,
/// with the phrases the screen quoted marked.
struct JobPostingTab: View {
    let jobID: UUID
    let details: JobDetails
    let client: HubClient
    let model: JobDetailModel
    /// Facts in a row, as the inspector fits them.
    private let columns = 2

    @State private var isShowingFacts = false
    @State private var isFinding = false
    @State private var query = ""
    @State private var matchIndex = 0
    /// The headings whose top has scrolled past the reading line.
    @State private var passedHeadings: Set<Int> = []
    @FocusState private var isQueryFocused: Bool

    var body: some View {
        let document = PostingDocument(markdown: details.job.description ?? "")
        let marks = document.findMarks(for: details.postingQuotes)
        let matches = isFinding ? document.findMatches(of: query) : []
        ScrollViewReader { proxy in
            VStack(alignment: .leading, spacing: Space.l) {
                keyFacts
                VStack(alignment: .leading, spacing: Space.m) {
                    sourceLine
                    if !document.blocks.isEmpty {
                        outline(document.outline, proxy: proxy)
                        HStack(spacing: Space.m) {
                            if !marks.isEmpty {
                                MarkLegend()
                            }
                            Spacer(minLength: 0)
                            findButton
                        }
                        if isFinding {
                            findField(matches, proxy: proxy)
                        }
                    }
                }
                if document.blocks.isEmpty {
                    Text("The board kept no text for this posting; the original has it.").foregroundStyle(.secondary)
                } else {
                    PostingReader(document: document, marks: marks, matches: matches) { block, isPassed in
                        if isPassed {
                            passedHeadings.insert(block)
                        } else {
                            passedHeadings.remove(block)
                        }
                    }
                }
            }
        }
    }

    // MARK: Key facts

    private var keyFacts: some View {
        let facts = details.keyFacts
        let rows = stride(from: 0, to: facts.count, by: columns).map { Array(facts[$0..<min($0 + columns, facts.count)]) }
        return VStack(alignment: .leading, spacing: Space.m) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Text("Key facts").font(.hubSection).accessibilityAddTraits(.isHeader)
                Spacer(minLength: 0)
                readingStatus
                Button("More about the facts", systemImage: "ellipsis.circle") { isShowingFacts.toggle() }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.borderless)
                    .help(details.facts == nil ? "Read facts now, and every fact from the board" : "Who read the facts, Read facts again, and every fact with its quote")
                    .popover(isPresented: $isShowingFacts, arrowEdge: .bottom) {
                        JobFactsPopover(jobID: jobID, details: details, client: client, model: model)
                    }
            }
            VStack(alignment: .leading, spacing: Space.m) {
                ForEach(Array(rows.enumerated()), id: \.offset) { _, row in
                    HStack(alignment: .top, spacing: Space.l) {
                        ForEach(row) { fact in
                            keyFact(fact)
                        }
                        // A short last row keeps its cells the width of the others.
                        ForEach(row.count..<columns, id: \.self) { _ in
                            Color.clear.frame(maxWidth: .infinity, maxHeight: 0)
                        }
                    }
                }
            }
        }
        .hubWell()
    }

    /// One fact: its title with where it came from, then the fact with the
    /// screen's verdict on it.
    private func keyFact(_ fact: KeyFact) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: Space.xs) {
                Text(fact.title).font(.hubCaption.weight(.medium)).foregroundStyle(.secondary)
                if let source = fact.source {
                    Image(systemName: source == .board ? "building.2" : "text.viewfinder")
                        .font(.hubCaption)
                        .foregroundStyle(.tertiary)
                        .help(source.label)
                        .accessibilityLabel(source.label)
                }
            }
            HStack(alignment: .firstTextBaseline, spacing: Space.xs) {
                if let verdict = fact.verdict {
                    Image(systemName: verdict.symbolName)
                        .imageScale(.small)
                        .foregroundStyle(verdict.tone.color)
                        .accessibilityLabel(verdict.spokenName)
                }
                Text(fact.text)
                    .foregroundStyle(fact.source == nil ? .secondary : .primary)
                    .lineLimit(3)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .help(fact.help ?? "")
        .accessibilityElement(children: .combine)
    }

    /// Where a Read facts run stands, while one is going.
    @ViewBuilder
    private var readingStatus: some View {
        if model.isReadingFacts {
            HStack(spacing: Space.xs) {
                ProgressView().controlSize(.mini)
                Text(model.factsReadState.statusText)
            }
            .font(.hubCaption)
            .foregroundStyle(.secondary)
        }
    }

    // MARK: Source and outline

    private var sourceLine: some View {
        HStack(spacing: Space.s) {
            Label(details.job.describeSource(companyName: details.companyName), systemImage: "doc.plaintext")
                .foregroundStyle(.secondary)
                .lineLimit(1)
            Spacer(minLength: 0)
            if let url = URL(string: details.job.url) {
                Link(destination: url) {
                    Text("Original \(Image(systemName: "arrow.up.right"))")
                }
                .help(details.job.url)
            }
        }
        .font(.hubCaption)
    }

    /// The posting's headings as chips that scroll to them; the last one
    /// scrolled past is marked. Marking changes only colors, so the chips
    /// never move as you scroll.
    @ViewBuilder
    private func outline(_ headings: [PostingHeading], proxy: ScrollViewProxy) -> some View {
        if !headings.isEmpty {
            let current = passedHeadings.max()
            FlowLayout {
                ForEach(headings) { heading in
                    let isCurrent = heading.block == current
                    Button {
                        withAnimation { proxy.scrollTo(PostingReader.blockID(heading.block), anchor: .top) }
                    } label: {
                        Text(heading.title)
                            .font(.hubCaption)
                            .lineLimit(1)
                            .padding(.horizontal, Space.s)
                            .padding(.vertical, 3)
                            .foregroundStyle(isCurrent ? Tone.accent.color : Color.secondary)
                            .background(isCurrent ? AnyShapeStyle(Tone.accent.fill) : AnyShapeStyle(.quinary), in: Capsule())
                            .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .help("Go to \(heading.title)")
                    .accessibilityAddTraits(isCurrent ? .isSelected : [])
                }
            }
        }
    }

    // MARK: Find

    private var findButton: some View {
        Button {
            isFinding = true
            isQueryFocused = true
        } label: {
            HStack(spacing: Space.xs) {
                Image(systemName: "magnifyingglass")
                Text("Find")
                Text("⌘F").foregroundStyle(.tertiary)
            }
        }
        .buttonStyle(.borderless)
        .font(.hubCaption)
        .keyboardShortcut("f", modifiers: .command)
        .help("Find in the posting")
    }

    /// The query, how many times it shows, and Done. Return goes to the next
    /// match; Escape closes it.
    private func findField(_ matches: [PostingSpan], proxy: ScrollViewProxy) -> some View {
        HStack(spacing: Space.s) {
            TextField("Find in the posting", text: $query)
                .textFieldStyle(.roundedBorder)
                .focused($isQueryFocused)
                .onAppear { isQueryFocused = true }
                .onSubmit { showNextMatch(matches, proxy: proxy) }
                .onExitCommand { closeFind() }
                .onChange(of: query) { matchIndex = 0 }
            if !query.isEmpty {
                Text(matches.isEmpty ? "No matches" : matches.count == 1 ? "1 match" : "\(matches.count) matches")
                    .font(.hubCaption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .fixedSize()
            }
            Button("Done") { closeFind() }
                .buttonStyle(.borderless)
        }
    }

    private func showNextMatch(_ matches: [PostingSpan], proxy: ScrollViewProxy) {
        guard !matches.isEmpty else { return }
        let match = matches[matchIndex % matches.count]
        matchIndex = (matchIndex + 1) % matches.count
        withAnimation { proxy.scrollTo(PostingReader.blockID(match.block), anchor: .center) }
        isQueryFocused = true
    }

    private func closeFind() {
        isFinding = false
        query = ""
        matchIndex = 0
    }
}

/// Behind the key facts' ⋯: who read the facts, when and with which prompt,
/// Read facts again (or now) with where its run stands, then every fact from
/// the board and every fact read with the posting's words behind it.
private struct JobFactsPopover: View {
    let jobID: UUID
    let details: JobDetails
    let client: HubClient
    let model: JobDetailModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Space.l) {
                VStack(alignment: .leading, spacing: Space.s) {
                    if let facts = details.facts {
                        Text("Read by \(facts.model) with prompt version \(facts.promptVersion), \(facts.extractedAt.formatted(date: .abbreviated, time: .shortened)).")
                            .foregroundStyle(.secondary)
                    } else {
                        Text("Not read yet. The hub reads new postings as they arrive, unless its model work is paused.")
                            .foregroundStyle(.secondary)
                    }
                    HStack(spacing: Space.s) {
                        AsyncButton(
                            details.facts == nil ? "Read facts now" : "Read facts again", busyTitle: "Reading…", systemImage: "arrow.clockwise",
                            isBusy: model.isReadingFacts
                        ) {
                            await model.readFactsNow(jobID, with: client)
                        }
                        if model.isReadingFacts {
                            Text(model.factsReadState.statusText).foregroundStyle(.secondary)
                        }
                    }
                }
                .fixedSize(horizontal: false, vertical: true)
                boardFacts
                if let facts = details.facts {
                    readFacts(facts)
                }
            }
            .padding(Space.l)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(width: 380, height: 460)
    }

    private var boardFacts: some View {
        let job = details.job
        return HubSection("From the board") {
            FactGrid {
                FactRow("Pay") {
                    if let pay = job.pay {
                        VStack(alignment: .leading, spacing: 2) {
                            ForEach(Array(pay.ranges.enumerated()), id: \.offset) { _, range in
                                Text([range.label, range.format()].compactMap { $0 }.joined(separator: ": "))
                            }
                            if let summary = pay.summary {
                                Text(summary).foregroundStyle(.secondary)
                            }
                        }
                    } else {
                        Text("Not published").foregroundStyle(.secondary)
                    }
                }
                FactRow("Location", text: job.location)
                FactRow("Workplace", text: job.workplaceType)
                FactRow("Employment", text: job.employmentType)
                FactRow("Department", text: job.department)
                FactRow("Also hiring in", text: job.otherLocations?.joined(separator: ", "))
                FactRow("Published", text: job.publishedAt?.formatted(date: .abbreviated, time: .omitted))
                FactRow("First seen", text: job.firstSeenAt.formatted(date: .abbreviated, time: .omitted))
            }
        }
    }

    private func readFacts(_ facts: LabelledJobFacts) -> some View {
        HubSection("Read from the posting") {
            FactGrid {
                ForEach(facts.entries) { entry in
                    FactRow(entry.title, help: entry.description) {
                        VStack(alignment: .leading, spacing: 2) {
                            switch entry.display {
                            case let .text(text): Text(text)
                            case let .list(items): Text(items.joined(separator: ", "))
                            case .notStated: Text("Not stated").foregroundStyle(.secondary)
                            }
                            if let evidence = entry.evidence {
                                Evidence(text: evidence)
                            }
                        }
                        .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
        }
    }
}

private extension JobFactsReadState {
    /// Where a Read facts run stands, as the card and the menu say it.
    var statusText: String {
        switch self {
        case .queued: "Waiting for its turn"
        case .running: "Reading now"
        case .notQueued: "Reading…"
        }
    }
}

private extension FitVerdict {
    /// The verdict's symbol, spoken.
    var spokenName: String {
        switch self {
        case .yes: "Passes the screen"
        case .unclear: "Unclear on the screen"
        case .no: "Fails the screen"
        }
    }
}
