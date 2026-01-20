import UIKit
import WebKit

/// Main WebView controller for GoHTMX apps
open class GoHTMXWebViewController: UIViewController {

    /// The WebView instance
    public private(set) var webView: WKWebView!

    /// The scheme handler for intercepting requests
    private let schemeHandler = GoHTMXSchemeHandler()

    /// Whether we're running in dev mode (connecting to local server)
    private var isDevMode: Bool {
        // Check for dev server URL in Info.plist or environment
        if let devURL = Bundle.main.object(forInfoDictionaryKey: "GOHTMX_DEV_SERVER") as? String,
           !devURL.isEmpty {
            return true
        }
        // Also check environment variable (for debugging)
        if let envURL = ProcessInfo.processInfo.environment["GOHTMX_DEV_SERVER"],
           !envURL.isEmpty {
            return true
        }
        return false
    }

    /// The dev server URL if in dev mode
    private var devServerURL: String? {
        if let devURL = Bundle.main.object(forInfoDictionaryKey: "GOHTMX_DEV_SERVER") as? String,
           !devURL.isEmpty {
            return devURL
        }
        if let envURL = ProcessInfo.processInfo.environment["GOHTMX_DEV_SERVER"],
           !envURL.isEmpty {
            return envURL
        }
        return nil
    }

    /// JavaScript bridge code for production mode (gohtmx:// scheme)
    private var productionBridgeScript: String {
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

            console.log('GoHTMX bridge initialized (production mode)');
        })();
        """
    }

    /// JavaScript for dev mode - includes live reload functionality
    private var devBridgeScript: String {
        return """
        (function() {
            console.log('GoHTMX running in dev mode - connecting to local server');

            // Live reload: poll /dev/reload for build timestamp changes
            let lastBuildTime = null;
            const checkInterval = 1000; // Check every second

            async function checkForReload() {
                try {
                    const response = await fetch('/dev/reload', {
                        cache: 'no-store'
                    });
                    const buildTime = await response.text();

                    if (lastBuildTime === null) {
                        // First check - just record the build time
                        lastBuildTime = buildTime;
                        console.log('GoHTMX: Connected to dev server (build: ' + buildTime + ')');
                    } else if (buildTime !== lastBuildTime) {
                        // Build time changed - server was rebuilt!
                        console.log('GoHTMX: Server rebuilt, reloading...');
                        window.location.reload();
                        return;
                    }
                } catch (error) {
                    // Server might be restarting, keep polling
                    console.log('GoHTMX: Waiting for server...');
                }

                setTimeout(checkForReload, checkInterval);
            }

            // Start checking after a short delay
            setTimeout(checkForReload, 500);

            console.log('GoHTMX: Live reload enabled');
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

        if isDevMode {
            // Dev mode: no custom scheme handler needed, just standard HTTP
            print("GoHTMX: Running in DEV MODE - connecting to \(devServerURL ?? "unknown")")

            let userScript = WKUserScript(
                source: devBridgeScript,
                injectionTime: .atDocumentStart,
                forMainFrameOnly: false
            )
            config.userContentController.addUserScript(userScript)
        } else {
            // Production mode: use custom scheme handler
            config.setURLSchemeHandler(schemeHandler, forURLScheme: GoHTMXSchemeHandler.scheme)

            let userScript = WKUserScript(
                source: productionBridgeScript,
                injectionTime: .atDocumentStart,
                forMainFrameOnly: false
            )
            config.userContentController.addUserScript(userScript)

            // Configure bridge
            // (done after webView is created)
        }

        // Configure preferences
        let prefs = WKWebpagePreferences()
        prefs.allowsContentJavaScript = true
        config.defaultWebpagePreferences = prefs

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

        // Configure bridge for production mode
        if !isDevMode {
            GoHTMXBridge.shared.configure(webView: webView)
        }
    }

    /// Load the initial HTML page
    private func loadInitialPage() {
        if isDevMode, let serverURL = devServerURL {
            // Dev mode: load from local server
            if let url = URL(string: serverURL) {
                webView.load(URLRequest(url: url))
            }
        } else {
            // Production mode: render from Go bridge
            let html = GoHTMXBridge.shared.renderInitialPage()
            webView.loadHTMLString(html, baseURL: URL(string: "gohtmx://app/"))
        }
    }

    /// Navigate to a path within the app
    public func navigate(to path: String) {
        if isDevMode, let serverURL = devServerURL {
            // Dev mode: navigate via HTTP
            var urlString = path
            if urlString.hasPrefix("/") {
                urlString = serverURL + urlString
            } else if !urlString.contains("://") {
                urlString = serverURL + "/" + urlString
            }

            if let url = URL(string: urlString) {
                webView.load(URLRequest(url: url))
            }
        } else {
            // Production mode: use gohtmx:// scheme
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

        // In dev mode, allow HTTP to localhost
        if isDevMode {
            if url.scheme == "http" || url.scheme == "https" {
                // Allow localhost connections
                if let host = url.host, (host == "localhost" || host == "127.0.0.1" || host.hasSuffix(".local")) {
                    decisionHandler(.allow)
                    return
                }
                // External URLs: open in Safari
                UIApplication.shared.open(url)
                decisionHandler(.cancel)
                return
            }
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
