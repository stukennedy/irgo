package com.irgo

import android.os.Handler
import android.os.Looper
import android.util.Base64
import android.webkit.JavascriptInterface
import mobile.Mobile as Irgo
import mobile.StreamCallback
import mobile.StreamHandle
import mobile.WebSocketCallback
import org.json.JSONObject
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors

/**
 * The unified native bridge exposed to JavaScript as `window.Irgo`.
 *
 * irgo-bridge.js (pkg/bridgejs) routes Datastar's fetch() and WebSocket
 * traffic through this interface:
 *
 *  - [httpRequest]        legacy buffered HTTP (whole response at once)
 *  - [httpRequestStream]  streaming HTTP (progressive SSE chunks)
 *  - [httpRequestCancel]  cancel an in-flight streaming request
 *  - [wsConnect] / [wsSend] / [wsClose]  virtual WebSockets
 *  - [nativeInvoke]       native capability calls (haptics, clipboard, ...)
 *
 * All results are delivered back to JavaScript by evaluating the
 * `window._irgo_*` callback functions on the main thread. String arguments
 * are always encoded as JSON string literals and binary payloads as base64,
 * so arbitrary content survives the JS boundary.
 *
 * Call [destroy] from the activity's onDestroy to cancel in-flight work and
 * release the background executor.
 */
class IrgoJSInterface(private val activity: IrgoActivity) {

    private val mainHandler = Handler(Looper.getMainLooper())
    private val executor: ExecutorService = Executors.newCachedThreadPool()

    /** In-flight streaming requests, keyed by the JS-generated request ID. */
    private val inFlightStreams = ConcurrentHashMap<String, StreamHandle>()

    @Volatile
    private var destroyed = false

    // ========================================
    // HTTP - buffered (legacy path)
    // ========================================

    /**
     * Buffered HTTP request. Returns immediately; the request is processed on
     * a background thread and the full response is delivered via
     * `window._irgo_http_response(requestId, status, headersJSON, base64Body)`
     * (or `window._irgo_http_error(requestId, message)` on failure).
     */
    @JavascriptInterface
    fun httpRequest(requestId: String, method: String, url: String, headersJSON: String, body: String) {
        executor.execute {
            try {
                val bodyBytes = if (body.isEmpty()) null else body.toByteArray(Charsets.UTF_8)

                // Pass the headers JSON straight through: Go's
                // core.DecodeHeaders does the tolerant parsing, and a
                // decode/re-encode here would collapse multi-value headers.
                val response = Irgo.handleRequest(method, url, headersJSON, bodyBytes)

                val responseBody = Base64.encodeToString(response?.body ?: ByteArray(0), Base64.NO_WRAP)
                callJs(
                    "window._irgo_http_response",
                    jsString(requestId),
                    (response?.status ?: 500L).toString(),
                    jsString(response?.headers ?: "{}"),
                    jsString(responseBody)
                )
            } catch (e: Exception) {
                callJs(
                    "window._irgo_http_error",
                    jsString(requestId),
                    jsString(e.message ?: "request failed")
                )
            }
        }
    }

    // ========================================
    // HTTP - streaming (progressive SSE)
    // ========================================

    /**
     * Streaming HTTP request. Returns immediately; Go delivers the response
     * progressively from a background goroutine:
     *
     *  - `window._irgo_stream_response(requestId, status, headersJSON)` once
     *  - `window._irgo_stream_chunk(requestId, base64Chunk)` per flush
     *  - `window._irgo_stream_complete(requestId, errorMessageOrEmpty)` last
     *
     * This is what makes SSE work on Android: each flush on the Go side
     * becomes a chunk while the handler is still running.
     */
    @JavascriptInterface
    fun httpRequestStream(requestId: String, method: String, url: String, headersJSON: String, body: String) {
        val bodyBytes = if (body.isEmpty()) null else body.toByteArray(Charsets.UTF_8)

        // Callbacks arrive sequentially on a background thread: onResponse
        // once, onChunk zero or more times, onComplete once (always last).
        val callback = object : StreamCallback {
            @Volatile
            var completed = false

            override fun onResponse(status: Long, responseHeadersJSON: String?) {
                callJs(
                    "window._irgo_stream_response",
                    jsString(requestId),
                    status.toString(),
                    jsString(responseHeadersJSON ?: "{}")
                )
            }

            override fun onChunk(chunk: ByteArray?) {
                if (chunk == null || chunk.isEmpty()) return
                callJs(
                    "window._irgo_stream_chunk",
                    jsString(requestId),
                    jsString(Base64.encodeToString(chunk, Base64.NO_WRAP))
                )
            }

            override fun onComplete(errorMessage: String?) {
                completed = true
                inFlightStreams.remove(requestId)
                callJs(
                    "window._irgo_stream_complete",
                    jsString(requestId),
                    jsString(errorMessage ?: "")
                )
            }
        }

        try {
            val handle = Irgo.handleRequestStream(method, url, headersJSON, bodyBytes, callback)
            if (handle != null) {
                inFlightStreams[requestId] = handle
                // The request may already have completed on the background
                // thread before the handle was stored - don't leak the entry.
                if (callback.completed) {
                    inFlightStreams.remove(requestId)
                }
            }
        } catch (e: Exception) {
            inFlightStreams.remove(requestId)
            callJs(
                "window._irgo_stream_complete",
                jsString(requestId),
                jsString(e.message ?: "stream failed")
            )
        }
    }

