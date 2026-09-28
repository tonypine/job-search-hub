// Prints the id of the largest on-screen window owned by the named app, for
// `screencapture -l`.
import CoreGraphics

let owner = CommandLine.arguments[1]
let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as? [[String: Any]] ?? []
var largest: (id: Int, area: Double)?
for window in windows where (window[kCGWindowOwnerName as String] as? String) == owner {
    let bounds = window[kCGWindowBounds as String] as? [String: Double] ?? [:]
    let area = (bounds["Width"] ?? 0) * (bounds["Height"] ?? 0)
    if area > (largest?.area ?? 0), let id = window[kCGWindowNumber as String] as? Int {
        largest = (id, area)
    }
}
if let largest { print(largest.id) }
