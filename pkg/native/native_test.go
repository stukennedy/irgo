package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// resetState clears package globals between tests.
func resetState() {
	mu.Lock()
	handlers = make(map[string]Handler)
	invoker = nil
	mu.Unlock()
}

func TestCallGoHandler(t *testing.T) {
	resetState()
	Register("echo.upper", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return map[string]string{"text": strings.ToUpper(p.Text)}, nil
	})

	result, err := Call(context.Background(), "echo.upper", Params{"text": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"text":"HI"}` {
		t.Errorf("result = %s", result)
	}
}

func TestCallNotSupported(t *testing.T) {
	resetState()
	_, err := Call(context.Background(), "no.such", nil)
	if !errors.Is(err, ErrNotSupported) {
		t.Errorf("err = %v, want ErrNotSupported", err)
	}
}

// fakeInvoker simulates the platform side.
type fakeInvoker struct {
	respond func(callID, method, paramsJSON string)
}

func (f *fakeInvoker) Invoke(callID, method, paramsJSON string) {
	go f.respond(callID, method, paramsJSON)
}

func TestCallViaInvoker(t *testing.T) {
	resetState()
	SetInvoker(&fakeInvoker{respond: func(callID, method, paramsJSON string) {
		if method != "haptics.impact" {
			DeliverResult(callID, false, "unexpected method")
			return
		}
		DeliverResult(callID, true, `{"done":true}`)
	}})
	defer resetState()

	result, err := Call(context.Background(), "haptics.impact", Params{"style": "light"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"done":true}` {
		t.Errorf("result = %s", result)
	}
}

func TestInvokerNotSupportedFallsBackToGo(t *testing.T) {
	resetState()
	SetInvoker(&fakeInvoker{respond: func(callID, method, paramsJSON string) {
		DeliverResult(callID, false, errNotSupportedMarker)
	}})
	Register("clipboard.read", func(ctx context.Context, params json.RawMessage) (any, error) {
		return map[string]string{"text": "from-go"}, nil
	})
	defer resetState()

	result, err := Call(context.Background(), "clipboard.read", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "from-go") {
		t.Errorf("result = %s", result)
	}
}

func TestCallInvokerError(t *testing.T) {
	resetState()
	SetInvoker(&fakeInvoker{respond: func(callID, method, paramsJSON string) {
		DeliverResult(callID, false, "permission denied")
	}})
	defer resetState()

	_, err := Call(context.Background(), "camera.open", nil)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v", err)
	}
}

func TestCallContextCancelled(t *testing.T) {
	resetState()
	SetInvoker(&fakeInvoker{respond: func(callID, method, paramsJSON string) {
		// Never respond.
	}})
	defer resetState()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Call(ctx, "hangs.forever", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestHTTPHandler(t *testing.T) {
	resetState()
	Register("math.add", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct{ A, B int }
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return map[string]int{"sum": p.A + p.B}, nil
	})
	defer resetState()

	req := httptest.NewRequest("POST", "/_irgo/native",
		strings.NewReader(`{"method":"math.add","params":{"a":2,"b":3}}`))
	w := httptest.NewRecorder()
	HTTPHandler(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || string(resp.Result) != `{"sum":5}` {
		t.Errorf("resp = %s", w.Body.String())
	}
}

func TestHTTPHandlerNotSupported(t *testing.T) {
	resetState()
	req := httptest.NewRequest("POST", "/_irgo/native",
		strings.NewReader(`{"method":"no.such"}`))
	w := httptest.NewRecorder()
	HTTPHandler(w, req)

	if w.Code != 501 {
		t.Errorf("status = %d, want 501", w.Code)
	}
}

func TestHTTPHandlerBadRequest(t *testing.T) {
	resetState()
	req := httptest.NewRequest("POST", "/_irgo/native", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	HTTPHandler(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
