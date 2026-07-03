package com.irgo

import android.annotation.SuppressLint
import android.content.pm.ApplicationInfo
import android.os.Bundle
import android.webkit.WebSettings
import android.webkit.WebView
import androidx.appcompat.app.AppCompatActivity
import java.util.concurrent.Executors

/**
 * Base activity for Irgo apps.
 * Subclass this in your app to customize behavior.
 *
 * In production the activity boots the embedded Go app and serves it to the
 * WebView over the virtual irgo:// bridge. For development, launch with the
 * string extra [EXTRA_DEV_SERVER] (e.g. "http://10.0.2.2:8080") to load a
 * hot-reloading dev server instead:
 *
 *   adb shell am start -n com.irgo.example/.MainActivity \
 *     --es irgoDevServer "http://10.0.2.2:8080"
 */
open class IrgoActivity : AppCompatActivity() {

    companion object {
        /**
         * Intent extra naming a dev server URL to load instead of the
         * embedded Go app. Dev-server URLs are usually plain http://, which
         * requires cleartext traffic to be allowed in the manifest
         * (android:usesCleartextTraffic="true"). Production apps should not
         * ship with cleartext traffic enabled - restrict it to debug builds
         * or a debug-only network security config.
         */
        const val EXTRA_DEV_SERVER = "irgoDevServer"
    }

    lateinit var webView: WebView
        private set

    /** Dev server URL when running in dev mode, null in production. */
    var devServerUrl: String? = null
        private set

    /** True when the activity was launched with the [EXTRA_DEV_SERVER] extra. */
    val isDevMode: Boolean
        get() = devServerUrl != null

    private var jsInterface: IrgoJSInterface? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Dev mode only exists in debuggable builds: the activity is
        // exported, so honoring the extra in release builds would let any
        // co-installed app point this WebView at arbitrary content.
        val debuggable = applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0
        devServerUrl = if (debuggable) intent.getStringExtra(EXTRA_DEV_SERVER) else null

        if (!isDevMode) {
            // Initialize Go bridge
            IrgoBridge.initialize()

            // Persist bridge state (cookie jar) so sessions survive restarts
            IrgoBridge.setStateDir(filesDir.absolutePath)

            // Route Go-side native.Call(...) into the plugin registry
            IrgoNative.installGoInvoker()

            // Built-in native plugins (device, haptics, clipboard, share,
            // browser, toast, storage, notifications)
            registerPlugins()

            jsInterface = IrgoJSInterface(this)
        }

        // Create and configure WebView
        webView = createWebView()
        setContentView(webView)

        if (!isDevMode) {
            // Configure bridge
            IrgoBridge.configure(webView)

            // Server-push WebSocket messages -> window._irgo_ws_*
            jsInterface?.registerWebSocketCallback()
        }

        // Load initial page
        loadInitialPage()
    }

    /**
     * Register native capability plugins. Called during onCreate in
     * production mode. Override to add your own plugins:
     *
     *   override fun registerPlugins() {
     *       super.registerPlugins()
     *       IrgoNative.register(MyPlugin(this))
     *   }
     */
    protected open fun registerPlugins() {
        IrgoPlugins.registerBuiltins(this)
    }

    @SuppressLint("SetJavaScriptEnabled")
    protected open fun createWebView(): WebView {
        return WebView(this).apply {
            // Set custom WebViewClient (dev-server aware)
            webViewClient = IrgoWebViewClient(devServerUrl)

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

            // Expose the unified native bridge to JavaScript as `window.Irgo`.
            // irgo-bridge.js routes Datastar's fetch() (HTTP) and WebSocket
            // traffic through this interface.
            //
            // In dev mode the interface is intentionally NOT added: with
            // window.Irgo undefined, irgo-bridge.js falls back to real
            // fetch()/WebSocket against the dev server.
            jsInterface?.let { addJavascriptInterface(it, "Irgo") }
        }
    }

    protected open fun loadInitialPage() {
        val devServer = devServerUrl
        if (devServer != null) {
            // DEV MODE: load directly from the dev server (from the emulator,
            // the host machine is reachable at http://10.0.2.2:8080). See the
            // cleartext note on EXTRA_DEV_SERVER.
            webView.loadUrl(devServer)
            return
        }

        // Render off the main thread: the Go "/" handler may call
        // native.Call, whose plugins run on the main thread — rendering
        // synchronously here would deadlock until the native-call timeout.
        Executors.newSingleThreadExecutor().execute {
            val html = IrgoBridge.renderInitialPage()
            runOnUiThread {
                if (isDestroyed || isFinishing) return@runOnUiThread
                // The bridge script (irgo-bridge.js) is loaded by the page
                // itself via layout.templ and served through
                // shouldInterceptRequest. The base URL uses the irgo:// scheme
                // so relative asset URLs resolve to the native bridge.
                webView.loadDataWithBaseURL(
                    "irgo://app/",
                    html,
                    "text/html",
                    "UTF-8",
                    null
                )
            }
        }
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

    override fun onResume() {
        super.onResume()
        if (!isDevMode) {
            IrgoBridge.onForeground()
        }
    }

    override fun onPause() {
        if (!isDevMode) {
            // Persists bridge state (cookie jar) and notifies Go lifecycle
            // handlers.
            IrgoBridge.onBackground()
        }
        super.onPause()
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        IrgoPlugins.onRequestPermissionsResult(requestCode, grantResults)
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            super.onBackPressed()
        }
    }

    override fun onDestroy() {
        jsInterface?.destroy()
        jsInterface = null
        if (!isDevMode) {
            IrgoBridge.shutdown()
        }
        super.onDestroy()
    }
}
