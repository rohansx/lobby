package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	cfg := Config{BaseURL: "http://lobby.test", DataDir: t.TempDir(), CheckMX: false}
	a, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

func newTestProject(t *testing.T, a *App, doi bool) *Project {
	t.Helper()
	p, _, err := a.CreateProject(context.Background(), "Convox", "convox-"+randCode(4))
	if err != nil {
		t.Fatal(err)
	}
	p.DOIEnabled = doi
	if err := a.UpdateProject(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	p, _ = a.ProjectByID(context.Background(), p.ID)
	return p
}

func signup(t *testing.T, a *App, p *Project, email, ref, ip string) *SignupResult {
	t.Helper()
	res, err := a.SubmitSignup(context.Background(), p, SignupInput{Email: email, Ref: ref, IP: ip, DwellMs: -1})
	if err != nil {
		t.Fatalf("signup %s: %v", email, err)
	}
	return res
}

func TestDedupeOnNormalizedEmail(t *testing.T) {
	a := newTestApp(t)
	p := newTestProject(t, a, true)
	first := signup(t, a, p, "Jane.Doe+waitlist@gmail.com", "", "1.1.1.1")
	if !first.Created {
		t.Fatal("first signup should be created")
	}
	again := signup(t, a, p, "janedoe@googlemail.com", "", "1.1.1.1")
	if again.Created || again.Signup.ID != first.Signup.ID {
		t.Fatalf("expected the existing signup back, got created=%v", again.Created)
	}
}

func TestSpamChecks(t *testing.T) {
	a := newTestApp(t)
	p := newTestProject(t, a, true)
	ctx := context.Background()
	res, err := a.SubmitSignup(ctx, p, SignupInput{Email: "bot@example.com", Honeypot: "http://spam", DwellMs: -1})
	if err != nil || !res.Fake {
		t.Fatalf("honeypot should fake success, got %+v %v", res, err)
	}
	res, _ = a.SubmitSignup(ctx, p, SignupInput{Email: "fast@example.com", DwellMs: 200})
	if !res.Fake {
		t.Fatal("sub-second submit should fake success")
	}
	_, err = a.SubmitSignup(ctx, p, SignupInput{Email: "x@mailinator.com", DwellMs: -1})
	var ve *ValidationError
	if err == nil || !errorsAs(err, &ve) {
		t.Fatalf("disposable domain should be rejected, got %v", err)
	}
	_, err = a.SubmitSignup(ctx, p, SignupInput{Email: "not an email", DwellMs: -1})
	if err == nil {
		t.Fatal("invalid email should be rejected")
	}
	var n int
	a.db.W.QueryRow(`SELECT COUNT(*) FROM signups`).Scan(&n)
	if n != 0 {
		t.Fatalf("no spam should be stored, found %d rows", n)
	}
}

func TestReferralCreditOnlyAfterConfirm(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	p := newTestProject(t, a, true)

	alice := signup(t, a, p, "alice@example.com", "", "10.0.0.1").Signup
	a.Confirm(ctx, p, alice.ID)
	bob := signup(t, a, p, "bob@example.com", alice.ReferralCode, "10.0.0.2").Signup

	if got, _ := a.Signup(ctx, p.ID, alice.ID); got.Points != 0 {
		t.Fatalf("referrer credited before the friend confirmed: %d points", got.Points)
	}
	a.Confirm(ctx, p, bob.ID)
	if got, _ := a.Signup(ctx, p.ID, alice.ID); got.Points != int64(p.PointsPerReferral) {
		t.Fatalf("referrer should have %d points after confirm, has %d", p.PointsPerReferral, got.Points)
	}
	// Confirming twice must not credit twice.
	a.Confirm(ctx, p, bob.ID)
	if got, _ := a.Signup(ctx, p.ID, alice.ID); got.Points != int64(p.PointsPerReferral) {
		t.Fatalf("double credit: %d", got.Points)
	}
}

func TestReferralIPCap(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	p := newTestProject(t, a, false)
	alice := signup(t, a, p, "alice@example.com", "", "10.0.0.1").Signup
	for i := 0; i < 5; i++ {
		signup(t, a, p, "friend"+string(rune('a'+i))+"@example.com", alice.ReferralCode, "66.66.66.66")
	}
	got, _ := a.Signup(ctx, p.ID, alice.ID)
	if want := int64(p.ReferralIPCap * p.PointsPerReferral); got.Points != want {
		t.Fatalf("IP cap: want %d points, got %d", want, got.Points)
	}
	pl, _ := a.PlaceOf(ctx, p, got)
	if pl.Referrals != int64(p.ReferralIPCap) {
		t.Fatalf("want %d credited referrals, got %d", p.ReferralIPCap, pl.Referrals)
	}
}

func TestPositionMovesWithReferrals(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	p := newTestProject(t, a, false)
	var people []*Signup
	for i := 0; i < 20; i++ {
		people = append(people, signup(t, a, p, "p"+string(rune('a'+i))+"@example.com", "", "10.0.0.1").Signup)
	}
	last := people[19]
	pl, _ := a.PlaceOf(ctx, p, last)
	if pl.Position != 20 || pl.Behind != 0 {
		t.Fatalf("last in line: got %+v", pl)
	}
	signup(t, a, p, "friend@example.com", last.ReferralCode, "10.9.9.9")
	last, _ = a.Signup(ctx, p.ID, last.ID)
	pl, _ = a.PlaceOf(ctx, p, last)
	if pl.Position != 10 {
		t.Fatalf("one referral at 10 points should move #20 to #10, got #%d", pl.Position)
	}
	// Invite the first 5: they leave the line, everyone else moves up.
	invited, err := a.InviteNext(ctx, p, 5)
	if err != nil || len(invited) != 5 {
		t.Fatalf("invite next 5: %v %d", err, len(invited))
	}
	pl, _ = a.PlaceOf(ctx, p, last)
	if pl.Position != 5 {
		t.Fatalf("after 5 invites expected #5, got #%d", pl.Position)
	}
}

func TestInviteRedeem(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	p := newTestProject(t, a, false)
	s := signup(t, a, p, "carol@example.com", "", "10.0.0.1").Signup
	if _, err := a.InviteIDs(ctx, p, []int64{s.ID}); err != nil {
		t.Fatal(err)
	}
	inv, err := a.LatestInvite(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Redeem(ctx, p, "", "someone-else@example.com", inv.Code); err != ErrInvalidInvite {
		t.Fatalf("code must be bound to the email, got %v", err)
	}
	got, err := a.Redeem(ctx, p, a.inviteToken(inv.ID), "", "")
	if err != nil || got.Status != StatusJoined {
		t.Fatalf("redeem by token: %v %+v", err, got)
	}
	if _, err := a.Redeem(ctx, p, a.inviteToken(inv.ID), "", ""); err != ErrInvalidInvite {
		t.Fatal("invite must be single use")
	}
}

func TestWebhookSignature(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	body := []byte(`{"test": 2432232314}`)
	// Reference vector from the Standard Webhooks spec.
	got := SignWebhook(secret, "msg_p5jXN8AQM9LWM0D4loKWxJek", "1614265330", body)
	if got != "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=" {
		t.Fatalf("signature mismatch: %s", got)
	}
}

func TestEventsAreQueuedWithChange(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	p := newTestProject(t, a, true)
	if _, err := a.CreateWebhook(ctx, p, "https://example.com/hook", "standard", nil); err != nil {
		t.Fatal(err)
	}
	s := signup(t, a, p, "dave@example.com", "", "10.0.0.1").Signup
	a.Confirm(ctx, p, s.ID)
	rows, _ := a.db.R.Query(`SELECT event FROM deliveries ORDER BY id`)
	var events []string
	for rows.Next() {
		var e string
		rows.Scan(&e)
		events = append(events, e)
	}
	rows.Close()
	if strings.Join(events, ",") != "signup.created,signup.confirmed" {
		t.Fatalf("unexpected events %v", events)
	}
}

func TestHTTPSignupDoesNotLeakMembership(t *testing.T) {
	a := newTestApp(t)
	p := newTestProject(t, a, true)
	srv, err := NewServer(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	post := func(email string) (int, map[string]any) {
		body := `{"email":"` + email + `"}`
		req := httptest.NewRequest("POST", "/v1/w/"+p.PublicKey+"/signup", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.9:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	c1, new1 := post("erin@example.com")
	c2, again := post("erin@example.com")
	if c1 != 200 || c2 != 200 || new1["message"] != again["message"] || len(new1) != len(again) {
		t.Fatalf("new and repeat signups must look identical: %v %v", new1, again)
	}
	// Plain form posts redirect.
	form := url.Values{"email": {"frank@example.com"}}
	req := httptest.NewRequest("POST", "/v1/w/"+p.PublicKey+"/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "/w/"+p.Slug+"/thanks") {
		t.Fatalf("form post: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	// An off-list redirect target is ignored.
	form.Set("email", "gina@example.com")
	form.Set("_redirect", "https://evil.example/phish")
	req = httptest.NewRequest("POST", "/v1/w/"+p.PublicKey+"/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if strings.Contains(rec.Header().Get("Location"), "evil") {
		t.Fatal("open redirect")
	}
}

func errorsAs(err error, target **ValidationError) bool {
	ve, ok := err.(*ValidationError)
	if ok {
		*target = ve
	}
	return ok
}

func TestAdminAPI(t *testing.T) {
	a := newTestApp(t)
	srv, err := NewServer(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	var cookie *http.Cookie
	do := func(method, path, body string, hdr ...string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Name == sessionCookie {
				cookie = c
			}
		}
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	if code, me := do("GET", "/api/admin/me", ""); code != 200 || me["setup"] != true {
		t.Fatalf("fresh install should ask for setup: %d %v", code, me)
	}
	if code, _ := do("GET", "/api/admin/projects", ""); code != 401 {
		t.Fatalf("projects without a session: %d", code)
	}
	if code, out := do("POST", "/api/admin/setup", `{"email":"admin@example.com","password":"correct horse battery"}`); code != 200 || cookie == nil {
		t.Fatalf("setup: %d %v", code, out)
	}
	if code, _ := do("POST", "/api/admin/setup", `{"email":"second@example.com","password":"correct horse battery"}`); code != 400 {
		t.Fatalf("second setup must fail: %d", code)
	}
	if code, out := do("POST", "/api/admin/projects", `{"name":"Convox","slug":"convox"}`); code != 201 || !strings.HasPrefix(out["secret_key"].(string), "sk_") {
		t.Fatalf("create project: %d %v", code, out)
	}
	if code, out := do("PATCH", "/api/admin/projects/convox", `{"headline":"Be first","slug":"hijack","public_key":"pk_x"}`); code != 200 || out["headline"] != "Be first" || out["slug"] != "convox" || out["name"] != "Convox" {
		t.Fatalf("patch should change headline only: %d %v", code, out)
	}
	if code, _ := do("POST", "/api/admin/projects", `{"name":"Evil"}`, "Sec-Fetch-Site", "cross-site"); code != 403 {
		t.Fatalf("cross-site write must be refused: %d", code)
	}
	if code, _ := do("DELETE", "/api/admin/projects/convox", ""); code != 400 {
		t.Fatalf("delete without confirmation: %d", code)
	}

	// Client-side routes fall back to the app shell; the API never does.
	req := httptest.NewRequest("GET", "/p/convox/signups", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Lobby</title>") {
		t.Fatalf("spa fallback: %d", rec.Code)
	}
	if code, _ := do("GET", "/api/admin/nope", ""); code != 404 {
		t.Fatalf("unknown api route: %d", code)
	}
	do("POST", "/api/admin/logout", "")
	if code, _ := do("GET", "/api/admin/projects", ""); code != 401 {
		t.Fatalf("after logout: %d", code)
	}
}

type captureMailer struct{ sent []*Message }

func (c *captureMailer) Name() string { return "capture" }
func (c *captureMailer) Send(_ context.Context, m *Message) error {
	c.sent = append(c.sent, m)
	return nil
}

func TestOwnerNotification(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if err := a.CreateFirstAdmin(ctx, "owner@example.com", "a-long-password-1"); err != nil {
		t.Fatal(err)
	}
	p := newTestProject(t, a, true)
	if p.NotifyEmail != "owner@example.com" {
		t.Fatalf("new projects should notify the admin, got %q", p.NotifyEmail)
	}
	mc := &captureMailer{}
	a.mailer = mc
	referrer := signup(t, a, p, "first@example.com", "", "1.1.1.1")
	res, err := a.SubmitSignup(ctx, p, SignupInput{Email: "second@example.com", Ref: referrer.Signup.ReferralCode, IP: "2.2.2.2", DwellMs: -1,
		SourceURL: "https://convox.ai/", UTMSource: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.runJobs(ctx); err != nil {
		t.Fatal(err)
	}
	var notes []*Message
	for _, m := range mc.sent {
		if m.To == "owner@example.com" {
			notes = append(notes, m)
		}
	}
	if len(notes) != 2 || len(mc.sent) != 4 {
		t.Fatalf("want 2 confirmations and 2 owner notes, got %d sent, %d notes", len(mc.sent), len(notes))
	}
	n := notes[1]
	for _, want := range []string{"second@example.com", "#2 of 2", "first@example.com", "https://convox.ai/", "source=x", "/p/" + p.Slug + "/signups"} {
		if !strings.Contains(n.Text, want) || !strings.Contains(n.HTML, want) {
			t.Errorf("owner note is missing %q:\n%s", want, n.Text)
		}
	}
	if n.ReplyTo != res.Signup.Email || !strings.Contains(n.Subject, "Convox") {
		t.Errorf("reply-to %q subject %q", n.ReplyTo, n.Subject)
	}

	// Turning it off stops the notes; a bad address is rejected.
	p.NotifyEmail = "nope"
	if err := a.UpdateProject(ctx, p); err == nil {
		t.Fatal("invalid notify address should be rejected")
	}
	p.NotifyEmail = ""
	if err := a.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	mc.sent = nil
	signup(t, a, p, "third@example.com", "", "3.3.3.3")
	a.runJobs(ctx)
	if len(mc.sent) != 1 {
		t.Fatalf("with notifications off only the confirmation should go out, got %d", len(mc.sent))
	}
}
