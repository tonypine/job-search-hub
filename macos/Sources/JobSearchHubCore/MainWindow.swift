import CoreGraphics

/// The main window's sizes. Its pages need the minimum, and AppKit keeps a
/// resize from going under it, but the window server doesn't: while the
/// screen is locked it gives a new window about 102×98 points, and SwiftUI's
/// toolbar then never settles once the inspector adds its section to it.
public enum MainWindow {
    public static let minimumSize = CGSize(width: 900, height: 600)
    public static let defaultSize = CGSize(width: 1400, height: 860)

    /// Whether a window frame is big enough for the pages and the inspector.
    public static func hasRoom(_ size: CGSize) -> Bool {
        size.width >= minimumSize.width && size.height >= minimumSize.height
    }

    /// A frame too small for the pages, grown to the default size or as
    /// much of it as the screen's visible frame holds, keeping its top-left
    /// corner where the screen allows. Nil when the frame already has room,
    /// or when the screen can't hold the minimum.
    public static func grownFrame(_ frame: CGRect, within visibleFrame: CGRect) -> CGRect? {
        guard !hasRoom(frame.size), hasRoom(visibleFrame.size) else { return nil }
        let width = min(max(frame.width, defaultSize.width), visibleFrame.width)
        let height = min(max(frame.height, defaultSize.height), visibleFrame.height)
        // Screen coordinates grow upwards, so the top edge is maxY.
        let x = min(max(frame.minX, visibleFrame.minX), visibleFrame.maxX - width)
        let top = min(max(frame.maxY, visibleFrame.minY + height), visibleFrame.maxY)
        return CGRect(x: x, y: top - height, width: width, height: height)
    }
}
