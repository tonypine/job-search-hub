import Foundation

/// The list a job was opened from as a page (P2): its jobs in the list's
/// order and filters, where the job shown stands in it (*3 of 48*), and
/// which job ↑ and ↓ bring up. It remembers where the job shown last stood,
/// so ↑ and ↓ still go on from there once a decision takes it out of the
/// list.
public struct JobPageList: Equatable, Sendable {
    public private(set) var order: [UUID]
    /// The index the job shown last had in the list.
    public private(set) var lastIndex: Int?

    public init(order: [UUID] = []) {
        self.order = order
    }

    /// Takes the list's new order. The job shown keeps its place when it's
    /// still listed, and its last one when it left.
    public mutating func setOrder(_ order: [UUID], showing id: UUID?) {
        self.order = order
        if let id, let index = order.firstIndex(of: id) {
            lastIndex = index
        }
    }

    /// Notes where the job now shown stands.
    public mutating func show(_ id: UUID) {
        if let index = order.firstIndex(of: id) {
            lastIndex = index
        }
    }

    /// "3 of 48"; nil when the job isn't in the list.
    public func getPosition(of id: UUID) -> String? {
        guard let index = order.firstIndex(of: id) else { return nil }
        return "\(index + 1) of \(order.count)"
    }

    /// The job a step down (1) or up (-1) from the one shown; nil at either
    /// end. A job that left the list goes on from where it stood: the job
    /// that took its place is next, the one before it previous.
    public func getNeighbor(of id: UUID, by step: Int) -> UUID? {
        let target: Int
        if let index = order.firstIndex(of: id) {
            target = index + step
        } else if let lastIndex {
            target = step > 0 ? lastIndex + step - 1 : lastIndex + step
        } else {
            return nil
        }
        return order.indices.contains(target) ? order[target] : nil
    }
}
