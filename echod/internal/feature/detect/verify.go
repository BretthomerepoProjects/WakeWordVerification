package detect

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/nearest"
	"github.com/HuskerMinion/techo5/echod/internal/feature/voice"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/mic"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
)

func (d *Detect) cancelVerification() {
	d.verifyMu.Lock()
	defer d.verifyMu.Unlock()
	if d.verifyCancel != nil {
		d.verifyCancel()
	}
}

func (d *Detect) claimVerifiedWake(slot int) {
	at := time.Now()
	model, ok := d.engine.Loaded(slot)
	if !ok || model.Phrase == "" || !voice.Get().CanVerifyWake(slot) {
		return
	}
	samples := mic.Get().Recent(mic.Confirming)
	if len(samples) == 0 {
		return
	}
	score := d.engine.Crossing(slot)
	gate, ok := d.contest.wakeHeard(d.engine.Margin(slot), d.commandsListening() > 0)
	if !ok {
		return
	}
	nearest.Get().WokeVerified(slot, at, func() {
		gate(func() { d.verifyWake(slot, model.ID, samples, score) })
	})
}

func (d *Detect) verifyWake(slot int, modelID string, samples []int16, score float64) {
	model, ok := d.engine.Loaded(slot)
	if !ok || model.ID != modelID || model.Phrase == "" || !voice.Get().CanVerifyWake(slot) {
		return
	}
	d.verifyMu.Lock()
	if d.verifyCancel != nil || !config.Get().Wake.OnHA.VerifySTT {
		d.verifyMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.verifyCancel = cancel
	d.verifyMu.Unlock()
	defer func() {
		cancel()
		d.verifyMu.Lock()
		d.verifyCancel = nil
		d.verifyMu.Unlock()
	}()

	pcm := make([]byte, 2*len(samples))
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(pcm[2*i:], uint16(sample))
	}
	check, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	allowed, blocked, sent, err := verifyCandidate(check, slot, model.Phrase, pcm, score)
	if sent {
		// Persist after the wake handoff so writing the totals cannot delay the chime.
		defer func() {
			counts, saveErr := config.Set().OnHA().RecordSTTWake(allowed, blocked)
			if saveErr != nil {
				slog.Error("saving wake STT counters failed", "err", saveErr)
			}
			d.onHA.publishSTTWakes(counts)
		}()
	}
	if err != nil {
		slog.Debug("wake verification STT failed", "err", err)
		return
	}
	if check.Err() != nil || !allowed {
		return
	}
	current, ok := d.engine.Loaded(slot)
	if !ok || current.ID != model.ID {
		return
	}
	chime, finish := context.WithTimeout(ctx, 3*time.Second)
	defer finish()
	voice.Get().VerifiedWake(chime, slot)
}

// Background speech can add words to the transcript. Match the configured
// phrase as consecutive whole tokens anywhere in the wake clip.
func matchesWakePhrase(transcript, phrase string) bool {
	normalize := func(value string) string {
		words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
		})
		return strings.Join(words, " ")
	}
	want := normalize(phrase)
	heard := " " + normalize(transcript) + " "
	if want == "okay nabu" || want == "ok nabu" {
		return strings.Contains(heard, " okay nabu ") || strings.Contains(heard, " ok nabu ")
	}
	return want != "" && strings.Contains(heard, " "+want+" ")
}

func verifyCandidate(ctx context.Context, slot int, phrase string, pcm []byte, score float64) (allowed, blocked, sent bool, err error) {
	if endpoint := wakeVerifierURL(); endpoint != "" {
		allowed, sent, err = askWakeVerifier(ctx, endpoint, pcm, score)
		return allowed, err == nil && !allowed, sent, err
	}
	mac, err := layout.FactoryMAC()
	if err != nil {
		return false, false, false, err
	}
	pipeline, err := hass.Get().WakePipeline(ctx, mac, slot)
	if err != nil {
		return false, false, false, err
	}
	transcript, sent, err := hass.Get().Transcribe(ctx, pipeline, mic.Rate, pcm)
	allowed = err == nil && matchesWakePhrase(transcript, phrase)
	blocked = err == nil && !allowed || errors.Is(err, hass.ErrSTTNoText)
	return
}
