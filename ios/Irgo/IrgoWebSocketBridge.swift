import Foundation
import WebKit
import Irgo

/// Bridge for virtual WebSocket connections
public class IrgoWebSocketBridge: NSObject {
    public static let shared = IrgoWebSocketBridge()

    private weak var webView: WKWebView?
    private var activeSessions: Set<String> = []

    private override init() {
        super.init()
        // Register as WebSocket callback handler
        MobileSetWebSocketCallback(self)
    }

    /// Configure with a WebView for message delivery
    public func configure(webView: WKWebView) {
        self.webView = webView
    }

    /// Connect to a virtual WebSocket
    /// - Parameter url: The WebSocket URL (e.g., "ws://app/chat")
    /// - Returns: Session ID
    public func connect(url: String) throws -> String {
        var error: NSError?
        let sessionID = MobileWebSocketConnect(url, &error)

        if let error = error {
            throw error
        }

        guard let sessionID = sessionID, !sessionID.isEmpty else {
            throw IrgoError.bridgeNotInitialized
        }

        activeSessions.insert(sessionID)
        return sessionID
    }

    /// Send a message through a virtual WebSocket
    /// - Parameters:
    ///   - sessionID: The session ID from connect()
    ///   - data: JSON-encoded message data
    public func send(sessionID: String, data: String) throws -> String? {
        var error: NSError?
        let response = MobileWebSocketSend(sessionID, data, &error)

        if let error = error {
            throw error
        }

        return response
    }

    /// Close a virtual WebSocket connection
    public func close(sessionID: String) {
        do {
            try MobileWebSocketClose(sessionID)
        } catch {
            print("Error closing WebSocket: \(error)")
        }
        activeSessions.remove(sessionID)
    }

    /// Close all active sessions
    public func closeAll() {
        for sessionID in activeSessions {
            close(sessionID: sessionID)
        }
    }
}

// MARK: - IrgoWebSocketCallbackProtocol
extension IrgoWebSocketBridge: IrgoWebSocketCallbackProtocol {

    /// Called when a message should be sent to the WebView
    public func onMessage(_ sessionID: String?, data: String?) {
        guard let sessionID = sessionID, let data = data else { return }

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

    /// Called when a WebSocket connection is closed
    public func onClose(_ sessionID: String?, code: Int, reason: String?) {
        guard let sessionID = sessionID else { return }

        activeSessions.remove(sessionID)

        let reasonEscaped = (reason ?? "")
            .replacingOccurrences(of: "'", with: "\\'")

        let js = "window._irgo_ws_close('\(sessionID)', \(code), '\(reasonEscaped)')"

        DispatchQueue.main.async { [weak self] in
            self?.webView?.evaluateJavaScript(js, completionHandler: nil)
        }
    }

    /// Called when an error occurs
    public func onError(_ sessionID: String?, error: String?) {
        guard let sessionID = sessionID, let error = error else { return }

        let errorEscaped = error.replacingOccurrences(of: "'", with: "\\'")
        let js = "window._irgo_ws_error('\(sessionID)', '\(errorEscaped)')"

        DispatchQueue.main.async { [weak self] in
            self?.webView?.evaluateJavaScript(js, completionHandler: nil)
        }
    }
}
