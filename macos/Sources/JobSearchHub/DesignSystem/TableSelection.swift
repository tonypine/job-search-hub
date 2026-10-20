import AppKit
import SwiftUI

extension View {
    /// The selected row of the Table this modifies in a light accent fill
    /// with an accent edge, in place of the solid accent fill AppKit draws,
    /// on which tone chips, accent text and monograms almost disappear.
    /// `revision` is whatever changes with the rows or the selection, so the
    /// highlight follows a row that moves. `cellSpacing`, when given, is the
    /// room between the cells of neighboring columns in place of AppKit's
    /// 17 pt, for a table whose columns' widths hold their own margins.
    func hubTableSelection(revision: some Hashable, cellSpacing: CGFloat? = nil) -> some View {
        background(TableSelectionStyle(revision: AnyHashable(revision), cellSpacing: cellSpacing))
    }
}

/// Finds the table next to it, turns off AppKit's selection highlight and
/// lays a SelectionHighlightView over the table's rows.
private struct TableSelectionStyle: NSViewRepresentable {
    let revision: AnyHashable
    let cellSpacing: CGFloat?

    func makeNSView(context: Context) -> TableFinderView {
        TableFinderView()
    }

    func updateNSView(_ view: TableFinderView, context: Context) {
        view.cellSpacing = cellSpacing
        DispatchQueue.main.async { [weak view] in view?.attach() }
    }
}

private final class TableFinderView: NSView {
    var cellSpacing: CGFloat?
    private weak var highlight: SelectionHighlightView?

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        DispatchQueue.main.async { [weak self] in self?.attach() }
    }

    /// Attaches the highlight to the table once, and redraws it after.
    func attach() {
        if let highlight, highlight.tableView != nil {
            highlight.cellSpacing = cellSpacing
            highlight.refresh()
            return
        }
        guard window != nil, let tableView = findTable() else { return }
        let highlight = SelectionHighlightView(tableView: tableView)
        highlight.cellSpacing = cellSpacing
        tableView.addSubview(highlight)
        self.highlight = highlight
        highlight.refresh()
    }

    /// The nearest table: the one under the closest ancestor that holds one.
    private func findTable() -> NSTableView? {
        var ancestor = superview
        while let view = ancestor {
            if let table = Self.findTable(in: view) { return table }
            ancestor = view.superview
        }
        return nil
    }

    private static func findTable(in view: NSView) -> NSTableView? {
        if let table = view as? NSTableView { return table }
        for subview in view.subviews {
            if let table = findTable(in: subview) { return table }
        }
        return nil
    }
}

/// Draws the selected rows of its table over them: a light accent fill and an
/// accent edge. It takes no clicks, so the rows under it still do. It also
/// keeps the table's cell spacing at `cellSpacing`, once the table has a
/// width to fit its columns to.
private final class SelectionHighlightView: NSView {
    private(set) weak var tableView: NSTableView?
    var cellSpacing: CGFloat?

    init(tableView: NSTableView) {
        self.tableView = tableView
        super.init(frame: tableView.bounds)
        autoresizingMask = [.width, .height]
        tableView.selectionHighlightStyle = .none
        tableView.postsFrameChangedNotifications = true
        let center = NotificationCenter.default
        center.addObserver(self, selector: #selector(changed), name: NSTableView.selectionDidChangeNotification, object: tableView)
        center.addObserver(self, selector: #selector(changed), name: NSView.frameDidChangeNotification, object: tableView)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is not supported")
    }

    deinit {
        NotificationCenter.default.removeObserver(self)
    }

    /// Keeps AppKit's highlight off and the cell spacing, either of which
    /// SwiftUI may set back, keeps the view above the rows, and redraws.
    func refresh() {
        guard let tableView else { return }
        if tableView.selectionHighlightStyle != .none {
            tableView.selectionHighlightStyle = .none
        }
        if let cellSpacing {
            tableView.setCellSpacing(cellSpacing)
        }
        if tableView.subviews.last !== self {
            tableView.addSubview(self, positioned: .above, relativeTo: nil)
        }
        if frame != tableView.bounds {
            frame = tableView.bounds
        }
        needsDisplay = true
    }

    @objc private func changed() {
        refresh()
    }

    /// Matches the table, which is flipped, so a row's rect lands on the row
    /// rather than mirrored from the bottom of the table.
    override var isFlipped: Bool {
        true
    }

    override func hitTest(_ point: NSPoint) -> NSView? {
        nil
    }

    override func viewDidChangeEffectiveAppearance() {
        super.viewDidChangeEffectiveAppearance()
        needsDisplay = true
    }

    override func draw(_ dirtyRect: NSRect) {
        guard let tableView else { return }
        let accent = NSColor(Color.hubAccent)
        for row in tableView.selectedRowIndexes where row < tableView.numberOfRows {
            let rect = convert(tableView.rect(ofRow: row), from: tableView).insetBy(dx: Space.xs, dy: 1)
            guard rect.intersects(dirtyRect) else { continue }
            let path = NSBezierPath(roundedRect: rect.insetBy(dx: 0.5, dy: 0.5), xRadius: Radius.control, yRadius: Radius.control)
            accent.withAlphaComponent(0.13).setFill()
            path.fill()
            accent.setStroke()
            path.lineWidth = 1
            path.stroke()
        }
    }
}

extension NSTableView {
    /// Leaves `spacing` between the cells of neighboring columns and gives
    /// the room that frees, or takes, to the first column that can be
    /// resized, so the columns end as far from the table's right edge as they
    /// start from its left. Until the table has a width, it waits: there is
    /// nothing to fit the columns to.
    func setCellSpacing(_ spacing: CGFloat) {
        guard intercellSpacing.width != spacing, let clipView = enclosingScrollView?.contentView,
              clipView.bounds.width > 0
        else { return }
        intercellSpacing = NSSize(width: spacing, height: intercellSpacing.height)
        tile()
        let shown = tableColumns.indices.filter { !tableColumns[$0].isHidden }
        guard let first = shown.first, let last = shown.last,
              let resizable = shown.first(where: { tableColumns[$0].maxWidth > tableColumns[$0].minWidth })
        else { return }
        let column = tableColumns[resizable]
        let room = clipView.bounds.width - rect(ofColumn: first).minX - rect(ofColumn: last).maxX
        column.width = min(max(column.width + room, column.minWidth), column.maxWidth)
    }
}
