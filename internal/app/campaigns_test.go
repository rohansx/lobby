package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type packEnv struct {
	a                *App
	h                http.Handler
	ops, alice, bob  *client
	aliceID, bobID   int64
	campaign         string // /api/admin/campaigns/{id}
	opsCampaign      string // /api/admin/ops/campaigns/{id}
	destinations     []int64
	destinationNames []string
}

// newPackEnv makes an operator, alice with a waitlist and a campaign, and bob.
func newPackEnv(t *testing.T) *packEnv {
	t.Helper()
	a := newCloudApp(t)
	a.cfg.Operators = []string{"ops@example.com"}
	srv, _ := NewServer(a)
	e := &packEnv{a: a, h: srv.Handler()}
	e.ops, e.alice, e.bob = &client{t: t, h: e.h}, &client{t: t, h: e.h}, &client{t: t, h: e.h}
	for c, addr := range map[*client]string{e.ops: "ops@example.com", e.alice: "alice@example.com", e.bob: "bob@example.com"} {
		c.do("POST", "/api/admin/signup", `{"email":"`+addr+`","password":"correct horse battery"}`)
	}
	a.db.W.Exec(`UPDATE admins SET verified_at = 1, created_at = 1`)
	a.db.W.QueryRow(`SELECT id FROM admins WHERE email = 'alice@example.com'`).Scan(&e.aliceID)
	a.db.W.QueryRow(`SELECT id FROM admins WHERE email = 'bob@example.com'`).Scan(&e.bobID)
	if code, out := e.alice.do("POST", "/api/admin/projects", `{"name":"Orbit","slug":"orbit"}`); code != 201 {
		t.Fatalf("project: %d %v", code, out)
	}
	e.bob.do("POST", "/api/admin/projects", `{"name":"Bobs","slug":"bobs"}`)
	code, out := e.ops.do("POST", "/api/admin/ops/campaigns", fmt.Sprintf(`{"account_id":%d,"price_cents":9900}`, e.aliceID))
	if code != 201 {
		t.Fatalf("create campaign: %d %v", code, out)
	}
	id := int64(out["id"].(float64))
	e.campaign = fmt.Sprintf("/api/admin/campaigns/%d", id)
	e.opsCampaign = fmt.Sprintf("/api/admin/ops/campaigns/%d", id)
	return e
}

func (e *packEnv) addDestinations(t *testing.T, n int) {
	t.Helper()
	for i := len(e.destinations); i < n; i++ {
		code, out := e.ops.do("POST", "/api/admin/ops/destinations", fmt.Sprintf(`{"slug":"dir-%d","name":"Directory %d","url":"https://dir%d.example.com","stages":"beta,live","fee_cents":500}`, i, i, i))
		if code != 201 {
			t.Fatalf("destination: %d %v", code, out)
		}
		e.destinations = append(e.destinations, int64(out["id"].(float64)))
	}
}

