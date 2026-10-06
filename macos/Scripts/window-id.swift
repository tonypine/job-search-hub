// Prints the id of the largest on-screen window owned by the named app, for
// `screencapture -l`; with a process id, only that process's windows count.
import CoreGraphics

let owner = CommandLine.arguments[1]
let pid = CommandLine.arguments.count > 2 ? Int(CommandLine.arguments[2]) : nil
let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as? [[String: Any]] ?? []
var largest: (id: Int, area: Double)?
for window in windows where (window[kCGWindowOwnerName as String] as? String) == owner {
    if let pid, (window[kCGWindowOwnerPID as String] as? Int) != pid {
        continue
    }
    let bounds = window[kCGWindowBounds as String] as? [String: Double] ?? [:]
    let area = (bounds["Width"] ?? 0) * (bounds["Height"] ?? 0)
    if area > (largest?.area ?? 0), let id = window[kCGWindowNumber as String] as? Int {
        largest = (id, area)
    }
}
if let largest { print(largest.id) }
