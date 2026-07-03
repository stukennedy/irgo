package com.irgo

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import java.io.ByteArrayInputStream

/**
 * Custom WebViewClient that intercepts irgo:// resource loads (navigation and
 * static assets such as CSS/JS) and routes them to Go.
 *
 * NOTE: Datastar's fetch() traffic does NOT flow through here. On Android,
 * shouldInterceptRequest cannot access request bodies (POST/PUT/etc.) and
 * cannot stream responses, so irgo-bridge.js patches window.fetch to route
 * those requests through the IrgoJSInterface (window.Irgo) instead. This client
 * only handles plain GET resource loads triggered by the WebView itself.
 *
 * Navigation policy (shouldOverrideUrlLoading):
 *  - irgo:// URLs stay in the WebView (served by shouldInterceptRequest)
 *  - in dev mode, same-origin loads on the dev server stay in the WebView
 *  - everything else (external http(s), mailto:, tel:, ...) opens externally
 *
 * @param devServerUrl the dev server URL when running in dev mode
 *   (see [IrgoActivity.EXTRA_DEV_SERVER]), or null in production.
 */
open class IrgoWebViewClient(devServerUrl: String? = null) : WebViewClient() {

    companion object {
        const val SCHEME = "irgo"
        const val HOST = "app"
    }

    private val devServerUri: Uri? = devServerUrl?.let { url ->
        try {
            Uri.parse(url)
        } catch (e: Exception) {
            null
        }
    }

    override fun shouldInterceptRequest(
        view: WebView?,
        request: WebResourceRequest?
    ): WebResourceResponse? {
        val url = request?.url ?: return null

        // Only intercept irgo:// scheme
        if (url.scheme != SCHEME) {
            return super.shouldInterceptRequest(view, request)
        }

        // Convert irgo://app/path?query -> /path?query
        var path = url.path ?: "/"
        if (path.isEmpty()) {
            path = "/"
        }
        url.query?.let { query ->
            if (query.isNotEmpty()) {
                path += "?$query"
            }
        }

        // Get HTTP method
        val method = request.method ?: "GET"

        // Get headers
        val headers = request.requestHeaders ?: emptyMap()

        // Handle request (this runs on WebView thread, which is fine for our use case)
        var response = IrgoBridge.handleRequest(
            method = method,
            url = path,
            headers = headers,
            body = null // GET resource loads only; body-bearing requests go through IrgoJSInterface
        )

        // Follow redirects here: WebResourceResponse throws
        // IllegalArgumentException for 3xx status codes, so a Go handler
        // returning ctx.Redirect() on a plain navigation would crash the app.
        var hops = 0
        while (response.status in 300..399 && hops < 5) {
            val location = response.headers["Location"] ?: break
            val target = if (location.startsWith("/")) location else "/$location"
            response = IrgoBridge.handleRequest("GET", target, headers, null)
            hops++
        }

        // WebResourceResponse rejects status < 100, 3xx, and empty reason
        // phrases; degrade to a plain 500 rather than throwing on the
        // Chromium thread.
        val status = if (response.status in 300..399 || response.status < 100) 500 else response.status

        // Determine MIME type
        val mimeType = response.headers["Content-Type"] ?: "text/html"

        // Create response
        return WebResourceResponse(
            mimeType.split(";").first().trim(),
            "UTF-8",
            status,
            if (status < 400) "OK" else "Error",
            response.headers,
            ByteArrayInputStream(response.body)
        )
    }

    override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
        val url = request?.url ?: return false

        // irgo:// navigation stays in the WebView (handled by shouldInterceptRequest)
        if (url.scheme == SCHEME) {
            return false
        }

        // Dev mode: same-origin loads on the dev server stay in the WebView
        if (isSameOrigin(url, devServerUri)) {
            return false
        }

        // External http(s) links (and other schemes like mailto:) open in the
        // system browser / matching app rather than hijacking the app WebView.
        return openExternally(view, url)
    }

    override fun onPageFinished(view: WebView?, url: String?) {
        super.onPageFinished(view, url)
        // Page loaded successfully
    }

    /**
     * Open a URL outside the app via ACTION_VIEW. Returns true (the WebView
     * never navigates) even if no activity can handle the URL.
     */
    protected open fun openExternally(view: WebView?, url: Uri): Boolean {
        val context = view?.context ?: return true
        try {
            val intent = Intent(Intent.ACTION_VIEW, url).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            context.startActivity(intent)
        } catch (e: ActivityNotFoundException) {
            // Nothing installed can handle this URL - swallow rather than
            // letting the WebView navigate away from the app.
        }
        return true
    }

    private fun isSameOrigin(a: Uri, b: Uri?): Boolean {
        if (b == null) return false
        val schemeA = a.scheme?.lowercase() ?: return false
        val schemeB = b.scheme?.lowercase() ?: return false
        if (schemeA != schemeB) return false
        if (!a.host.equals(b.host, ignoreCase = true)) return false
        return portOf(a, schemeA) == portOf(b, schemeB)
    }

    private fun portOf(uri: Uri, scheme: String): Int {
        if (uri.port != -1) return uri.port
        return when (scheme) {
            "http" -> 80
            "https" -> 443
            else -> -1
        }
    }
}
