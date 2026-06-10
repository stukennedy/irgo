package com.irgo

import android.annotation.SuppressLint
import android.os.Bundle
import android.webkit.WebSettings
import android.webkit.WebView
import androidx.appcompat.app.AppCompatActivity

/**
 * Base activity for Irgo apps.
 * Subclass this in your app to customize behavior.
 */
open class IrgoActivity : AppCompatActivity() {

    lateinit var webView: WebView
        private set

    private val bridgeScript = """
        (function() {
            // Store original fetch
            const originalFetch = window.fetch;

            // Rewrite all fetch() URLs to http://localhost so Android WebView's
            // Fetch API doesn't reject them as "unsupported scheme". The actual
            // requests are intercepted in IrgoWebViewClient.shouldInterceptRequest
            // and routed to Go without ever touching the network.
            window.fetch = function(input, init) {
                let url = input;
                if (typeof input === 'object' && input.url) {
                    url = input.url;
                }

                // Convert relative URLs to http://localhost/
                if (typeof url === 'string') {
                    if (url.startsWith('/')) {
                        url = 'http://localhost' + url;
                    } else if (!url.includes('://')) {
                        url = 'http://localhost/' + url;
                    }
                }

                // For external (https://...) URLs, pass through unchanged.
                if (typeof url === 'string' && !url.startsWith('http://localhost')) {
                    return originalFetch(input, init);
                }

                return originalFetch(url, init);
            };

            console.log('Irgo bridge initialized');
        })();
    """.trimIndent()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Initialize Go bridge
        IrgoBridge.initialize()

        // Create and configure WebView
        webView = createWebView()
        setContentView(webView)

        // Configure bridge
        IrgoBridge.configure(webView)

        // Load initial page
        loadInitialPage()
    }

    @SuppressLint("SetJavaScriptEnabled")
    protected open fun createWebView(): WebView {
        return WebView(this).apply {
            // Set custom WebViewClient
            webViewClient = IrgoWebViewClient()

            // Configure settings
            settings.apply {
                javaScriptEnabled = true
                domStorageEnabled = true
                databaseEnabled = true
                allowFileAccess = false
                allowContentAccess = false

                // Mobile-friendly settings
                useWideViewPort = true
                loadWithOverviewMode = true

                // Disable zoom for app-like experience
                setSupportZoom(false)
                builtInZoomControls = false
                displayZoomControls = false

                // Cache settings
                cacheMode = WebSettings.LOAD_DEFAULT
            }
        }
    }

    protected open fun loadInitialPage() {
        val html = IrgoBridge.renderInitialPage()

        // Inject bridge script before loading
        val fullHtml = html.replace(
            "<head>",
            "<head><script>$bridgeScript</script>"
        )

        webView.loadDataWithBaseURL(
            "http://localhost/",
            fullHtml,
            "text/html",
            "UTF-8",
            null
        )
    }

    /**
     * Navigate to a path within the app
     */
    fun navigate(path: String) {
        var url = path
        if (!url.startsWith("http://localhost")) {
            url = if (url.startsWith("/")) {
                "http://localhost$url"
            } else {
                "http://localhost/$url"
            }
        }
        webView.loadUrl(url)
    }

    /**
     * Evaluate JavaScript in the WebView
     */
    fun evaluateJavaScript(script: String, callback: ((String?) -> Unit)? = null) {
        webView.evaluateJavascript(script) { result ->
            callback?.invoke(result)
        }
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            super.onBackPressed()
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        IrgoBridge.shutdown()
    }
}
