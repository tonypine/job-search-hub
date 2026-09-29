import Foundation
import JobSearchHubCore

/// Tailscale's command line, for this Mac's address on the tailnet.
enum TailscaleCommand {
    /// Where Tailscale installs its command, then the app's own binary, which
    /// that command runs.
    private static let executablePaths = ["/usr/local/bin/tailscale", "/Applications/Tailscale.app/Contents/MacOS/Tailscale"]

    /// This Mac's HTTPS address on the tailnet, or nil when Tailscale isn't
    /// installed or connected, or the tailnet has no HTTPS certificates.
    static func readThisMacAddress() async -> String? {
        guard let path = executablePaths.first(where: FileManager.default.isExecutableFile(atPath:)) else {
            return nil
        }
        let status: Data? = await withCheckedContinuation { continuation in
            DispatchQueue.global().async {
                let process = Process()
                process.executableURL = URL(fileURLWithPath: path)
                process.arguments = ["status", "--json"]
                let output = Pipe()
                process.standardOutput = output
                process.standardError = FileHandle.nullDevice
                do {
                    try process.run()
                } catch {
                    continuation.resume(returning: nil)
                    return
                }
                // Read before waiting, so a large status can't fill the pipe and stall the command.
                let data = output.fileHandleForReading.readDataToEndOfFile()
                process.waitUntilExit()
                continuation.resume(returning: process.terminationStatus == 0 ? data : nil)
            }
        }
        return status.flatMap(PhoneHubAddress.parseTailscaleStatusToAddress)
    }
}
