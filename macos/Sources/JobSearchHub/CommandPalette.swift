import JobSearchHubCore
import SwiftUI

/// What the ⌘K palette searches: the open jobs, the companies, the people
/// and the pages. Read each time the palette opens; what was read before
/// shows meanwhile.
@MainActor
@Observable
final class PaletteModel {
    private var jobs: [PaletteItem] = []
    private var companies: [PaletteItem] = []
    private var people: [PaletteItem] = []
    private(set) var items: [PaletteItem] = Page.allCases.map(PaletteItem.page)
    /// Whether the local models' background work is paused, for Pause or
    /// Resume; nil until read.
    private(set) var isModelWorkPaused: Bool?
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    /// Moves when the items do, so the palette ranks them again.
    private(set) var revision = 0

    /// Reads the four lists. One that fails keeps what it had, and the error
    /// says so.
    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        async let readJobs = client.getAllJobs(search: "", status: .open)
        async let readCompanies = client.get("v1/companies", as: CompaniesResponse.self)
        async let readPeople = client.get("v1/people", as: PeopleResponse.self)
        async let readWork = client.get("v1/model-work", as: ModelWork.self)
        var failure: (any Error)?
        do { jobs = JobsOrder.sort(try await readJobs.jobs).map(PaletteItem.job) } catch { failure = failure ?? error }
        do { companies = try await readCompanies.companies.map(PaletteItem.company) } catch { failure = failure ?? error }
        do { people = try await readPeople.people.map(PaletteItem.person) } catch { failure = failure ?? error }
        do { isModelWorkPaused = try await readWork.paused } catch { failure = failure ?? error }
        items = jobs + companies + people + Page.allCases.map(PaletteItem.page)
        revision += 1
        // Closing the palette cancels the reads, which failed nothing.
        if !Task.isCancelled {
            loadError = failure.map { HubFailure("Couldn't read everything to search", $0) }
        }
    }
}

/// Jump anywhere: a field that finds jobs, companies, people and pages, and
/// the rare actions kept out of the toolbars. ↑↓ move, Return chooses, Esc
/// closes.
struct CommandPalette: View {
    let model: PaletteModel
    let actions: [PaletteAction]
    let choose: (PaletteItem) -> Void
    let close: () -> Void
    @State private var query = ""
    /// The rows for the query, ranked again only when it or the items
    /// change, not on every hover.
    @State private var sections: [PaletteSection] = []
    @State private var selectedID: PaletteTarget?
    @FocusState private var isFieldFocused: Bool