// toPlanning takes the campaign through intake and readiness.
func (e *packEnv) toPlanning(t *testing.T) {
	t.Helper()
	mustCode(t, e.alice, "PUT", e.campaign+"/project", `{"slug":"orbit"}`, 200)
	mustCode(t, e.alice, "POST", "/api/admin/projects/orbit/profile", `{"name":"Orbit","stage":"beta","tagline":"Orbit for teams"}`, 201)
	mustCode(t, e.alice, "POST", e.campaign+"/submit-intake", "", 200)
	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"status":"planning","readiness":[{"label":"Landing page","ok":true,"note":""}]}`, 200)
}

func mustCode(t *testing.T, c *client, method, path, body string, want int) map[string]any {
	t.Helper()
	code, out := c.do(method, path, body)
	if code != want {
		t.Fatalf("%s %s: got %d want %d: %v", method, path, code, want, out)
	}
	return out
}

func (e *packEnv) flushMail(t *testing.T) []*Message {
	t.Helper()
	if _, err := e.a.runJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mc := e.a.mailer.(*captureMailer)
	out := mc.snapshot()
	mc.mu.Lock()
	mc.sent = nil
	mc.mu.Unlock()
	return out
}

func mailTo(msgs []*Message, to, subject string) bool {
	for _, m := range msgs {
		if m.To == to && strings.Contains(m.Subject, subject) {
			return true
		}
	}
	return false
}

func TestCampaignMoves(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		by       actor
		ok       bool
	}{
		{statusIntake, statusReadiness, byFounder, true},
		{statusIntake, statusReadiness, byOperator, false},
		{statusIntake, statusExecuting, byFounder, false},
		{statusPlanning, statusAwaitingApproval, byOperator, true},
		{statusPlanning, statusAwaitingApproval, byFounder, false},
		{statusAwaitingApproval, statusExecuting, byFounder, true},
		{statusAwaitingApproval, statusExecuting, byOperator, false},
		{statusExecuting, statusDone, byOperator, false},
		{statusReporting, statusDone, byOperator, true},
		{statusExecuting, statusCancelled, byOperator, true},
		{statusExecuting, statusCancelled, byFounder, false},
		{statusDone, statusCancelled, byOperator, false},
		{statusCancelled, statusIntake, byOperator, false},
	} {
		if err := checkMove(tc.from, tc.to, tc.by); (err == nil) != tc.ok {
			t.Errorf("%s -> %s by %s: %v", tc.from, tc.to, tc.by, err)
		}
	}
}

func TestCampaignFlow(t *testing.T) {
	e := newPackEnv(t)
	msgs := e.flushMail(t)
	if !mailTo(msgs, "alice@example.com", "describe your product") || !mailTo(msgs, "ops@example.com", "New Launch Pack") {
		t.Fatalf("start emails: %v", msgs)
	}
	if ac, _ := e.a.Account(context.Background(), e.aliceID); ac.Plan != "pack" {
		t.Fatalf("plan: %s", ac.Plan)
	}

	// Intake needs a waitlist and a profile; invalid moves are 409.
	mustCode(t, e.alice, "POST", e.campaign+"/submit-intake", "", 409)
	mustCode(t, e.alice, "POST", e.campaign+"/approve", "", 409)
	mustCode(t, e.alice, "PUT", e.campaign+"/project", `{"slug":"bobs"}`, 404)
	mustCode(t, e.alice, "PUT", e.campaign+"/project", `{"slug":"orbit"}`, 200)
	mustCode(t, e.alice, "POST", e.campaign+"/submit-intake", "", 409)
	mustCode(t, e.alice, "POST", "/api/admin/projects/orbit/profile", `{"name":"Orbit","stage":"beta"}`, 201)
	mustCode(t, e.alice, "POST", e.campaign+"/submit-intake", "", 200)
	mustCode(t, e.alice, "PUT", e.campaign+"/project", `{"slug":"orbit"}`, 409)
	if !mailTo(e.flushMail(t), "ops@example.com", "Intake submitted") {
		t.Fatal("intake email")
	}

	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"status":"executing"}`, 409)
	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"status":"nonsense"}`, 400)
	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"status":"planning","operator_minutes":45}`, 200)
	mustCode(t, e.ops, "POST", e.opsCampaign+"/send-for-approval", "", 409) // empty plan

	e.addDestinations(t, 2)
	staff := mustCode(t, e.ops, "POST", e.opsCampaign+"/tasks", fmt.Sprintf(`{"destination_id":%d,"reason":"Dev tools audience"}`, e.destinations[0]), 201)
	founder := mustCode(t, e.ops, "POST", e.opsCampaign+"/tasks", fmt.Sprintf(`{"destination_id":%d,"owner":"founder","instructions":"Post from your account"}`, e.destinations[1]), 201)
	mustCode(t, e.ops, "POST", e.opsCampaign+"/tasks", fmt.Sprintf(`{"destination_id":%d}`, e.destinations[0]), 409)
	if staff["fee_cents"] != 500.0 || staff["state"] != "planned" {
		t.Fatalf("task defaults: %v", staff)
	}
	mustCode(t, e.ops, "POST", e.opsCampaign+"/send-for-approval", "", 200)
	if !mailTo(e.flushMail(t), "alice@example.com", "ready to approve") {
		t.Fatal("approval email")
	}

	// Request changes goes back to planning with the note; then approve.
	mustCode(t, e.alice, "POST", e.campaign+"/request-changes", `{"note":"Drop the second one"}`, 200)
	if out := mustCode(t, e.ops, "GET", e.opsCampaign, "", 200); out["campaign"].(map[string]any)["changes_note"] != "Drop the second one" {
		t.Fatalf("changes note: %v", out["campaign"])
	}
	if !mailTo(e.flushMail(t), "ops@example.com", "Changes requested") {
		t.Fatal("changes email")
	}
	mustCode(t, e.ops, "POST", e.opsCampaign+"/send-for-approval", "", 200)
	// A profile saved after sending is the one approval pins.
	v2 := mustCode(t, e.alice, "POST", "/api/admin/projects/orbit/profile", `{"name":"Orbit","stage":"live"}`, 201)["version"].(map[string]any)["id"]
	out := mustCode(t, e.alice, "POST", e.campaign+"/approve", "", 200)
	if out["status"] != statusExecuting || out["profile_version_id"] != v2 || out["approved_at"] == nil || out["operator_minutes"] != nil {
		t.Fatalf("approved: %v", out)
	}
	mustCode(t, e.alice, "POST", e.campaign+"/approve", "", 409)
	msgs = e.flushMail(t)
	if !mailTo(msgs, "ops@example.com", "Plan approved") || !mailTo(msgs, "alice@example.com", "needs a step from you") {
		t.Fatalf("approved emails: %v", msgs)
	}
	got := mustCode(t, e.alice, "GET", e.campaign, "", 200)
	tasks := got["tasks"].([]any)
	if tasks[0].(map[string]any)["state"] != "ready" || tasks[1].(map[string]any)["state"] != "awaiting_founder" {
		t.Fatalf("task states after approval: %v", tasks)
	}

	// Founder: own tasks, allowed fields only.
	staffTask := fmt.Sprintf("%s/tasks/%v", e.campaign, staff["id"])
	founderTask := fmt.Sprintf("%s/tasks/%v", e.campaign, founder["id"])
	mustCode(t, e.alice, "PATCH", staffTask, `{"state":"submitted"}`, 409)
	mustCode(t, e.alice, "PATCH", founderTask, `{"state":"submitted","owner":"staff"}`, 400)
	mustCode(t, e.alice, "PATCH", founderTask, `{"state":"ready"}`, 400)
	mustCode(t, e.alice, "PATCH", founderTask, `{"state":"live","listing_url":"javascript:alert(1)"}`, 400)
	ft := mustCode(t, e.alice, "PATCH", founderTask, `{"state":"submitted","listing_url":"https://dir1.example.com/orbit"}`, 200)
	if ft["submitted_at"] == nil || ft["live_at"] != nil {
		t.Fatalf("submitted is not live: %v", ft)
	}
	st := mustCode(t, e.ops, "PATCH", e.opsCampaign+fmt.Sprintf("/tasks/%v", staff["id"]), `{"state":"live","evidence":"screenshot","minutes":20}`, 200)
	if st["live_at"] == nil || st["minutes"] != 20.0 {
		t.Fatalf("staff task: %v", st)
	}
	mustCode(t, e.ops, "DELETE", e.opsCampaign+fmt.Sprintf("/tasks/%v", staff["id"]), "", 409)
	mustCode(t, e.ops, "DELETE", fmt.Sprintf("/api/admin/ops/destinations/%d", e.destinations[0]), "", 409)

	// Report: publish needs reporting and a note.
	mustCode(t, e.ops, "POST", e.opsCampaign+"/publish-report", "", 409)
	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"status":"reporting"}`, 200)
	mustCode(t, e.ops, "POST", e.opsCampaign+"/publish-report", "", 409)
	if c := mustCode(t, e.alice, "GET", e.campaign, "", 200)["campaign"].(map[string]any); c["report_note"] != "" {
		t.Fatalf("draft report leaked: %v", c)
	}
	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"report_note":"Two live listings."}`, 200)
	done := mustCode(t, e.ops, "POST", e.opsCampaign+"/publish-report", "", 200)
	if done["status"] != statusDone || done["report_published_at"] == nil {
		t.Fatalf("done: %v", done)
	}
	if !mailTo(e.flushMail(t), "alice@example.com", "report is ready") {
		t.Fatal("report email")
	}
	mustCode(t, e.ops, "PATCH", e.opsCampaign, `{"status":"cancelled"}`, 409)

	// Campaign mail is account mail: it doesn't use the plan's allowance.
	if u := e.a.UsageOf(context.Background(), e.aliceID); u.Emails != 0 {
		t.Fatalf("campaign mail counted: %d", u.Emails)
	}
}

