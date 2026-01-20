package com.irgo

import android.webkit.JavascriptInterface
import irgo.Irgo

/**
 * JavaScript interface for virtual WebSocket connections.
 * Exposed as IrgoNative in JavaScript.
 */
class IrgoWebSocketInterface(private val activity: IrgoActivity) : Irgo.WebSocketCallback {

    private val activeSessions = mutableSetOf<String>()

    init {
        Irgo.setWebSocketCallback(this)
    }

    /**
     * Connect to a virtual WebSocket.
     * Called from JavaScript: IrgoNative.wsConnect(url)
     */
    @JavascriptInterface
    fun wsConnect(url: String): String {
        return try {
            val sessionID = Irgo.webSocketConnect(url)
            activeSessions.add(sessionID)
            sessionID
        } catch (e: Exception) {
            ""
        }
    }

    /**
     * Send a message through a virtual WebSocket.
     * Called from JavaScript: IrgoNative.wsSend(sessionID, data)
     */
    @JavascriptInterface
    fun wsSend(sessionID: String, data: String): String? {
        return try {
            Irgo.webSocketSend(sessionID, data)
        } catch (e: Exception) {
            null
        }
    }

    /**
     * Close a virtual WebSocket connection.
     * Called from JavaScript: IrgoNative.wsClose(sessionID)
     */
    @JavascriptInterface
    fun wsClose(sessionID: String) {
        try {
            Irgo.webSocketClose(sessionID)
        } catch (e: Exception) {
            // Ignore errors on close
        }
        activeSessions.remove(sessionID)
    }

    // WebSocketCallback implementation

    override fun onMessage(sessionID: String?, data: String?) {
        if (sessionID == null || data == null) return

        val escaped = data
            .replace("\\", "\\\\")
            .replace("'", "\\'")
            .replace("\n", "\\n")
            .replace("\r", "\\r")

        activity.runOnUiThread {
            activity.evaluateJavaScript(
                "window._irgo_ws_message('$sessionID', '$escaped')"
            )
        }
    }

    override fun onClose(sessionID: String?, code: Long, reason: String?) {
        if (sessionID == null) return

        activeSessions.remove(sessionID)

        val reasonEscaped = (reason ?: "").replace("'", "\\'")

        activity.runOnUiThread {
            activity.evaluateJavaScript(
                "window._irgo_ws_close('$sessionID', $code, '$reasonEscaped')"
            )
        }
    }

    override fun onError(sessionID: String?, error: String?) {
        if (sessionID == null || error == null) return

        val errorEscaped = error.replace("'", "\\'")

        activity.runOnUiThread {
            activity.evaluateJavaScript(
                "window._irgo_ws_error('$sessionID', '$errorEscaped')"
            )
        }
    }

    /**
     * Close all active sessions
     */
    fun closeAll() {
        activeSessions.toList().forEach { wsClose(it) }
    }
}
