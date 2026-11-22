package web

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// Arabic toast text must survive the trip through an HTTP header.
func TestToastHeaderIsASCII(t *testing.T) {
	w := httptest.NewRecorder()
	toast(w, "تم تسجيل الملاحظة", "ok")
	h := w.Header().Get("HX-Trigger")
	for i := 0; i < len(h); i++ {
		if h[i] >= 0x80 {
			t.Fatalf("non-ASCII byte in header: %q", h)
		}
	}
	var got map[string]map[string]string
	if err := json.Unmarshal([]byte(h), &got); err != nil || got["toast"]["message"] != "تم تسجيل الملاحظة" {
		t.Fatalf("round trip failed: %v %v", got, err)
	}
}
