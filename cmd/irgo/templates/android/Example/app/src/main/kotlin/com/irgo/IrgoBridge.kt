package com.irgo

import android.webkit.WebView
import mobile.Mobile as Irgo
import org.json.JSONArray
import org.json.JSONObject

/**
 * Main bridge class that connects Android to the Go framework
 */
object IrgoBridge {

    private var webView: WebView? = null

    /**
     * Initialize the Go bridge. Call once at app startup.
     */
    fun initialize() {
        Irgo.initialize()
    }

    /**
     * Configure the bridge with a WebView
     */
    fun configure(webView: WebView) {
        this.webView = webView
    }

    /**
     * Check if the bridge is ready
     */
    val isReady: Boolean
        get() = Irgo.isReady()

    /**
     * Handle an HTTP request and return the response
     */
    fun handleRequest(
        method: String,
        url: String,
        headers: Map<String, String> = emptyMap(),
        body: ByteArray? = null
    ): IrgoResponse {
        val headersJson = JSONObject(headers).toString()
        val response = Irgo.handleRequest(method, url, headersJson, body)
            ?: return IrgoResponse(500, emptyMap(), ByteArray(0))

        return IrgoResponse.from(response)
    }

    /**
     * Get the initial HTML page content
     */
    fun renderInitialPage(): String {
        return Irgo.renderInitialPage()
    }

    /**
     * Enable on-disk persistence for bridge state (the Go cookie jar, so
     * login sessions survive app restarts). Call at startup with an
     * app-private writable directory, e.g. context.filesDir.absolutePath.
     */
    fun setStateDir(dir: String) {
        Irgo.setStateDir(dir)
    }

    /**
     * Remove all cookies, including persisted ones. Useful for logout.
     */
    fun clearCookies() {
        Irgo.clearCookies()
    }

    /**
     * Notify Go that the app moved to the background (call from onPause).
     * Persists bridge state and fires Go-side lifecycle handlers.
     */
    fun onBackground() {
        Irgo.onBackground()
    }

    /**
     * Notify Go that the app returned to the foreground (call from onResume).
     */
    fun onForeground() {
        Irgo.onForeground()
    }

    /**
     * Shutdown the bridge
     */
    fun shutdown() {
        Irgo.shutdown()
    }
}

/**
 * Kotlin-friendly response wrapper
 */
data class IrgoResponse(
    val status: Int,
    val headers: Map<String, String>,
    val body: ByteArray
) {
    val bodyString: String
        get() = String(body, Charsets.UTF_8)

    companion object {
        fun from(response: core.Response): IrgoResponse {
            // Header values may be strings or arrays (multi-value headers
            // such as Set-Cookie); arrays are joined with ", " for this flat
            // map view. Naive getString would corrupt or drop array values.
            val headers = mutableMapOf<String, String>()
            try {
                val headersJson = JSONObject(response.headers)
                headersJson.keys().forEach { key ->
                    when (val value = headersJson.get(key)) {
                        is JSONArray -> {
                            val parts = mutableListOf<String>()
                            for (i in 0 until value.length()) {
                                parts.add(value.optString(i))
                            }
                            headers[key] = parts.joinToString(", ")
                        }
                        else -> headers[key] = value.toString()
                    }
                }
            } catch (e: Exception) {
                // Ignore JSON parsing errors
            }

            return IrgoResponse(
                status = response.status.toInt(),
                headers = headers,
                body = response.body ?: ByteArray(0)
            )
        }
    }

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false
        other as IrgoResponse
        return status == other.status && headers == other.headers && body.contentEquals(other.body)
    }

    override fun hashCode(): Int {
        var result = status
        result = 31 * result + headers.hashCode()
        result = 31 * result + body.contentHashCode()
        return result
    }
}
