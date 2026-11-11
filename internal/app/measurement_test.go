package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

func visit(e *packEnv, method, path, ua string, hdr ...string) int {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", ua)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code
}

func visitCounts(t *testing.T, a *App) map[string]int64 {
	t.Helper()
	rows, err := a.db.R.Query(`SELECT source, SUM(count) FROM visits GROUP BY source`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var s string
		var n int64
		rows.Scan(&s, &n)
		out[s] = n
	}
	return out
}

func TestVisitCounting(t *testing.T) {
	e := newPackEnv(t)
	for _, v := range []struct {
		method, path, ua string
		hdr              []string
	}{
		{"GET", "/w/orbit?utm_source=Dir-0&utm_campaign=c1", browserUA, nil},
		{"GET", "/w/orbit?utm_source=dir-0", browserUA, nil},
		{"GET", "/w/orbit", browserUA, nil},
		{"GET", "/w/orbit?utm_source=%3Cscript%3E", browserUA, nil},
		{"HEAD", "/w/orbit?utm_source=dir-0", browserUA, nil},
		{"GET", "/w/orbit?utm_source=dir-0", "Mozilla/5.0 (compatible; Googlebot/2.1)", nil},
		{"GET", "/w/orbit?utm_source=dir-0", "Slackbot-LinkExpanding 1.0", nil},
		{"GET", "/w/orbit?utm_source=dir-0", "", nil},
		{"GET", "/w/orbit?utm_source=dir-0", browserUA, []string{"Sec-Purpose", "prefetch;prerender"}},
		{"GET", "/w/orbit?utm_source=dir-0", browserUA, []string{"Purpose", "prefetch"}},
	} {
		if code := visit(e, v.method, v.path, v.ua, v.hdr...); code != 200 {
			t.Fatalf("%s %s: %d", v.method, v.path, code)
		}
	}
	got := visitCounts(t, e.a)
	want := map[string]int64{"dir-0": 2, "": 1, otherSource: 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("visits: %v want %v", got, want)
	}

	// Past the day's cap, new sources fold into (other); known ones still count.
	day := time.Now().UTC().Format("2006-01-02")
	p, _ := e.a.ProjectBySlug(context.Background(), "orbit")
	for i := len(got); i < maxDaySources; i++ {
		e.a.db.W.Exec(`INSERT INTO visits (project_id, day, source, count) VALUES (?, ?, ?, 1)`, p.ID, day, fmt.Sprintf("seed-%d", i))
	}
	visit(e, "GET", "/w/orbit?utm_source=made-up", browserUA)
	visit(e, "GET", "/w/orbit?utm_source=dir-0", browserUA)
	if got := visitCounts(t, e.a); got["made-up"] != 0 || got[otherSource] != 2 || got["dir-0"] != 3 {
		t.Fatalf("source cap: %v", got)
	}
	// One address can only add so many counted visits an hour.
	for i := 0; i < maxIPVisits; i++ {
		visit(e, "GET", "/w/orbit?utm_source=dir-0", browserUA)
	}
	// Six counted so far from this address (four above, two past the cap).
	if got := visitCounts(t, e.a); got["dir-0"] != 3+maxIPVisits-6 {
		t.Fatalf("per-address cap: %v", got)
	}

	// A failed write still serves the page.
	e.a.db.W.Exec(`DROP TABLE visits`)
	if code := visit(e, "GET", "/w/orbit", browserUA); code != 200 {
		t.Fatalf("page broke without visits table: %d", code)
	}
}

func TestVisitsNotCountedSelfHosted(t *testing.T) {
	a := newTestApp(t)
	p := newTestProject(t, a, true)
	srv, _ := NewServer(a)
	e := &packEnv{a: a, h: srv.Handler()}
	visit(e, "GET", "/w/"+p.Slug, browserUA)
	if got := visitCounts(t, a); len(got) != 0 {
		t.Fatalf("self-hosted counted visits: %v", got)
	}
}

