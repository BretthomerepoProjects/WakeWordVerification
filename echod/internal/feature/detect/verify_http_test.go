package detect

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWakeVerifierVerdict(t *testing.T) {
	for _, tc := range []struct {
		name, reason               string
		approved, stale, wantError bool
	}{
		{"accept", "phrase_match", true, false, false},
		{"reject", "phrase_mismatch", false, false, false},
		{"stale", "phrase_match", true, true, true},
		{"contradictory", "stt_timeout", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" ||
					r.Header.Get("X-Wake-Score") != "1" || len(body) != 2 {
					t.Error("invalid wake request")
				}
				id := r.Header.Get("X-Wake-Event-Id")
				if tc.stale {
					id = "different"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"event_id": id, "approved": tc.approved, "reason": tc.reason,
				})
			}))
			defer server.Close()
			approved, sent, err := askWakeVerifier(context.Background(), server.URL+wakeVerifierPath, []byte{0, 0}, 1)
			if !sent || (err != nil) != tc.wantError || approved != (tc.approved && !tc.wantError) {
				t.Fatalf("approved=%v sent=%v err=%v", approved, sent, err)
			}
		})
	}
}
