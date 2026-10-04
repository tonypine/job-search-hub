import AppKit
import JobSearchHubCore
import UserNotifications

/// Keeps the hub's event stream open while the app runs: each update raises
/// a notification, and bumps `revision` so pages refresh what they show.
/// A dropped stream reconnects, resuming after the last event it heard.
@MainActor
@Observable
final class HubEventStream {
    private(set) var revision = 0
    private(set) var isConnected = false
    @ObservationIgnored private var lastEventID: String?

    func run(with client: HubClient) async {
        _ = try? await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound])
        var backoff: Duration = .seconds(2)
        while !Task.isCancelled {
            do {
                let lines = try await client.openEventStream("v1/events", lastEventID: lastEventID)
                isConnected = true
                backoff = .seconds(2)
                var parser = ServerSentEventParser()
                for try await line in lines {
                    if let event = parser.consume(line) {
                        handle(event)
                    }
                }
            } catch {
                // The next attempt resumes from the last event heard.
            }
            isConnected = false
            try? await Task.sleep(for: backoff)
            backoff = min(backoff * 2, .seconds(30))
        }
    }

    /// Has every page read again, as an update would: View › Refresh, for
    /// when the stream missed something.
    func requestRefresh() {
        revision += 1
    }

    private func handle(_ event: ServerSentEvent) {
        if let id = event.id {
            lastEventID = id
        }
        guard event.name == "update", let update = try? HubJSON.makeDecoder().decode(HubUpdate.self, from: Data(event.data.utf8)) else { return }
        revision += 1
        let content = UNMutableNotificationContent()
        content.title = update.title
        if let subject = update.subject {
            content.subtitle = subject
        }
        content.body = update.body ?? ""
        content.sound = .default
        content.userInfo = ["jobID": update.jobID?.uuidString ?? "", "companyID": update.companyID?.uuidString ?? ""]
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: update.id.uuidString, content: content, trigger: nil))
    }
}
