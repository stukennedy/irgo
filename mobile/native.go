package mobile

import "github.com/stukennedy/irgo/pkg/native"

// NativeInvoker is implemented by Swift/Kotlin to receive native-capability
// calls made from Go code via native.Call. The implementation dispatches to
// its plugin registry and must eventually call NativeResult with the same
// callID.
//
// If no plugin claims the method, report it with
// NativeResult(callID, false, "IRGO_NOT_SUPPORTED") so the call can fall
// back to Go-registered handlers.
type NativeInvoker interface {
	Invoke(callID, method, paramsJSON string)
}

// SetNativeInvoker registers the platform dispatcher for native-capability
// calls. Called by the native shell during initialization.
func SetNativeInvoker(inv NativeInvoker) {
	if inv == nil {
		native.SetInvoker(nil)
		return
	}
	native.SetInvoker(invokerAdapter{inv})
}

// NativeResult delivers the result of a native-capability call back to Go.
// Called by the native shell when a plugin completes (or fails) a call.
func NativeResult(callID string, ok bool, payloadJSON string) {
	native.DeliverResult(callID, ok, payloadJSON)
}

// invokerAdapter bridges the gomobile-compatible interface to pkg/native.
type invokerAdapter struct {
	inv NativeInvoker
}

func (a invokerAdapter) Invoke(callID, method, paramsJSON string) {
	a.inv.Invoke(callID, method, paramsJSON)
}
