import Foundation
import WebKit

/// Custom URL scheme handler that intercepts requests and routes them to Go
public class GoHTMXSchemeHandler: NSObject, WKURLSchemeHandler {

    /// The URL scheme to intercept (e.g., "gohtmx")
    public static let scheme = "gohtmx"

    /// Start handling a request
    public func webView(_ webView: WKWebView, start urlSchemeTask: WKURLSchemeTask) {
        guard let url = urlSchemeTask.request.url else {
            urlSchemeTask.didFailWithError(GoHTMXError.invalidURL)
            return
        }

        // Convert gohtmx:// URL to path
        // gohtmx://app/path?query -> /path?query
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
            let response = GoHTMXBridge.shared.handleRequest(
                method: method,
                url: path,
                headers: headers,
                body: body
            )

            // Create URL response
            let mimeType = response.headers["Content-Type"] ?? "text/html"
            let urlResponse = HTTPURLResponse(
                url: url,
                statusCode: response.status,
                httpVersion: "HTTP/1.1",
                headerFields: response.headers
            )

            DispatchQueue.main.async {
                if let urlResponse = urlResponse {
                    urlSchemeTask.didReceive(urlResponse)
                    urlSchemeTask.didReceive(response.body)
                    urlSchemeTask.didFinish()
                } else {
                    urlSchemeTask.didFailWithError(GoHTMXError.responseError)
                }
            }
        }
    }

    /// Stop handling a request (cancellation)
    public func webView(_ webView: WKWebView, stop urlSchemeTask: WKURLSchemeTask) {
        // Request was cancelled, nothing to clean up
    }
}

/// GoHTMX specific errors
public enum GoHTMXError: Error {
    case invalidURL
    case responseError
    case bridgeNotInitialized
    case unsupported(String)
}