    /**
     * Cancel an in-flight streaming request (e.g. AbortController or a
     * cancelled ReadableStream). Safe to call for unknown/completed IDs.
     */
    @JavascriptInterface
    fun httpRequestCancel(requestId: String) {
        inFlightStreams.remove(requestId)?.let { handle ->
            try {
                handle.cancel()
            } catch (e: Exception) {
                // Already completed - nothing to do.
            }
        }
    }

    // ========================================
    // WebSocket
    // ========================================

    /**
     * Open a virtual WebSocket session and synchronously return its session
     * ID. On failure a RuntimeException is thrown, which Chromium surfaces to
     * the JavaScript caller as an exception (rejecting the awaited promise).
     */
    @JavascriptInterface
    fun wsConnect(url: String): String {
        try {
            val sessionId = Irgo.webSocketConnect(url)
            if (sessionId.isNullOrEmpty()) {
                throw RuntimeException("WebSocket connect failed: empty session ID")
            }
            return sessionId
        } catch (e: RuntimeException) {
            throw e
        } catch (e: Exception) {
            throw RuntimeException(e.message ?: "WebSocket connect failed")
        }
    }

    /**
     * Send a message on a virtual WebSocket session. If Go returns a
     * synchronous reply envelope it is delivered to the page just like a
     * server-push message.
     */
    @JavascriptInterface
    fun wsSend(sessionId: String, data: String) {
        executor.execute {
            try {
                val reply = Irgo.webSocketSend(sessionId, data)
                if (!reply.isNullOrEmpty()) {
                    callJs("window._irgo_ws_message", jsString(sessionId), jsString(reply))
                }
            } catch (e: Exception) {
                callJs(
                    "window._irgo_ws_error",
                    jsString(sessionId),
                    jsString(e.message ?: "send failed")
                )
            }
        }
    }

    /**
     * Close a virtual WebSocket session. The Go export takes only the session
     * ID; code/reason are accepted to match the JS call signature (the JS
     * side dispatches its own close event with them).
     */
    @JavascriptInterface
    fun wsClose(sessionId: String, code: Int, reason: String) {
        executor.execute {
            try {
                Irgo.webSocketClose(sessionId)
            } catch (e: Exception) {
                // Session already gone - nothing to do.
            }
        }
    }

    /**
     * Server-push WebSocket events from Go, forwarded to the page. Register
     * with [registerWebSocketCallback] during activity setup.
     */
    val webSocketCallback: WebSocketCallback = object : WebSocketCallback {
        override fun onMessage(sessionID: String?, data: String?) {
            callJs("window._irgo_ws_message", jsString(sessionID ?: ""), jsString(data ?: ""))
        }

        override fun onClose(sessionID: String?, code: Long, reason: String?) {
            callJs(
                "window._irgo_ws_close",
                jsString(sessionID ?: ""),
                code.toString(),
                jsString(reason ?: "")
            )
        }

        override fun onError(sessionID: String?, errorMsg: String?) {
            callJs("window._irgo_ws_error", jsString(sessionID ?: ""), jsString(errorMsg ?: ""))
        }
    }

    /** Register [webSocketCallback] with the Go bridge. */
    fun registerWebSocketCallback() {
        Irgo.setWebSocketCallback(webSocketCallback)
    }

    // ========================================
    // Native capabilities
    // ========================================

    /**
     * Invoke a native capability (e.g. "haptics.impact") via the plugin
     * registry. The result is delivered via
     * `window._irgo_native_result(requestId, ok, payloadJSON)` where
     * payloadJSON is JSON-encoded - or the exact string "IRGO_NOT_SUPPORTED"
     * when no plugin claims the method, letting the JS side fall back to
     * Go-registered handlers.
     */
    @JavascriptInterface
    fun nativeInvoke(requestId: String, method: String, paramsJSON: String) {
        IrgoNative.dispatch(method, paramsJSON) { ok, payloadJSON ->
            callJs(
                "window._irgo_native_result",
                jsString(requestId),
                ok.toString(),
                jsString(payloadJSON)
            )
        }
    }

    // ========================================
    // Lifecycle
    // ========================================

    /**
     * Cancel in-flight requests and shut down the background executor.
     * Call from the activity's onDestroy.
     */
    fun destroy() {
        destroyed = true
        inFlightStreams.values.forEach { handle ->
            try {
                handle.cancel()
            } catch (e: Exception) {
                // Already completed.
            }
        }
        inFlightStreams.clear()
        try {
            Irgo.setWebSocketCallback(null)
        } catch (e: Exception) {
            // Bridge already shut down.
        }
        executor.shutdownNow()
    }

    // ========================================
    // Helpers
    // ========================================

    /**
     * Encode a Kotlin string as a JavaScript string literal. JSON escaping is
     * valid JS except for U+2028/U+2029, which are escaped explicitly.
     */
    private fun jsString(value: String?): String {
        if (value == null) return "null"
        return JSONObject.quote(value)
            .replace("\u2028", "\\u2028")
            .replace("\u2029", "\\u2029")
    }

    /**
     * Evaluate `function(args...)` in the WebView on the main thread. Args
     * must already be encoded as JS literals ([jsString], number/bool
     * toString).
     */
    private fun callJs(function: String, vararg args: String) {
        if (destroyed) return
        val script = "if (typeof $function === 'function') $function(${args.joinToString(",")});"
        mainHandler.post {
            if (destroyed) return@post
            try {
                activity.webView.evaluateJavascript(script, null)
            } catch (e: Exception) {
                // WebView not ready or already destroyed - drop the event.
            }
        }
    }
}
