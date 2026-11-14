package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	if pk.Location.Screenshot != fmt.Sprintf("/v1/issues/%d/screenshot", iss.ID) {
		t.Fatalf("packet screenshot: %q", pk.Location.Screenshot)
	}
	// Agents fetch it with their key once the issue is approved; the secret key always can.
	if rec := e.req("GET", pk.Location.Screenshot, "", "Authorization", "Bearer "+e.sk); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("api screenshot: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := e.req("GET", pk.Location.Screenshot, "", "Authorization", "Bearer "+e.ak); rec.Code != 404 {
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
	if access["enabled"] != true || access["screenshots"] != false {
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
		return map[string]any{"ask": true, "question": "  Which number looks wrong,\n and what should it be? "}, nil
	}}
	e.a.ai = fc.call
	_, rt := e.invited("ana@example.com")
	_, other := e.invited("ben@example.com")
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNG)
	out := e.reportWith(rt, map[string]any{"screenshot": png})
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
	if len(fc.images) != 1 || fc.images[0] != 1 || !strings.Contains(fc.prompts[0], "<on_the_page>Ledger check › Checks failing</on_the_page>") {
		t.Fatalf("Claude didn't get the screenshot and context: %v", fc.images)
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
	iss, _ := e.a.Issue(ctx, e.p, int64(out["issue"].(float64)))
	pk, _ := e.a.BuildPacket(ctx, e.p, iss)
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
