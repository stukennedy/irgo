import Foundation
import WebKit

/// Custom URL scheme handler that intercepts requests and routes them to Go
public class IrgoSchemeHandler: NSObject, WKURLSchemeHandler {

    /// The URL scheme to intercept (e.g., "irgo")
    public static let scheme = "irgo"

    /// Tasks that are still alive, keyed by identity. WebKit calls stop(_:)
    /// when a request is cancelled (e.g. Datastar aborts an in-flight fetch
    /// because the user triggered the same action again). Calling
    /// didReceive/didFinish on a stopped task raises an Objective-C
    /// exception and crashes the app, so every completion is guarded.
    /// Accessed on the main thread only (WebKit delivers start/stop there).
    private var activeTasks = Set<ObjectIdentifier>()

    /// Start handling a request
    public func webView(_ webView: WKWebView, start urlSchemeTask: WKURLSchemeTask) {
        guard let url = urlSchemeTask.request.url else {
            urlSchemeTask.didFailWithError(IrgoError.invalidURL)
            return
        }

        activeTasks.insert(ObjectIdentifier(urlSchemeTask))

        // Convert irgo:// URL to path
        // irgo://app/path?query -> /path?query
        var path = url.path
        if path.isEmpty {
            path = "/"
        }
        if let query = url.query, !query.isEmpty {
            path += "?" + query
        }

        // Get HTTP method
        let method = urlSchemeTask.request.httpMethod ?? "GET"

        // Get headers
        var headers: [String: String] = [:]
        urlSchemeTask.request.allHTTPHeaderFields?.forEach { key, value in
            headers[key] = value
        }

        // Get body
        let body = urlSchemeTask.request.httpBody

        // Handle request in background
        DispatchQueue.global(qos: .userInitiated).async {
            let response = IrgoBridge.shared.handleRequest(
                method: method,
                url: path,
                headers: headers,
                body: body
            )

            // Create URL response
            let urlResponse = HTTPURLResponse(
                url: url,
                statusCode: response.status,
                httpVersion: "HTTP/1.1",
                headerFields: response.headers
            )

            DispatchQueue.main.async { [weak self] in
                guard let self = self else { return }

                // The task may have been cancelled while Go was handling the
                // request - touching it now would crash with
                // NSInternalInconsistencyException.
                let id = ObjectIdentifier(urlSchemeTask)
                guard self.activeTasks.contains(id) else { return }
                self.activeTasks.remove(id)

                if let urlResponse = urlResponse {
                    urlSchemeTask.didReceive(urlResponse)
                    urlSchemeTask.didReceive(response.body)
                    urlSchemeTask.didFinish()
                } else {
                    urlSchemeTask.didFailWithError(IrgoError.responseError)
                }
            }
        }
    }

    /// Stop handling a request (cancellation)
    public func webView(_ webView: WKWebView, stop urlSchemeTask: WKURLSchemeTask) {
        // Mark the task dead so the in-flight completion above is dropped.
        activeTasks.remove(ObjectIdentifier(urlSchemeTask))
    }
}

/// Irgo specific errors
public enum IrgoError: Error {
    case invalidURL
    case responseError
    case bridgeNotInitialized
}
