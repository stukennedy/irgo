package com.irgo.example

import com.irgo.IrgoActivity

class MainActivity : IrgoActivity() {
    // IrgoActivity handles everything:
    // - Initializing the Go bridge
    // - Setting up the WebView
    // - Loading the initial page
    // - Handling back navigation
}