func TestCampaignOwnerIsolation(t *testing.T) {
	e := newPackEnv(t)
	e.toPlanning(t)
	e.addDestinations(t, 1)
	task := mustCode(t, e.ops, "POST", e.opsCampaign+"/tasks", fmt.Sprintf(`{"destination_id":%d,"owner":"founder"}`, e.destinations[0]), 201)
	mustCode(t, e.ops, "POST", e.opsCampaign+"/send-for-approval", "", 200)
	asset := uploadAsset(t, e, e.alice, "orbit", "logo", "logo.png", pngBytes(100))
	if asset.Code != 201 {
		t.Fatalf("upload: %d %s", asset.Code, asset.Body)
	}

	if out := mustCode(t, e.bob, "GET", "/api/admin/campaigns", "", 200); len(out["data"].([]any)) != 0 {
		t.Fatalf("bob sees campaigns: %v", out)
	}
	for _, rq := range [][3]string{
		{"GET", e.campaign, ""},
		{"PUT", e.campaign + "/project", `{"slug":"bobs"}`},
		{"POST", e.campaign + "/approve", ""},
		{"POST", e.campaign + "/request-changes", `{"note":"x"}`},
		{"POST", e.campaign + "/submit-intake", ""},
		{"PATCH", fmt.Sprintf("%s/tasks/%v", e.campaign, task["id"]), `{"state":"submitted"}`},
		{"GET", "/api/admin/projects/orbit/profile", ""},
		{"POST", "/api/admin/projects/orbit/profile", `{"name":"Mine"}`},
		{"GET", "/api/admin/projects/orbit/assets", ""},
		{"POST", "/api/admin/projects/orbit/assets", ""},
		{"GET", "/api/admin/projects/orbit/assets/1", ""},
		{"DELETE", "/api/admin/projects/orbit/assets/1", ""},
	} {
		mustCode(t, e.bob, rq[0], rq[1], rq[2], 404)
	}
	// Bob's own project can't serve alice's asset id either.
	mustCode(t, e.bob, "GET", "/api/admin/projects/bobs/assets/1", "", 404)
	// The operator can read alice's profile and files, but not write them.
	mustCode(t, e.ops, "GET", "/api/admin/projects/orbit/profile", "", 200)
	mustCode(t, e.ops, "GET", "/api/admin/projects/orbit/assets/1", "", 200)
	mustCode(t, e.ops, "POST", "/api/admin/projects/orbit/profile", `{"name":"Ops"}`, 404)
	mustCode(t, e.ops, "DELETE", "/api/admin/projects/orbit/assets/1", "", 404)
	if c := mustCode(t, e.alice, "GET", e.campaign, "", 200)["campaign"].(map[string]any); c["status"] != statusAwaitingApproval {
		t.Fatalf("bob changed alice's campaign: %v", c)
	}
}

