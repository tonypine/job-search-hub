import AppKit
import WebKit

/// Prints HTML to a paginated A4 PDF through WebKit, offscreen. The page's
/// own @page rule sets the margins.
@MainActor
final class CVPrinter: NSObject, WKNavigationDelegate {
    enum PrintError: Error {
        case loadFailed(String)
        case printFailed
    }

    private let window = NSWindow(contentRect: NSRect(x: -10_000, y: -10_000, width: 595, height: 842), styleMask: [.borderless], backing: .buffered, defer: false)
    private let webView = WKWebView(frame: NSRect(x: 0, y: 0, width: 595, height: 842))
    private var output: URL?
    private var continuation: CheckedContinuation<Void, Error>?

    /// Writes the PDF of html to output.
    func printPDF(html: String, to output: URL) async throws {
        self.output = output
        window.contentView = webView
        webView.navigationDelegate = self
        try await withCheckedThrowingContinuation { continuation in
            self.continuation = continuation
            webView.loadHTMLString(html, baseURL: nil)
        }
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        guard let output else { return }
        let info = NSPrintInfo()
        info.paperSize = NSSize(width: 595.28, height: 841.89)
        info.topMargin = 0
        info.bottomMargin = 0
        info.leftMargin = 0
        info.rightMargin = 0
        info.horizontalPagination = .automatic
        info.verticalPagination = .automatic
        info.jobDisposition = .save
        info.dictionary()[NSPrintInfo.AttributeKey.jobSavingURL] = output
        let operation = webView.printOperation(with: info)
        operation.showsPrintPanel = false
        operation.showsProgressPanel = false
        operation.view?.frame = webView.bounds
        operation.runModal(for: window, delegate: self, didRun: #selector(didPrint(_:success:contextInfo:)), contextInfo: nil)
    }

    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        finish(.failure(PrintError.loadFailed(error.localizedDescription)))
    }

    /// AppKit calls this on the print operation's own thread, not the main one.
    @objc nonisolated private func didPrint(_ operation: NSPrintOperation, success: Bool, contextInfo: UnsafeMutableRawPointer?) {
        Task { @MainActor in
            self.finish(success ? .success(()) : .failure(PrintError.printFailed))
        }
    }

    private func finish(_ result: Result<Void, Error>) {
        continuation?.resume(with: result)
        continuation = nil
    }
}
