import Foundation

/// The value axis beside a window's charts: the counts its gridlines mark.
/// Runs and tokens come in whole numbers, so every gridline does too: from 0
/// in steps of 1, 2 or 5 times a power of ten, at most three of them, the
/// last at or above the highest bar. A chart whose busiest bucket holds one
/// run reads 0 and 1, never a half-way line that a whole-number label rounds
/// to 0.
public enum RunValueAxis {
    /// The most steps above 0, so the axis keeps to a few labels.
    public static let maxSteps = 3

    /// The gridlines of a chart whose highest bar reaches this count, 0
    /// first. The last is the top of the chart's scale. An empty chart reads
    /// 0 and 1.
    public static func getTicks(max: Int) -> [Int] {
        let highest = Swift.max(max, 1)
        var power = 1
        while true {
            for multiple in [1, 2, 5] {
                let step = multiple * power
                let steps = (highest + step - 1) / step
                if steps <= maxSteps {
                    return (0 ... steps).map { $0 * step }
                }
            }
            power *= 10
        }
    }
}