    var body: some View {
        let rows = sections.flatMap(\.items)
        let selected = rows.first { $0.id == selectedID } ?? rows.first
        VStack(spacing: 0) {
            field(rows: rows, selected: selected)
            Divider()
            if let loadError = model.loadError {
                HubErrorView(loadError).padding(Space.m)
            }
            if rows.isEmpty {
                Text(model.isLoading ? "Reading the jobs, companies and people…" : "Nothing matches “\(query)”")
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
                    .padding(Space.xl)
            } else {
                results(sections, selected: selected)
            }
            Divider()
            footer
        }
        .frame(width: 640)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(.separator))
        .shadow(color: .black.opacity(0.18), radius: 24, y: 8)
        .onAppear { isFieldFocused = true }
        .onExitCommand(perform: close)
        .onChange(of: query, initial: true) {
            selectedID = nil
            rank()
        }
        .onChange(of: model.revision) { rank() }
        .onChange(of: actions) { rank() }
    }

    private func rank() {
        sections = PaletteSearch.rank(model.items + actions.map(PaletteItem.action), query: query)
    }

    private func field(rows: [PaletteItem], selected: PaletteItem?) -> some View {
        HStack(spacing: Space.s) {
            Image(systemName: "magnifyingglass").foregroundStyle(.secondary).accessibilityHidden(true)
            TextField("Jump to a job, company or person", text: $query)
                .textFieldStyle(.plain)
                .font(.title3)
                .focused($isFieldFocused)
                .accessibilityLabel("Jump to")
                .onSubmit {
                    if let selected { choose(selected) }
                }
                .onKeyPress(.downArrow) {
                    selectedID = step(from: selected, by: 1, in: rows)
                    return .handled
                }
                .onKeyPress(.upArrow) {
                    selectedID = step(from: selected, by: -1, in: rows)
                    return .handled
                }
                .onKeyPress(.escape) {
                    close()
                    return .handled
                }
            if model.isLoading {
                ProgressView().controlSize(.small)
            }
            KeyCap("esc")
        }
        .padding(.horizontal, Space.l)
        .padding(.vertical, Space.m)
    }

    private func results(_ sections: [PaletteSection], selected: PaletteItem?) -> some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 2) {
                    ForEach(sections) { section in
                        Text(section.kind.title)
                            .font(.hubCaption.weight(.semibold))
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, Space.s)
                            .padding(.top, Space.s)
                        ForEach(section.items) { item in
                            row(item, isSelected: item.id == selected?.id)
                                .id(item.id)
                        }
                    }
                }
                .padding(Space.s)
            }
            .frame(maxHeight: 420)
            .fixedSize(horizontal: false, vertical: true)
            .onChange(of: selected?.id) {
                if let id = selected?.id { proxy.scrollTo(id) }
            }
        }
    }

    private func row(_ item: PaletteItem, isSelected: Bool) -> some View {
        Button {
            choose(item)
        } label: {
            HStack(spacing: Space.s) {
                Image(systemName: item.symbolName)
                    .foregroundStyle(isSelected ? Tone.accent.color : .secondary)
                    .frame(width: 20)
                    .accessibilityHidden(true)
                Text(item.title).lineLimit(1).layoutPriority(1)
                if let detail = item.detail {
                    Text(detail).foregroundStyle(.secondary).lineLimit(1)
                }
                if let chip = item.chip {
                    ToneChip(chip.text, tone: chip.tone)
                }
                Spacer(minLength: Space.s)
                if isSelected {
                    Text(item.kind == .action ? "↩ Run" : "↩ Open").font(.hubCaption).foregroundStyle(.secondary)
                }
            }
            .padding(.horizontal, Space.s)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                isSelected ? AnyShapeStyle(Tone.accent.fill) : AnyShapeStyle(.clear),
                in: RoundedRectangle(cornerRadius: Radius.control)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { isHovering in
            if isHovering { selectedID = item.id }
        }
        .accessibilityLabel([item.kind.title, item.title, item.detail].compactMap { $0 }.joined(separator: ", "))
    }

    private var footer: some View {
        HStack(spacing: Space.l) {
            HStack(spacing: Space.xs) { KeyCap("↑↓"); Text("move") }
            HStack(spacing: Space.xs) { KeyCap("↩"); Text("open in the inspector, or run") }
            HStack(spacing: Space.xs) { KeyCap("esc"); Text("close") }
            Spacer()
        }
        .font(.hubCaption)
        .foregroundStyle(.secondary)
        .padding(.horizontal, Space.l)
        .padding(.vertical, Space.s)
    }

    private func step(from selected: PaletteItem?, by offset: Int, in rows: [PaletteItem]) -> PaletteTarget? {
        guard !rows.isEmpty else { return nil }
        let index = selected.flatMap { item in rows.firstIndex { $0.id == item.id } } ?? 0
        return rows[min(max(index + offset, 0), rows.count - 1)].id
    }
}

/// A key as the palette's hints draw it.
private struct KeyCap: View {
    let key: String

    init(_ key: String) {
        self.key = key
    }

    var body: some View {
        Text(key)
            .font(.hubCaption.weight(.semibold))
            .foregroundStyle(.secondary)
            .padding(.horizontal, 5)
            .padding(.vertical, 1)
            .background(.quinary, in: RoundedRectangle(cornerRadius: 4))
            .overlay(RoundedRectangle(cornerRadius: 4).strokeBorder(.separator))
            .accessibilityHidden(true)
    }
}