func TestCampaignOperatorRoutes(t *testing.T) {
	e := newPackEnv(t)
	for _, rq := range [][2]string{
		{"GET", "/api/admin/ops/campaigns"}, {"POST", "/api/admin/ops/campaigns"}, {"GET", e.opsCampaign},
		{"PATCH", e.opsCampaign}, {"POST", e.opsCampaign + "/tasks"}, {"PATCH", e.opsCampaign + "/tasks/1"},
		{"DELETE", e.opsCampaign + "/tasks/1"}, {"POST", e.opsCampaign + "/send-for-approval"},
		{"POST", e.opsCampaign + "/publish-report"}, {"GET", "/api/admin/ops/destinations"},
		{"POST", "/api/admin/ops/destinations"}, {"PATCH", "/api/admin/ops/destinations/1"}, {"DELETE", "/api/admin/ops/destinations/1"},
	} {
		mustCode(t, e.alice, rq[0], rq[1], `{"account_id":1}`, 404)
	}
	if out := mustCode(t, e.ops, "GET", "/api/admin/ops/campaigns", "", 200); len(out["data"].([]any)) != 1 {
		t.Fatalf("ops list: %v", out)
	}
	mustCode(t, e.ops, "POST", "/api/admin/ops/destinations", `{"slug":"x","name":"X","url":"ftp://x"}`, 400)
	mustCode(t, e.ops, "POST", "/api/admin/ops/destinations", `{"slug":"x","name":"X","url":"https://x.example","mode":"robot"}`, 400)
	mustCode(t, e.ops, "POST", "/api/admin/ops/destinations", `{"slug":"x","name":"X","url":"https://x.example","stages":"beta,unicorn"}`, 400)
	mustCode(t, e.ops, "GET", "/api/admin/ops/destinations?stage=unicorn", "", 400)
	d := mustCode(t, e.ops, "POST", "/api/admin/ops/destinations", `{"slug":"x","name":"X","url":"https://x.example","stages":"live"}`, 201)
	mustCode(t, e.ops, "PATCH", fmt.Sprintf("/api/admin/ops/destinations/%v", d["id"]), `{"active":false}`, 200)
	if out := mustCode(t, e.ops, "GET", "/api/admin/ops/destinations?stage=live", "", 200); len(out["data"].([]any)) != 0 {
		t.Fatalf("inactive destination listed for a stage: %v", out)
	}
	mustCode(t, e.ops, "DELETE", fmt.Sprintf("/api/admin/ops/destinations/%v", d["id"]), "", 200)
}

