import UIKit
import WebKit

/// Main WebView controller for GoHTMX apps
open class GoHTMXWebViewController: UIViewController {

    /// The WebView instance
    public private(set) var webView: WKWebView!

    /// The scheme handler for intercepting requests
    private let schemeHandler = GoHTMXSchemeHandler()

    /// JavaScript bridge code
    private var bridgeScript: String {
        return """
        (function() {
            // Store original fetch
            const originalFetch = window.fetch;

            // Override fetch to use gohtmx:// scheme
            window.fetch = function(input, init) {
                let url = input;
                if (typeof input === 'object' && input.url) {
                    url = input.url;
                }

                // Convert relative URLs to gohtmx:// scheme
                if (typeof url === 'string') {
                    if (url.startsWith('/')) {
                        url = 'gohtmx://app' + url;
                    } else if (!url.includes('://')) {
                        url = 'gohtmx://app/' + url;
                    }
                }

                // For external URLs, use original fetch
                if (!url.startsWith('gohtmx://')) {
                    return originalFetch(input, init);
                }

                return originalFetch(url, init);
            };

            // Configure HTMX to use gohtmx:// scheme
            if (typeof htmx !== 'undefined') {
                // HTMX 4 event for modifying requests
                document.body.addEventListener('htmx:configRequest', function(evt) {
                    let path = evt.detail.path;
                    if (path.startsWith('/')) {
                        evt.detail.path = 'gohtmx://app' + path;
                    } else if (!path.includes('://')) {
                        evt.detail.path = 'gohtmx://app/' + path;
                    }
                });
            }

            console.log('GoHTMX bridge initialized');
        })();
        """
    }

    open override func viewDidLoad() {
        super.viewDidLoad()
        setupWebView()
        loadInitialPage()
    }

    /// Set up the WebView with custom configuration
    private func setupWebView() {
        // Create configuration
        let config = WKWebViewConfiguration()

        // Register custom scheme handler
        config.setURLSchemeHandler(schemeHandler, forURLScheme: GoHTMXSchemeHandler.scheme)

        // Add bridge script
        let userScript = WKUserScript(
            source: bridgeScript,
            injectionTime: .atDocumentStart,
            forMainFrameOnly: false
        )
        config.userContentController.addUserScript(userScript)

        // Configure preferences
        config.preferences.javaScriptEnabled = true

        // Allow inline media playback
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []

        // Create WebView
        webView = WKWebView(frame: view.bounds, configuration: config)
        webView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        webView.navigationDelegate = self
        webView.scrollView.contentInsetAdjustmentBehavior = .never

        // Configure for mobile
        webView.scrollView.bounces = true
        webView.allowsBackForwardNavigationGestures = true

        // Add to view
        view.addSubview(webView)

        // Configure bridge
        GoHTMXBridge.shared.configure(webView: webView)
    }

    /// Load the initial HTML page
    private func loadInitialPage() {
        let html = GoHTMXBridge.shared.renderInitialPage()

        // Load with base URL using our custom scheme
        webView.loadHTMLString(html, baseURL: URL(string: "gohtmx://app/"))
    }

    /// Navigate to a path within the app
    public func navigate(to path: String) {
        var url = path
        if !url.hasPrefix("gohtmx://") {
            if url.hasPrefix("/") {
                url = "gohtmx://app" + url
            } else {
                url = "gohtmx://app/" + url
            }
        }

        if let navURL = URL(string: url) {
            webView.load(URLRequest(url: navURL))
        }
    }

    /// Inject JavaScript into the WebView
    public func evaluateJavaScript(_ script: String, completion: ((Any?, Error?) -> Void)? = nil) {
        webView.evaluateJavaScript(script, completionHandler: completion)
    }
}

// MARK: - WKNavigationDelegate
extension GoHTMXWebViewController: WKNavigationDelegate {

    public func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        // Page loaded successfully
    }

    public func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        print("GoHTMX navigation failed: \(error.localizedDescription)")
    }

    public func webView(
        _ webView: WKWebView,
        decidePolicyFor navigationAction: WKNavigationAction,
        decisionHandler: @escaping (WKNavigationActionPolicy) -> Void
    ) {
        guard let url = navigationAction.request.url else {
            decisionHandler(.cancel)
            return
        }

        // Allow gohtmx:// scheme
        if url.scheme == GoHTMXSchemeHandler.scheme {
            decisionHandler(.allow)
            return
        }

        // Allow data: URLs (for initial HTML load)
        if url.scheme == "data" || url.scheme == "about" {
            decisionHandler(.allow)
            return
        }

        // For external URLs, open in Safari
        if url.scheme == "http" || url.scheme == "https" {
            UIApplication.shared.open(url)
            decisionHandler(.cancel)
            return
        }

        decisionHandler(.allow)
    }
}
