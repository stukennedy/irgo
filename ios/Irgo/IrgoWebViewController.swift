import UIKit
import WebKit

/// Main WebView controller for Irgo apps
open class IrgoWebViewController: UIViewController {

    /// The WebView instance
    public private(set) var webView: WKWebView!

    /// The scheme handler for intercepting requests
    private let schemeHandler = IrgoSchemeHandler()

    /// JavaScript bridge code
    private var bridgeScript: String {
        return """
        (function() {
            // Store original fetch
            const originalFetch = window.fetch;

            // Override fetch to use irgo:// scheme
            window.fetch = function(input, init) {
                let url = input;
                if (typeof input === 'object' && input.url) {
                    url = input.url;
                }

                // Convert relative URLs to irgo:// scheme
                if (typeof url === 'string') {
                    if (url.startsWith('/')) {
                        url = 'irgo://app' + url;
                    } else if (!url.includes('://')) {
                        url = 'irgo://app/' + url;
                    }
                }

                // For external URLs, use original fetch
                if (!url.startsWith('irgo://')) {
                    return originalFetch(input, init);
                }

                return originalFetch(url, init);
            };

            // Datastar uses fetch for SSE requests, which is already intercepted above

            console.log('Irgo bridge initialized');
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
        config.setURLSchemeHandler(schemeHandler, forURLScheme: IrgoSchemeHandler.scheme)

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
        IrgoBridge.shared.configure(webView: webView)
    }

    /// Load the initial HTML page
    private func loadInitialPage() {
        let html = IrgoBridge.shared.renderInitialPage()

        // Load with base URL using our custom scheme
        webView.loadHTMLString(html, baseURL: URL(string: "irgo://app/"))
    }

    /// Navigate to a path within the app
    public func navigate(to path: String) {
        var url = path
        if !url.hasPrefix("irgo://") {
            if url.hasPrefix("/") {
                url = "irgo://app" + url
            } else {
                url = "irgo://app/" + url
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
extension IrgoWebViewController: WKNavigationDelegate {

    public func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        // Page loaded successfully
    }

    public func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        print("Irgo navigation failed: \(error.localizedDescription)")
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

        // Allow irgo:// scheme
        if url.scheme == IrgoSchemeHandler.scheme {
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