func TestCampaignRoutesSelfHosted(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Operators = []string{"admin@example.com"}
	ctx := context.Background()
	if err := a.CreateFirstAdmin(ctx, "admin@example.com", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	a.CreateProject(ctx, 0, "Orbit", "orbit")
	srv, _ := NewServer(a)
	c := &client{t: t, h: srv.Handler()}
	c.do("POST", "/api/admin/login", `{"email":"admin@example.com","password":"correct horse battery"}`)
	for _, rq := range [][2]string{
		{"GET", "/api/admin/campaigns"}, {"GET", "/api/admin/campaigns/1"}, {"POST", "/api/admin/campaigns/1/approve"},
		{"GET", "/api/admin/projects/orbit/profile"}, {"POST", "/api/admin/projects/orbit/profile"},
		{"GET", "/api/admin/projects/orbit/assets"}, {"GET", "/api/admin/ops/campaigns"},
		{"GET", "/api/admin/ops/destinations"}, {"POST", "/api/admin/ops/campaigns"},
	} {
		mustCode(t, c, rq[0], rq[1], `{}`, 404)
	}
}

func TestCreateCampaignIdempotent(t *testing.T) {
	a := newCloudApp(t)
	ctx := context.Background()
	id, _ := a.SignUp(ctx, "alice@example.com", "correct horse battery")
	c1, err := a.CreateCampaign(ctx, id, 9900, "order_1")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := a.CreateCampaign(ctx, id, 9900, "order_1")
	if err != nil || c2.ID != c1.ID {
		t.Fatalf("repeat order made a new campaign: %v %v", c2, err)
	}
	var n int
	a.db.W.QueryRow(`SELECT COUNT(*) FROM campaigns`).Scan(&n)
	if n != 1 || c1.Status != statusIntake || c1.PriceCents != 9900 || c1.PaidAt == nil {
		t.Fatalf("campaigns: %d %+v", n, c1)
	}
	if a.runJobs(ctx); len(a.mailer.(*captureMailer).snapshot()) != 1 { // founder only; no operators configured
		t.Fatalf("emails: %v", a.mailer.(*captureMailer).snapshot())
	}
	ac, _ := a.Account(ctx, id)
	if ac.Plan != "pack" || a.PlanOf(ac).Projects != 3 || a.PlanOf(ac).Branding {
		t.Fatalf("plan: %+v", a.PlanOf(ac))
	}
	if _, err := a.CreateCampaign(ctx, 9999, 9900, "order_2"); err != ErrNotFound {
		t.Fatalf("unknown account: %v", err)
	}
	if _, err := a.CreateCampaign(ctx, id, -1, ""); err == nil {
		t.Fatal("negative price accepted")
	}

	// A grandfathered subscriber keeps their plan, and falls back to pack when it ends.
	bob, _ := a.SignUp(ctx, "bob@example.com", "correct horse battery")
	a.db.W.Exec(`UPDATE admins SET plan = 'pro', sub_status = 'active', polar_subscription_id = 'sub_b' WHERE id = ?`, bob)
	a.CreateCampaign(ctx, bob, 9900, "order_3")
	if ac, _ := a.Account(ctx, bob); ac.Plan != "pro" {
		t.Fatalf("subscriber plan changed: %s", ac.Plan)
	}
	ext := externalID(bob)
	sub := &polarSubscription{ID: "sub_b", Status: "canceled", ProductID: "prod-pro-y"}
	sub.Customer.ExternalID = &ext
	if err := a.ApplySubscription(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if ac, _ := a.Account(ctx, bob); ac.Plan != "pack" {
		t.Fatalf("ended subscription should fall back to pack: %s", ac.Plan)
	}
}

func TestCampaignTaskLimit(t *testing.T) {
	e := newPackEnv(t)
	e.toPlanning(t)
	e.addDestinations(t, maxCampaignTasks+1)
	for i, d := range e.destinations {
		want := 201
		if i == maxCampaignTasks {
			want = 409
		}
		mustCode(t, e.ops, "POST", e.opsCampaign+"/tasks", fmt.Sprintf(`{"destination_id":%d}`, d), want)
	}
	mustCode(t, e.ops, "POST", e.opsCampaign+"/tasks", `{"destination_id":99999}`, 400)
}

func TestProfileVersions(t *testing.T) {
	e := newPackEnv(t)
	if out := mustCode(t, e.alice, "GET", "/api/admin/projects/orbit/profile", "", 200); out["latest"] != nil || len(out["missing"].([]any)) != 15 {
		t.Fatalf("empty profile: %v", out)
	}
	for _, bad := range []string{
		`{"tagline":"` + strings.Repeat("x", 61) + `"}`,
		`{"stage":"unicorn"}`,
		`{"url":"javascript:alert(1)"}`,
		`{"founder_email":"not an email"}`,
		`{"founder_links":"https://ok.example\nftp://bad"}`,
	} {
		mustCode(t, e.alice, "POST", "/api/admin/projects/orbit/profile", bad, 400)
	}
	mustCode(t, e.alice, "POST", "/api/admin/projects/orbit/profile", `{"name":" Orbit ","url":"https://orbit.example"}`, 201)
	mustCode(t, e.alice, "POST", "/api/admin/projects/orbit/profile", `{"name":"Orbit 2"}`, 201)
	var n int
	e.a.db.W.QueryRow(`SELECT COUNT(*) FROM profile_versions`).Scan(&n)
	if n != 2 {
		t.Fatalf("versions: %d", n)
	}
	out := mustCode(t, e.alice, "GET", "/api/admin/projects/orbit/profile", "", 200)
	latest := out["latest"].(map[string]any)["profile"].(map[string]any)
	if latest["name"] != "Orbit 2" || latest["url"] != "" || out["approved"] != nil || len(out["missing"].([]any)) != 14 {
		t.Fatalf("latest: %v", out)
	}
}

func pngBytes(n int) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, n)...)
}

