package detect

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

const wakeVerifierPath = "/api/wake_word_verification/v1/verify"

var wakeVerifierHTTP = &http.Client{
	Transport: &http.Transport{},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func wakeVerifierURL() string {
	data, err := os.ReadFile(layout.StateDir + "/wake-verifier-url")
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "-"
	}
	return strings.TrimSpace(string(data))
}

func askWakeVerifier(ctx context.Context, endpoint string, pcm []byte, score float64) (bool, bool, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.Path != wakeVerifierPath || u.RawQuery != "" || u.Fragment != "" {
		return false, false, fmt.Errorf("invalid wake verifier URL")
	}
	// The integration requires a UUID. Derive its bytes directly from the OS RNG.
	var random [16]byte
	rand.Read(random[:])
	random[6] = random[6]&15 | 64
	random[8] = random[8]&63 | 128
	id := fmt.Sprintf("%x-%x-%x-%x-%x", random[:4], random[4:6], random[6:8], random[8:10], random[10:])
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(pcm))
	if err != nil {
		return false, false, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Wake-Event-Id", id)
	req.Header.Set("X-Wake-Score", strconv.FormatFloat(score, 'f', -1, 64))
	resp, err := wakeVerifierHTTP.Do(req)
	if err != nil {
		return false, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, false, fmt.Errorf("wake verifier returned HTTP %d", resp.StatusCode)
	}
	var verdict struct {
		EventID  string `json:"event_id"`
		Approved *bool  `json:"approved"`
		Reason   string `json:"reason"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4096))
	if decoder.Decode(&verdict) != nil || decoder.Decode(new(any)) != io.EOF ||
		verdict.EventID != id || verdict.Approved == nil || verdict.Reason == "" ||
		*verdict.Approved != (verdict.Reason == "phrase_match") {
		return false, true, fmt.Errorf("invalid wake verifier verdict")
	}
	return *verdict.Approved, true, nil
}
