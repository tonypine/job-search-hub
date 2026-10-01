import AppKit
import WebKit

// hub-cvprint <html-file> <pdf-file>: prints the HTML to a paginated A4 PDF through WebKit, offscreen, with no window
// shown. The hub server prints CVs with it; the page's own @page rule sets the margins.
@MainActor
final class Printer: NSObject, WKNavigationDelegate {
    let window = NSWindow(contentRect: NSRect(x: -10_000, y: -10_000, width: 595, height: 842), styleMask: [.borderless], backing: .buffered, defer: false)
    let webView = WKWebView(frame: NSRect(x: 0, y: 0, width: 595, height: 842))
    let output: URL
    init(output: URL) { self.output = output }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        let info = NSPrintInfo()
        info.paperSize = NSSize(width: 595.28, height: 841.89)
        info.topMargin = 0; info.bottomMargin = 0; info.leftMargin = 0; info.rightMargin = 0
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
        FileHandle.standardError.write("load failed: \(error.localizedDescription)\n".data(using: .utf8)!); exit(1)
    }
    /// AppKit calls this on the print operation's own thread, not the main one.
    @objc nonisolated func didPrint(_ operation: NSPrintOperation, success: Bool, contextInfo: UnsafeMutableRawPointer?) {
        DispatchQueue.main.async { exit(success ? 0 : 1) }
    }
}

let arguments = CommandLine.arguments
guard arguments.count == 3, let html = try? String(contentsOfFile: arguments[1], encoding: .utf8) else {
    FileHandle.standardError.write("usage: cvprint <html-file> <pdf-file>\n".data(using: .utf8)!); exit(2)
}
// A background-only app: no Dock icon, no menu bar.
let app = NSApplication.shared
app.setActivationPolicy(.prohibited)
let printer = Printer(output: URL(filePath: arguments[2]))
printer.window.contentView = printer.webView
printer.webView.navigationDelegate = printer
printer.webView.loadHTMLString(html, baseURL: nil)
DispatchQueue.main.asyncAfter(deadline: .now() + 60) { FileHandle.standardError.write("timed out\n".data(using: .utf8)!); exit(3) }
app.run()
