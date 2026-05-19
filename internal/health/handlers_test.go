package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tkhq/talos-state-metrics/internal/state"
)

func TestHealthzAlwaysReturnsOK(t *testing.T) {
	recorder := httptest.NewRecorder()
	Healthz().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected healthz status 200, got %d", recorder.Code)
	}
}

func TestReadyzFollowsWatchConnection(t *testing.T) {
	s := state.NewSnapshot()
	handler := Readyz(s)

	disconnected := httptest.NewRecorder()
	handler.ServeHTTP(disconnected, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if disconnected.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected disconnected readyz status 503, got %d", disconnected.Code)
	}

	s.SetConnected(true, time.Unix(1, 0).UTC())
	connected := httptest.NewRecorder()
	handler.ServeHTTP(connected, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if connected.Code != http.StatusOK {
		t.Fatalf("expected connected readyz status 200, got %d", connected.Code)
	}
}
