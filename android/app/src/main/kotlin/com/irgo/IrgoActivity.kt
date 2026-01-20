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
                document.body.addEventListener('htmx:configRequest', function(evt) {
                    let path = evt.detail.path;
                    if (path.startsWith('/')) {
                        evt.detail.path = 'irgo://app' + path;
                    } else if (!path.includes('://')) {
                        evt.detail.path = 'irgo://app/' + path;
                    }
                });
            }

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

            // Add JavaScript interface for WebSocket bridge
            addJavascriptInterface(IrgoWebSocketInterface(this@IrgoActivity), "IrgoNative")
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
            "irgo://app/",
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
        if (!url.startsWith("irgo://")) {
            url = if (url.startsWith("/")) {
                "irgo://app$url"
            } else {
                "irgo://app/$url"
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
