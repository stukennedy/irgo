package com.irgo.{{PROJECT_IDENT}}

import com.irgo.IrgoActivity

/**
 * Entry-point activity for {{PROJECT_NAME}}.
 *
 * Inherits all behavior from IrgoActivity (in the AAR):
 *   - Initializes the Go bridge at onCreate
 *   - Creates and configures the WebView
 *   - Loads the initial page from Go (renderInitialPage)
 *   - Handles back navigation via WebView history
 *
 * Override createWebView() or loadInitialPage() in this class if you need
 * to customize the WebView settings or initial route.
 */
class MainActivity : IrgoActivity()
