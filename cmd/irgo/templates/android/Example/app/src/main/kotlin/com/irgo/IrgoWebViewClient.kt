package com.irgo

import android.net.Uri
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import java.io.ByteArrayInputStream

/**
 * Custom WebViewClient that intercepts requests and routes them to Go
 */
open class IrgoWebViewClient : WebViewClient() {

    companion object {
        // The bridge intercepts all requests under http://localhost/ and routes
        // them to Go. No real network socket is opened — WebViewClient.
        // shouldInterceptRequest serves the response directly from Go memory.
        //
        // We use http://localhost (instead of a custom scheme like irgo://)
        // because Android WebView's Fetch API rejects custom URL schemes at the
        // JS layer before shouldInterceptRequest can run. http://localhost is on
        // the Fetch allowlist, so all Datastar SSE requests work transparently.
        const val HOST = "localhost"
    }

    override fun shouldInterceptRequest(
        view: WebView?,
        request: WebResourceRequest?
    ): WebResourceResponse? {
        val url = request?.url ?: return null

        // Only intercept http://localhost/* — anything else (https://api.example.com,
        // file://, etc.) is allowed to fall through to the network layer.
        if (url.scheme != "http" || url.host != HOST) {
            return super.shouldInterceptRequest(view, request)
        }

        // Convert http://localhost/path?query -> /path?query
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
        val response = IrgoBridge.handleRequest(
            method = method,
            url = path,
            headers = headers,
            body = null // WebResourceRequest doesn't provide body access
        )

        // Determine MIME type
        val mimeType = response.headers["Content-Type"] ?: "text/html"

        // Create response
        return WebResourceResponse(
            mimeType.split(";").first().trim(),
            "UTF-8",
            response.status,
            if (response.status < 400) "OK" else "Error",
            response.headers,
            ByteArrayInputStream(response.body)
        )
    }

    override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
        val url = request?.url ?: return false

        // Let WebView handle http://localhost (will be intercepted by shouldInterceptRequest).
        if (url.scheme == "http" && url.host == HOST) {
            return false
        }

        // External URLs fall through to the default handling.
        return false
    }

    override fun onPageFinished(view: WebView?, url: String?) {
        super.onPageFinished(view, url)
        // Page loaded successfully
    }
}
