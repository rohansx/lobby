package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Voice lets reporters speak instead of typing. The widget records up to 30
// seconds; Lobby passes the audio to Sarvam AI, which transcribes it and
// translates it to English, and hands the text back for the reporter to check
// and edit. The audio is never stored. Optional: without SARVAM_API_KEY, or
// with a waitlist's Voice switch off, the widget shows no mic button.

const (
	maxVoiceAudio       = 2 << 20 // 30 s of Opus is well under this
	voicePerReporterDay = 40
	voicePerProjectDay  = 400
	voiceTimeout        = 40 * time.Second
)

var sarvamSpeechEndpoint = "https://api.sarvam.ai/speech-to-text" // a var so tests can point it elsewhere

var (
	ErrVoiceUnavailable = errors.New("voice input isn't available here")
	ErrVoiceLimit       = errors.New("that's a lot of recordings today; please type instead")
)

// voiceText turns audio into English text and the spoken language's BCP-47 code.
// It is a field so tests can swap in a fake.
type voiceText func(ctx context.Context, audio []byte, mime string) (text, lang string, err error)

func newSarvam(cfg Config) voiceText {
	if cfg.SarvamKey == "" {
		return nil
	}
	client := &http.Client{Timeout: voiceTimeout}
	return func(ctx context.Context, audio []byte, mime string) (string, string, error) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		ext := map[string]string{"audio/webm": "webm", "audio/ogg": "ogg", "audio/mp4": "m4a", "audio/mpeg": "mp3", "audio/wav": "wav"}[mime]
		part, err := w.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="file"; filename="speech.` + ext + `"`},
			"Content-Type":        {mime},
		})
		if err != nil {
			return "", "", err
		}
		part.Write(audio)
		w.WriteField("model", "saaras:v3")
		w.WriteField("mode", "translate") // any Indian language (or English) in, English out
		w.WriteField("language_code", "unknown")
		w.Close()
		req, err := http.NewRequestWithContext(ctx, "POST", sarvamSpeechEndpoint, &body)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("api-subscription-key", cfg.SarvamKey)
		res, err := client.Do(req)
		if err != nil {
			return "", "", fmt.Errorf("sarvam: %w", err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if res.StatusCode != 200 {
			return "", "", fmt.Errorf("sarvam: %d %s", res.StatusCode, clipRunes(string(raw), 300))
		}
		var out struct {
			Transcript   string  `json:"transcript"`
			LanguageCode *string `json:"language_code"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", "", fmt.Errorf("sarvam: bad JSON: %w", err)
		}
		lang := ""
		if out.LanguageCode != nil {
			lang = *out.LanguageCode
		}
		return strings.TrimSpace(out.Transcript), lang, nil
	}
}

// voiceMime keeps only the audio types browsers record, without codec parameters.
func voiceMime(contentType string) string {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch m {
	case "audio/webm", "audio/ogg", "audio/mp4", "audio/mpeg", "audio/wav":
		return m
	case "audio/x-m4a", "audio/aac":
		return "audio/mp4"
	}
	return ""
}

// VoiceEnabled says whether this waitlist's widget shows the mic button.
func (a *App) VoiceEnabled(ctx context.Context, p *Project) bool {
	return a.voice != nil && a.FeedbackSettings(ctx, p).Voice
}

// Transcribe turns a reporter's recording into English text. Nothing is stored.
func (a *App) Transcribe(ctx context.Context, p *Project, reporter string, audio []byte, contentType string) (string, string, error) {
	if !a.VoiceEnabled(ctx, p) || a.ownerSuspended(ctx, p.OwnerID) {
		return "", "", ErrVoiceUnavailable
	}
	signupID, ok := a.parseReporter(reporter)
	if !ok || !a.ReporterAllowed(ctx, p, reporter) {
		return "", "", ErrNotReporter
	}
	mime := voiceMime(contentType)
	if mime == "" || len(audio) == 0 {
		return "", "", &ValidationError{"Send the recording as WebM, Ogg, MP4, MP3 or WAV audio."}
	}
	if len(audio) > maxVoiceAudio {
		return "", "", &ValidationError{"That recording is too long. Keep it under 30 seconds."}
	}
	if !a.spam.Allow("voice:"+strconv.FormatInt(signupID, 10), voicePerReporterDay, 24*time.Hour) ||
		!a.spam.Allow("voicep:"+strconv.FormatInt(p.ID, 10), voicePerProjectDay, 24*time.Hour) {
		return "", "", ErrVoiceLimit
	}
	text, lang, err := a.voice(ctx, audio, mime)
	if err != nil {
		a.log.Warn("voice", "project", p.ID, "err", err)
		return "", "", err
	}
	return clipRunes(text, maxFeedbackComment), lang, nil
}
