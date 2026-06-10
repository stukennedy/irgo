import Foundation
import WebKit
import Irgo

/// Bridge for virtual WebSocket connections
/// Note: WebSocket support requires additional setup in the mobile package.
/// For now, this provides the interface but WebSocket calls will be no-ops.
public class IrgoWebSocketBridge: NSObject {
    public static let shared = IrgoWebSocketBridge()

    private weak var webView: WKWebView?
    private var activeSessions: Set<String> = []

    private override init() {
        super.init()
        // WebSocket callback registration will be added when the mobile package
        // exports WebSocket functions
    }

    /// Configure with a WebView for message delivery
    public func configure(webView: WKWebView) {
        self.webView = webView
    }

    /// Connect to a virtual WebSocket
    /// - Parameter url: The WebSocket URL (e.g., "ws://app/chat")
    /// - Returns: Session ID
    public func connect(url: String) throws -> String {
        // TODO: Implement when mobile package exports WebSocketConnect
        throw IrgoError.unsupported("WebSocket support not yet implemented")
    }

    /// Send a message through a virtual WebSocket
    public func send(sessionID: String, data: String) throws -> String? {
        // TODO: Implement when mobile package exports WebSocketSend
        throw IrgoError.unsupported("WebSocket support not yet implemented")
    }

    /// Close a virtual WebSocket connection
    public func close(sessionID: String) {
        activeSessions.remove(sessionID)
    }

    /// Close all active sessions
    public func closeAll() {
        activeSessions.removeAll()
    }

    // MARK: - Internal message delivery (called from native callback)

    func deliverMessage(sessionID: String, data: String) {
        // Escape for JavaScript
        let escaped = data
            .replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "'", with: "\\'")
            .replacingOccurrences(of: "\n", with: "\\n")
            .replacingOccurrences(of: "\r", with: "\\r")

        let js = "window._irgo_ws_message('\(sessionID)', '\(escaped)')"

        DispatchQueue.main.async { [weak self] in
            self?.webView?.evaluateJavaScript(js, completionHandler: nil)
        }
    }

    func deliverClose(sessionID: String, code: Int, reason: String) {
        activeSessions.remove(sessionID)

        let reasonEscaped = reason.replacingOccurrences(of: "'", with: "\\'")
        let js = "window._irgo_ws_close('\(sessionID)', \(code), '\(reasonEscaped)')"

        DispatchQueue.main.async { [weak self] in
            self?.webView?.evaluateJavaScript(js, completionHandler: nil)
        }
    }
}