func uploadAsset(t *testing.T, e *packEnv, c *client, slug, kind, name string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("kind", kind)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	req := httptest.NewRequest("POST", "/api/admin/projects/"+slug+"/assets", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(c.cookie)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestAssetLimits(t *testing.T) {
	e := newPackEnv(t)
	// Bob owns a waitlist but has no Launch Pack: no uploads.
	if rec := uploadAsset(t, e, e.bob, "bobs", "logo", "x.png", pngBytes(10)); rec.Code != 403 {
		t.Fatalf("upload without a campaign: %d %s", rec.Code, rec.Body)
	}
	if rec := uploadAsset(t, e, e.alice, "orbit", "logo", "x.png", []byte("<html><script>alert(1)</script>")); rec.Code != 400 {
		t.Fatalf("html accepted: %d", rec.Code)
	}
	if rec := uploadAsset(t, e, e.alice, "orbit", "wallpaper", "x.png", pngBytes(10)); rec.Code != 400 {
		t.Fatalf("bad kind accepted: %d", rec.Code)
	}
	if rec := uploadAsset(t, e, e.alice, "orbit", "logo", "big.png", pngBytes(maxAssetBytes)); rec.Code != 413 {
		t.Fatalf("oversize: %d %s", rec.Code, rec.Body)
	}
	rec := uploadAsset(t, e, e.alice, "orbit", "logo", `../../"evil".png`, pngBytes(10))
	var as Asset
	json.Unmarshal(rec.Body.Bytes(), &as)
	if rec.Code != 201 || as.Mime != "image/png" || as.Filename != "evil.png" {
		t.Fatalf("upload: %d %+v", rec.Code, as)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/admin/projects/orbit/assets/%d", as.ID), nil)
	req.AddCookie(e.alice.cookie)
	got := httptest.NewRecorder()
	e.h.ServeHTTP(got, req)
	hd := got.Header()
	if got.Code != 200 || hd.Get("Content-Type") != "image/png" || hd.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(hd.Get("Content-Security-Policy"), "sandbox") || !strings.Contains(hd.Get("Content-Disposition"), "evil.png") ||
		!bytes.Equal(got.Body.Bytes(), pngBytes(10)) {
		t.Fatalf("serve: %d %v", got.Code, hd)
	}

	for i := 1; i < maxAssetsPerProject; i++ {
		if rec := uploadAsset(t, e, e.alice, "orbit", "screenshot", "s.png", pngBytes(10)); rec.Code != 201 {
			t.Fatalf("upload %d: %d", i, rec.Code)
		}
	}
	if rec := uploadAsset(t, e, e.alice, "orbit", "screenshot", "s.png", pngBytes(10)); rec.Code != 400 {
		t.Fatalf("21st upload: %d", rec.Code)
	}
	if out := mustCode(t, e.alice, "GET", "/api/admin/projects/orbit/assets", "", 200); len(out["data"].([]any)) != maxAssetsPerProject {
		t.Fatalf("list: %v", out)
	}
	mustCode(t, e.alice, "DELETE", fmt.Sprintf("/api/admin/projects/orbit/assets/%d", as.ID), "", 200)
	mustCode(t, e.alice, "DELETE", fmt.Sprintf("/api/admin/projects/orbit/assets/%d", as.ID), "", 404)
}