func TestQualificationAnswer(t *testing.T) {
	e := newPackEnv(t)
	ctx := context.Background()
	p, _ := e.a.ProjectBySlug(ctx, "orbit")
	sub := func(email, answer string) (*SignupResult, error) {
		return e.a.SubmitSignup(ctx, p, SignupInput{Email: email, Answer: answer, DwellMs: -1})
	}
	// No question: the answer is dropped.
	res, err := sub("ann@example.com", "ignored")
	if err != nil || res.Signup.Answer != "" {
		t.Fatalf("answer without question: %v %v", res, err)
	}
	mustCode(t, e.alice, "PATCH", "/api/admin/projects/orbit", `{"qualification_question":"`+strings.Repeat("q", 201)+`"}`, 400)
	if out := mustCode(t, e.alice, "PATCH", "/api/admin/projects/orbit", `{"qualification_question":"  What do you build?  "}`, 200); out["qualification_question"] != "What do you build?" {
		t.Fatalf("question: %v", out)
	}
	p, _ = e.a.ProjectBySlug(ctx, "orbit")
	if _, err := sub("ben@example.com", strings.Repeat("é", 301)); err == nil || !strings.Contains(err.Error(), "300") {
		t.Fatalf("long answer: %v", err)
	}
	if res, err := sub("cat@example.com", "  "+strings.Repeat("é", 300)+"  "); err != nil || res.Signup.Answer != strings.Repeat("é", 300) {
		t.Fatalf("300-char answer: %v", err)
	}

	// Public endpoint takes "answer"; the hosted page shows the question.
	req := httptest.NewRequest("POST", "/v1/w/"+p.PublicKey+"/signup", strings.NewReader(`{"email":"dan@example.com","answer":" Dev tools "}`))
	req.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(httptest.NewRecorder(), req)
	if su, err := e.a.SignupByEmail(ctx, p.ID, "dan@example.com"); err != nil || su.Answer != "Dev tools" {
		t.Fatalf("public answer: %v %v", su, err)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, httptest.NewRequest("GET", "/w/orbit?utm_source=dir-0", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "What do you build?") || !strings.Contains(body, `name="answer"`) || !strings.Contains(body, `name="utm_source" value="dir-0"`) {
		t.Fatalf("hosted page: %s", body)
	}

	// Signups API and export carry the answer.
	list := mustCode(t, e.alice, "GET", "/api/admin/projects/orbit/signups", "", 200)["data"].([]any)
	if list[0].(map[string]any)["answer"] != "Dev tools" {
		t.Fatalf("signups api: %v", list[0])
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/admin/projects/orbit/export/csv?status=all", nil)
	req.AddCookie(e.alice.cookie)
	e.h.ServeHTTP(rec, req)
	if lines := strings.Split(rec.Body.String(), "\n"); !strings.HasSuffix(lines[0], ",answer,relevant,feedback") || !strings.Contains(rec.Body.String(), "Dev tools") {
		t.Fatalf("csv: %s", rec.Body.String())
	}
}

func TestQuestionIgnoredSelfHosted(t *testing.T) {
	a := newTestApp(t)
	p := newTestProject(t, a, true)
	p.QualificationQuestion = "Why?"
	if err := a.UpdateProject(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if p, _ = a.ProjectByID(context.Background(), p.ID); p.QualificationQuestion != "" {
		t.Fatal("self-hosted stored a question")
	}
}

func TestRelevantOwnerIsolation(t *testing.T) {
	e := newPackEnv(t)
	ctx := context.Background()
	p, _ := e.a.ProjectBySlug(ctx, "orbit")
	su := signup(t, e.a, p, "erin@example.com", "", "")
	path := fmt.Sprintf("/api/admin/projects/orbit/signups/%d/relevant", su.Signup.ID)
	mustCode(t, e.bob, "PUT", path, `{"relevant":true}`, 404)
	mustCode(t, e.bob, "PUT", fmt.Sprintf("/api/admin/projects/bobs/signups/%d/relevant", su.Signup.ID), `{"relevant":true}`, 404)
	mustCode(t, e.alice, "PUT", "/api/admin/projects/orbit/signups/999999/relevant", `{"relevant":true}`, 404)
	check := func(want string) {
		t.Helper()
		got, _ := e.a.Signup(ctx, p.ID, su.Signup.ID)
		b, _ := json.Marshal(got.Relevant)
		if string(b) != want {
			t.Fatalf("relevant: %s want %s", b, want)
		}
	}
	check("null")
	mustCode(t, e.alice, "PUT", path, `{"relevant":false}`, 200)
	check("false")
	mustCode(t, e.alice, "PUT", path, `{"relevant":true}`, 200)
	check("true")
	mustCode(t, e.alice, "PUT", path, `{"relevant":null}`, 200)
	check("null")
}

// reportFixture: two destinations approved, dir-0 with visits and dir-1 without.
func reportFixture(t *testing.T) (*packEnv, *Project) {
	t.Helper()
	e := newPackEnv(t)
	e.toPlanning(t)
	e.addDestinations(t, 2)
	for _, d := range e.destinations {
		mustCode(t, e.ops, "POST", e.opsCampaign+"/tasks", fmt.Sprintf(`{"destination_id":%d}`, d), 201)
	}
	mustCode(t, e.ops, "POST", e.opsCampaign+"/send-for-approval", "", 200)
	mustCode(t, e.alice, "POST", e.campaign+"/approve", "", 200)
	p, _ := e.a.ProjectBySlug(context.Background(), "orbit")
	return e, p
}

func reportOf(t *testing.T, c *client, path string) Report {
	t.Helper()
	code, out := c.do("GET", path, "")
	if code != 200 {
		t.Fatalf("report: %d %v", code, out)
	}
	b, _ := json.Marshal(out)
	var rep Report
	json.Unmarshal(b, &rep)
	return rep
}

func TestCampaignReport(t *testing.T) {
	e, p := reportFixture(t)
	ctx := context.Background()

	// Before any visit: tracking is missing, not zero.
	rep := reportOf(t, e.alice, e.campaign+"/report")
	if rep.VisitsTracked || rep.Rows[0].Visits != nil || rep.Total.Visits != nil || rep.From == nil {
		t.Fatalf("untracked: %+v", rep)
	}

	for i := 0; i < 3; i++ {
		visit(e, "GET", "/w/orbit?utm_source=dir-0&utm_campaign=c1", browserUA)
	}
	visit(e, "GET", "/w/orbit?utm_source=dir-0", "Googlebot")
	add := func(email, source string) *Signup {
		res, err := e.a.SubmitSignup(ctx, p, SignupInput{Email: email, UTMSource: source, DwellMs: -1})
		if err != nil {
			t.Fatal(err)
		}
		return res.Signup
	}
	add("a@example.com", "dir-0")
	b := add("b@example.com", "DIR-0")
	add("a@example.com", "dir-0") // duplicate: same signup
	c := add("c@example.com", "dir-1")
	add("d@example.com", "")
	add("e@example.com", "twitter")
	old := add("f@example.com", "dir-0") // before the window
	gone := add("g@example.com", "dir-1")
	e.a.db.W.Exec(`UPDATE signups SET created_at = 1 WHERE id = ?`, old.ID)
	e.a.Remove(ctx, p, gone.ID, false)
	e.a.Confirm(ctx, p, b.ID)
	e.a.Confirm(ctx, p, c.ID)
	e.a.SetRelevant(ctx, p, c.ID, ptr(true))

	rep = reportOf(t, e.alice, e.campaign+"/report")
	n := func(v *int64) string {
		if v == nil {
			return "nil"
		}
		return fmt.Sprint(*v)
	}
	if !rep.VisitsTracked || len(rep.Rows) != 2 {
		t.Fatalf("report: %+v", rep)
	}
	d0, d1 := rep.Rows[0], rep.Rows[1]
	if d0.Source != "dir-0" || n(d0.Visits) != "3" || d0.Signups != 2 || d0.Confirmed != 1 || d0.Relevant != 0 ||
		d0.Conversion == nil || *d0.Conversion != 2.0/3 || d0.FeeCents != 500 || d0.State != "ready" {
		t.Fatalf("dir-0: %+v", d0)
	}
	if n(d1.Visits) != "0" || d1.Signups != 1 || d1.Confirmed != 1 || d1.Relevant != 1 || d1.Conversion != nil {
		t.Fatalf("dir-1: %+v", d1)
	}
	if rep.Unknown.Signups != 1 || n(rep.Unknown.Visits) != "0" || rep.Other.Signups != 1 || rep.Other.Confirmed != 0 {
		t.Fatalf("unknown/other: %+v %+v", rep.Unknown, rep.Other)
	}
	if rep.Total.Signups != 5 || rep.Total.Confirmed != 2 || n(rep.Total.Visits) != "3" {
		t.Fatalf("total: %+v", rep.Total)
	}
	if rep.PackPriceCents != 9900 || rep.PlatformFeesCents != 1000 || rep.AttributedConfirmed != 2 || n(rep.CostPerConfirmedCents) != "5450" {
		t.Fatalf("money: %+v", rep)
	}
	if rep.Campaign.OperatorMinutes != 0 || rep.Campaign.ReportNote != "" {
		t.Fatalf("founder view leaked: %+v", rep.Campaign)
	}

	// Operator sees the same numbers and the draft note.
	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"report_note":"Draft","operator_minutes":30}`, 200)
	if op := reportOf(t, e.ops, e.opsCampaign+"/report"); op.Total.Signups != 5 || op.Campaign.ReportNote != "Draft" {
		t.Fatalf("operator report: %+v", op)
	}
	if reportOf(t, e.alice, e.campaign+"/report").Campaign.ReportNote != "" {
		t.Fatal("draft note leaked to founder")
	}
}

func TestCampaignReportIsolation(t *testing.T) {
	e, _ := reportFixture(t)
	mustCode(t, e.bob, "GET", e.campaign+"/report", "", 404)
	mustCode(t, e.bob, "GET", e.opsCampaign+"/report", "", 404)
	mustCode(t, e.alice, "GET", e.opsCampaign+"/report", "", 404)
	mustCode(t, e.ops, "GET", "/api/admin/ops/campaigns/999999/report", "", 404)
	mustCode(t, e.alice, "GET", e.campaign+"/report", "", 200)
}

func TestCampaignLinks(t *testing.T) {
	e, _ := reportFixture(t)
	tasks := mustCode(t, e.alice, "GET", e.campaign, "", 200)["tasks"].([]any)
	id := strings.TrimPrefix(e.campaign, "/api/admin/campaigns/")
	if got := tasks[0].(map[string]any)["campaign_link"]; got != "http://lobby.test/w/orbit?utm_source=dir-0&utm_campaign=c"+id {
		t.Fatalf("link: %v", got)
	}
}

func ptr[T any](v T) *T { return &v }
