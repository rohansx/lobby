package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// A PNG header is enough for the type check.
var testPNG = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)

func (e *fbEnv) reportWith(rt string, extra map[string]any) map[string]any {
	e.t.Helper()
	body := map[string]any{"project": e.p.PublicKey, "reporter": rt, "type": "bug", "comment": "this is wrong",
		"url": "/ledger", "selector": "main > div", "tag": "div", "context": "Ledger check › Checks failing"}
	for k, v := range extra {
		body[k] = v
	}
	b, _ := json.Marshal(body)
	rec := e.req("POST", "/v1/feedback", string(b), "Origin", "https://app.example.com")
	if rec.Code != 201 {
		e.t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestFeedbackScreenshots(t *testing.T) {
	e := newFBEnv(t)
	ctx := context.Background()
	_, rt := e.invited("ana@example.com")
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNG)

	out := e.reportWith(rt, map[string]any{"screenshot": png})
	rid := int64(out["report"].(float64))
	im, err := e.a.ReportShot(ctx, e.p, rid)
	if err != nil || im.Mime != "image/png" || len(im.Data) != len(testPNG) {
		t.Fatalf("screenshot not stored: %v", err)
	}
	iss, _ := e.a.Issue(ctx, e.p, int64(out["issue"].(float64)))
	reps, _ := e.a.Reports(ctx, e.p, iss.ID)
	if !reps[0].HasScreenshot {
		t.Fatal("report doesn't say it has a screenshot")
	}
	pk, _ := e.a.BuildPacket(ctx, e.p, iss)
	// The packet carries a signed link that opens without a key, so a pasted prompt works.
	link := strings.TrimPrefix(pk.Location.Screenshot, e.a.cfg.BaseURL)
	if !strings.HasPrefix(link, "/v1/shots/") || !strings.Contains(pk.Markdown(), pk.Location.Screenshot) {
		t.Fatalf("packet screenshot link: %q", pk.Location.Screenshot)
	}
	if rec := e.req("GET", link, ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("signed link: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := e.req("GET", link[:len(link)-1]+"0", ""); rec.Code != 404 {
		t.Fatalf("tampered link served: %d", rec.Code)
	}
	expired := strconv.FormatInt(rid, 36) + "." + strconv.FormatInt(now()-1, 36)
	if rec := e.req("GET", "/v1/shots/"+expired+"."+e.a.tokenSig("shot", expired), ""); rec.Code != 404 {
		t.Fatalf("expired link served: %d", rec.Code)
	}
	// With a key: the secret key always; the agent key only once the issue is approved.
	path := fmt.Sprintf("/v1/issues/%d/screenshot", iss.ID)
	if rec := e.req("GET", path, "", "Authorization", "Bearer "+e.sk); rec.Code != 200 {
		t.Fatalf("api screenshot: %d", rec.Code)
	}
	if rec := e.req("GET", path, "", "Authorization", "Bearer "+e.ak); rec.Code != 404 {
		t.Fatalf("agent key read an unapproved issue's screenshot: %d", rec.Code)
	}

	// A lying or oversized image is dropped; the report still goes in.
	for _, bad := range []string{
		"data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(testPNG), // PNG bytes called JPEG
		"data:image/svg+xml;base64,PHN2Zz4=",
		"data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, maxScreenshot+1)),
		"not a data url",
	} {
		out := e.reportWith(rt, map[string]any{"screenshot": bad, "url": "/bad"})
		if _, err := e.a.ReportShot(ctx, e.p, int64(out["report"].(float64))); err == nil {
			t.Fatalf("kept a bad screenshot: %.40s", bad)
		}
	}

	// Switched off, the widget is told not to capture and the server ignores one anyway.
	e.a.SetFeedbackFlag(ctx, e.p, "screenshots", false)
	rec := e.req("GET", "/v1/feedback/access?project="+e.p.PublicKey+"&reporter="+rt, "", "Origin", "https://app.example.com")
	var access map[string]any
	json.Unmarshal(rec.Body.Bytes(), &access)
	if access["enabled"] != true || access["screenshots"] != false || access["masks"] != false {
		t.Fatalf("access: %s", rec.Body.String())
	}
	out = e.reportWith(rt, map[string]any{"screenshot": png, "url": "/off"})
	if _, err := e.a.ReportShot(ctx, e.p, int64(out["report"].(float64))); err == nil {
		t.Fatal("stored a screenshot with screenshots off")
	}
}

func TestFeedbackFollowup(t *testing.T) {
	e := newFBEnv(t)
	ctx := context.Background()
	fc := &fakeClaude{reply: func(user string) (any, error) {
		return map[string]any{"ask": true, "question": "  Which number looks wrong,\n and what should it be? ",
			"choices": []string{"The total", " The total ", "The date", ""}}, nil
	}}
	e.a.ai = fc.call
	_, rt := e.invited("ana@example.com")
	_, other := e.invited("ben@example.com")
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNG)
	out := e.reportWith(rt, map[string]any{"screenshot": png, "state": "disabled,  loading",
		"steps": []string{"go /engagements/:id", "click button · Run checks"}})
	path := fmt.Sprintf("/v1/feedback/reports/%d/", int64(out["report"].(float64)))
	call := func(action, reporter, answer string) (int, map[string]any) {
		b, _ := json.Marshal(map[string]any{"project": e.p.PublicKey, "reporter": reporter, "answer": answer})
		rec := e.req("POST", path+action, string(b), "Origin", "https://app.example.com")
		var m map[string]any
		json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}

	if code, _ := call("question", other, ""); code != 404 {
		t.Fatalf("another reporter asked about this report: %d", code)
	}
	code, m := call("question", rt, "")
	if code != 200 || m["question"] != "Which number looks wrong, and what should it be?" {
		t.Fatalf("question: %d %v", code, m)
	}
	if ch, _ := m["choices"].([]any); len(ch) != 2 || ch[0] != "The total" || ch[1] != "The date" {
		t.Fatalf("choices not cleaned: %v", m["choices"])
	}
	if len(fc.images) != 1 || fc.images[0] != 1 || !strings.Contains(fc.prompts[0], "<on_the_page>Ledger check › Checks failing</on_the_page>") {
		t.Fatalf("Claude didn't get the screenshot and context: %v", fc.images)
	}
	if !strings.Contains(fc.prompts[0], "<element_state>disabled, loading</element_state>") || !strings.Contains(fc.prompts[0], "<step>click button · Run checks</step>") {
		t.Fatal("Claude didn't get the element state and steps")
	}
	iss := issueByPath(t, e, "/ledger")
	pk, _ := e.a.BuildPacket(ctx, e.p, iss)
	md := pk.Markdown()
	if pk.Location.State != "disabled, loading" || len(pk.Evidence.Steps) != 2 || !strings.Contains(md, "2. `click button · Run checks`") ||
		!strings.Contains(md, "Element state when reported: `disabled, loading`") {
		t.Fatalf("packet lacks state or steps:\n%s", md)
	}
	if _, m := call("question", rt, ""); m["question"] != "" || len(fc.prompts) != 1 {
		t.Fatal("asked twice about one report")
	}
	if code, _ := call("answer", rt, "  "); code != 400 {
		t.Fatalf("empty answer: %d", code)
	}
	if code, _ := call("answer", rt, "Checks failing says 3, there are none"); code != 200 {
		t.Fatalf("answer: %d", code)
	}
	if code, _ := call("answer", rt, "again"); code != 409 {
		t.Fatalf("second answer: %d", code)
	}
	iss, _ = e.a.Issue(ctx, e.p, int64(out["issue"].(float64)))
	pk, _ = e.a.BuildPacket(ctx, e.p, iss)
	if !strings.Contains(pk.Markdown(), "> Answer: Checks failing says 3, there are none") {
		t.Fatalf("packet lacks the answer:\n%s", pk.Markdown())
	}

	// A clear report gets no question; with AI off nothing is asked at all.
	fc.reply = func(string) (any, error) { return map[string]any{"ask": false, "question": ""}, nil }
	out = e.reportWith(rt, map[string]any{"url": "/clear"})
	path = fmt.Sprintf("/v1/feedback/reports/%d/", int64(out["report"].(float64)))
	if _, m := call("question", rt, ""); m["question"] != "" {
		t.Fatalf("asked about a clear report: %v", m)
	}
	e.a.SetFeedbackFlag(ctx, e.p, "ai", false)
	out = e.reportWith(rt, map[string]any{"url": "/off"})
	path = fmt.Sprintf("/v1/feedback/reports/%d/", int64(out["report"].(float64)))
	before := len(fc.prompts)
	if _, m := call("question", rt, ""); m["question"] != "" || len(fc.prompts) != before {
		t.Fatal("Claude was called with AI off")
	}
}

func TestFeedbackMasksSwitch(t *testing.T) {
	e := newFBEnv(t)
	ctx := context.Background()
	if e.a.FeedbackSettings(ctx, e.p).Masks {
		t.Fatal("privacy masks should be off by default")
	}
	_, rt := e.invited("ana@example.com")
	access := func() map[string]any {
		rec := e.req("GET", "/v1/feedback/access?project="+e.p.PublicKey+"&reporter="+rt, "", "Origin", "https://app.example.com")
		var m map[string]any
		json.Unmarshal(rec.Body.Bytes(), &m)
		return m
	}
	if access()["masks"] != false {
		t.Fatal("access should tell the widget masks are off")
	}
	if err := e.a.SetFeedbackFlag(ctx, e.p, "masks", true); err != nil {
		t.Fatal(err)
	}
	if access()["masks"] != true || !e.a.FeedbackSettings(ctx, e.p).Masks {
		t.Fatal("masks switch didn't stick")
	}
}
