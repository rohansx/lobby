package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (e *fbEnv) voice(rt, contentType string, audio []byte, origin string) (int, map[string]any) {
	req := httptest.NewRequest("POST", "/v1/feedback/voice", strings.NewReader(string(audio)))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Lobby-Project", e.p.PublicKey)
	req.Header.Set("X-Lobby-Reporter", rt)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestFeedbackVoice(t *testing.T) {
	e := newFBEnv(t)
	ctx := context.Background()
	_, rt := e.invited("ana@example.com")
	const app = "https://app.example.com"
	audio := []byte("\x1aE\xdf\xa3 fake webm")

	// No Sarvam key on this instance: no mic, and the endpoint refuses.
	if code, _ := e.voice(rt, "audio/webm", audio, app); code != 403 {
		t.Fatalf("without a key: %d", code)
	}
	var gotMime string
	var calls int
	e.a.voice = func(_ context.Context, b []byte, mime string) (string, string, error) {
		calls++
		gotMime = mime
		return "  The total on this card is wrong  ", "hi-IN", nil
	}
	rec := e.req("GET", "/v1/feedback/access?project="+e.p.PublicKey+"&reporter="+rt, "", "Origin", app)
	var access map[string]any
	json.Unmarshal(rec.Body.Bytes(), &access)
	if access["voice"] != true {
		t.Fatalf("access should offer voice: %v", access)
	}

	code, out := e.voice(rt, "audio/webm;codecs=opus", audio, app)
	if code != 200 || out["text"] != "The total on this card is wrong" || out["language"] != "hi-IN" || gotMime != "audio/webm" {
		t.Fatalf("transcribe: %d %v (mime %q)", code, out, gotMime)
	}
	for _, c := range []struct {
		name, rt, ct, origin string
		audio                []byte
		want                 int
	}{
		{"bad token", "rt_nope", "audio/webm", app, audio, 401},
		{"other origin", rt, "audio/webm", "https://evil.example", audio, 403},
		{"not audio", rt, "text/html", app, audio, 400},
		{"empty", rt, "audio/webm", app, nil, 400},
		{"too long", rt, "audio/webm", app, make([]byte, maxVoiceAudio+1), 400},
	} {
		if code, _ := e.voice(c.rt, c.ct, c.audio, c.origin); code != c.want {
			t.Errorf("%s: got %d want %d", c.name, code, c.want)
		}
	}
	if calls != 1 {
		t.Fatalf("refused requests reached Sarvam: %d calls", calls)
	}

	// The founder's switch turns it off.
	e.a.SetFeedbackFlag(ctx, e.p, "voice", false)
	if code, _ := e.voice(rt, "audio/webm", audio, app); code != 403 {
		t.Fatalf("switched off: %d", code)
	}
	// Preflight allows the headers the widget sends.
	pre := e.req("OPTIONS", "/v1/feedback/voice", "", "Origin", app)
	if !strings.Contains(pre.Header().Get("Access-Control-Allow-Headers"), "X-Lobby-Reporter") {
		t.Fatalf("preflight headers: %q", pre.Header().Get("Access-Control-Allow-Headers"))
	}
}

// The real client sends what Sarvam's speech-to-text API expects.
func TestSarvamRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-subscription-key") != "test-key" {
			w.WriteHeader(403)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("file part: %v", err)
		}
		b, _ := io.ReadAll(f)
		if string(b) != "AUDIO" || h.Filename != "speech.webm" || h.Header.Get("Content-Type") != "audio/webm" {
			t.Errorf("file part: %q %q %q", b, h.Filename, h.Header.Get("Content-Type"))
		}
		if r.FormValue("model") != "saaras:v3" || r.FormValue("mode") != "translate" || r.FormValue("language_code") != "unknown" {
			t.Errorf("fields: %v", r.MultipartForm.Value)
		}
		w.Write([]byte(`{"request_id":"r1","transcript":" Save does nothing ","language_code":"ta-IN"}`))
	}))
	defer srv.Close()
	old := sarvamSpeechEndpoint
	sarvamSpeechEndpoint = srv.URL
	defer func() { sarvamSpeechEndpoint = old }()

	text, lang, err := newSarvam(Config{SarvamKey: "test-key"})(context.Background(), []byte("AUDIO"), "audio/webm")
	if err != nil || text != "Save does nothing" || lang != "ta-IN" {
		t.Fatalf("got %q %q %v", text, lang, err)
	}
	if _, _, err := newSarvam(Config{SarvamKey: "wrong"})(context.Background(), []byte("AUDIO"), "audio/webm"); err == nil {
		t.Fatal("a 403 from Sarvam should be an error")
	}
	if newSarvam(Config{}) != nil {
		t.Fatal("no key should mean no voice")
	}
}
