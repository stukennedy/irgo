import UIKit
import WebKit

/// Main WebView controller for Irgo apps
open class IrgoWebViewController: UIViewController {

    /// The WebView instance
    public private(set) var webView: WKWebView!

    /// The scheme handler for intercepting requests
    private let schemeHandler = IrgoSchemeHandler()

    /// Whether we're running in dev mode (connecting to local server)
    private var isDevMode: Bool {
        // Check for dev server URL in Info.plist or environment
        if let devURL = Bundle.main.object(forInfoDictionaryKey: "IRGO_DEV_SERVER") as? String,
           !devURL.isEmpty {
            return true
        }
        // Also check environment variable (for debugging)
        if let envURL = ProcessInfo.processInfo.environment["IRGO_DEV_SERVER"],
           !envURL.isEmpty {
            return true
        }
        return false
    }

    /// The dev server URL if in dev mode
    private var devServerURL: String? {
        if let devURL = Bundle.main.object(forInfoDictionaryKey: "IRGO_DEV_SERVER") as? String,
           !devURL.isEmpty {
            return devURL
        }
        if let envURL = ProcessInfo.processInfo.environment["IRGO_DEV_SERVER"],
           !envURL.isEmpty {
            return envURL
        }
        return nil
    }

    /// JavaScript bridge code for production mode (irgo:// scheme)
    private var productionBridgeScript: String {
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

            // Configure HTMX to use irgo:// scheme
            if (typeof htmx !== 'undefined') {
                // HTMX 4 event for modifying requests
                document.body.addEventListener('htmx:configRequest', function(evt) {
                    let path = evt.detail.path;
                    if (path.startsWith('/')) {
                        evt.detail.path = 'irgo://app' + path;
                    } else if (!path.includes('://')) {
                        evt.detail.path = 'irgo://app/' + path;
                    }
                });
            }

            console.log('Irgo bridge initialized (production mode)');
        })();
        """
    }

    /// JavaScript for dev mode - includes live reload functionality
    private var devBridgeScript: String {
        return """
        (function() {
            console.log('Irgo running in dev mode - connecting to local server');

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
                        console.log('Irgo: Connected to dev server (build: ' + buildTime + ')');
                    } else if (buildTime !== lastBuildTime) {
                        // Build time changed - server was rebuilt!
                        console.log('Irgo: Server rebuilt, reloading...');
                        window.location.reload();
                        return;
                    }
                } catch (error) {
                    // Server might be restarting, keep polling
                    console.log('Irgo: Waiting for server...');
                }

                setTimeout(checkForReload, checkInterval);
            }

            // Start checking after a short delay
            setTimeout(checkForReload, 500);

            console.log('Irgo: Live reload enabled');
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
            print("Irgo: Running in DEV MODE - connecting to \(devServerURL ?? "unknown")")

            let userScript = WKUserScript(
                source: devBridgeScript,
                injectionTime: .atDocumentStart,
                forMainFrameOnly: false
            )
            config.userContentController.addUserScript(userScript)
        } else {
            // Production mode: use custom scheme handler
            config.setURLSchemeHandler(schemeHandler, forURLScheme: IrgoSchemeHandler.scheme)

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
            IrgoBridge.shared.configure(webView: webView)
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
            let html = IrgoBridge.shared.renderInitialPage()
            webView.loadHTMLString(html, baseURL: URL(string: "irgo://app/"))
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
            // Production mode: use irgo:// scheme
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
