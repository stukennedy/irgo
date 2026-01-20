package com.irgo

import android.webkit.WebView
import irgo.Irgo
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
        fun from(response: irgo.Core.Response): IrgoResponse {
            val headers = mutableMapOf<String, String>()
            try {
                val headersJson = JSONObject(response.headers)
                headersJson.keys().forEach { key ->
                    headers[key] = headersJson.getString(key)
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
