//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

void setupMenu(const char* appName, const char* version) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];

        // Create main menu bar
        NSMenu *menuBar = [[NSMenu alloc] init];
        [app setMainMenu:menuBar];

        NSString *appNameStr = [NSString stringWithUTF8String:appName];

        // App menu
        NSMenuItem *appMenuItem = [[NSMenuItem alloc] init];
        [menuBar addItem:appMenuItem];

        NSMenu *appMenu = [[NSMenu alloc] init];
        [appMenuItem setSubmenu:appMenu];

        // About
        NSString *aboutTitle = [NSString stringWithFormat:@"About %@", appNameStr];
        NSMenuItem *aboutItem = [[NSMenuItem alloc]
            initWithTitle:aboutTitle
            action:@selector(orderFrontStandardAboutPanel:)
            keyEquivalent:@""];
        [appMenu addItem:aboutItem];

        [appMenu addItem:[NSMenuItem separatorItem]];

        // Hide App
        NSString *hideTitle = [NSString stringWithFormat:@"Hide %@", appNameStr];
        NSMenuItem *hideItem = [[NSMenuItem alloc]
            initWithTitle:hideTitle
            action:@selector(hide:)
            keyEquivalent:@"h"];
        [appMenu addItem:hideItem];

        // Hide Others
        NSMenuItem *hideOthersItem = [[NSMenuItem alloc]
            initWithTitle:@"Hide Others"
            action:@selector(hideOtherApplications:)
            keyEquivalent:@"h"];
        [hideOthersItem setKeyEquivalentModifierMask:NSEventModifierFlagCommand | NSEventModifierFlagOption];
        [appMenu addItem:hideOthersItem];

        // Show All
        NSMenuItem *showAllItem = [[NSMenuItem alloc]
            initWithTitle:@"Show All"
            action:@selector(unhideAllApplications:)
            keyEquivalent:@""];
        [appMenu addItem:showAllItem];

        [appMenu addItem:[NSMenuItem separatorItem]];

        // Quit
        NSString *quitTitle = [NSString stringWithFormat:@"Quit %@", appNameStr];
        NSMenuItem *quitItem = [[NSMenuItem alloc]
            initWithTitle:quitTitle
            action:@selector(terminate:)
            keyEquivalent:@"q"];
        [appMenu addItem:quitItem];

        // Edit menu (for copy/paste support in webview)
        NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
        [menuBar addItem:editMenuItem];

        NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
        [editMenuItem setSubmenu:editMenu];

        NSMenuItem *undoItem = [[NSMenuItem alloc]
            initWithTitle:@"Undo"
            action:@selector(undo:)
            keyEquivalent:@"z"];
        [editMenu addItem:undoItem];

        NSMenuItem *redoItem = [[NSMenuItem alloc]
            initWithTitle:@"Redo"
            action:@selector(redo:)
            keyEquivalent:@"Z"];
        [editMenu addItem:redoItem];

        [editMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *cutItem = [[NSMenuItem alloc]
            initWithTitle:@"Cut"
            action:@selector(cut:)
            keyEquivalent:@"x"];
        [editMenu addItem:cutItem];

        NSMenuItem *copyItem = [[NSMenuItem alloc]
            initWithTitle:@"Copy"
            action:@selector(copy:)
            keyEquivalent:@"c"];
        [editMenu addItem:copyItem];

        NSMenuItem *pasteItem = [[NSMenuItem alloc]
            initWithTitle:@"Paste"
            action:@selector(paste:)
            keyEquivalent:@"v"];
        [editMenu addItem:pasteItem];

        NSMenuItem *selectAllItem = [[NSMenuItem alloc]
            initWithTitle:@"Select All"
            action:@selector(selectAll:)
            keyEquivalent:@"a"];
        [editMenu addItem:selectAllItem];

        // Window menu
        NSMenuItem *windowMenuItem = [[NSMenuItem alloc] init];
        [menuBar addItem:windowMenuItem];

        NSMenu *windowMenu = [[NSMenu alloc] initWithTitle:@"Window"];
        [windowMenuItem setSubmenu:windowMenu];

        NSMenuItem *minimizeItem = [[NSMenuItem alloc]
            initWithTitle:@"Minimize"
            action:@selector(performMiniaturize:)
            keyEquivalent:@"m"];
        [windowMenu addItem:minimizeItem];

        NSMenuItem *zoomItem = [[NSMenuItem alloc]
            initWithTitle:@"Zoom"
            action:@selector(performZoom:)
            keyEquivalent:@""];
        [windowMenu addItem:zoomItem];

        [windowMenu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *bringAllItem = [[NSMenuItem alloc]
            initWithTitle:@"Bring All to Front"
            action:@selector(arrangeInFront:)
            keyEquivalent:@""];
        [windowMenu addItem:bringAllItem];

        [app setWindowsMenu:windowMenu];
    }
}
*/
import "C"

// SetupMenu configures the native macOS menu bar with standard menus.
// This should be called before creating the webview.
func SetupMenu(appName, version string) {
	C.setupMenu(C.CString(appName), C.CString(version))
}
