package app

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

func TestOpsPage(t *testing.T) {
	a := newCloudApp(t)
	a.cfg.Operators = []string{"OPS@example.com"}
	ctx := context.Background()
	srv, _ := NewServer(a)
	h := srv.Handler()
	ops, alice := &client{t: t, h: h}, &client{t: t, h: h}
	ops.do("POST", "/api/admin/signup", `{"email":"ops@example.com","password":"correct horse battery"}`)
	alice.do("POST", "/api/admin/signup", `{"email":"alice@example.com","password":"correct horse battery"}`)
	bobID, _ := a.SignUp(ctx, "bob@example.com", "correct horse battery")

	// An operator email isn't an operator until it's verified.
	if code, _ := ops.do("GET", "/api/admin/ops", ""); code != 404 {
		t.Fatalf("unverified operator: %d", code)
	}
	a.db.W.Exec(`UPDATE admins SET verified_at = 1, created_at = 1`)
	if code, _ := alice.do("GET", "/api/admin/ops", ""); code != 404 {
		t.Fatalf("non-operator must get 404: %d", code)
	}
	if code, _ := alice.do("POST", "/api/admin/ops/accounts/"+strconv.FormatInt(bobID, 10)+"/suspend", ""); code != 404 {
		t.Fatalf("non-operator suspend must get 404: %d", code)
	}
	if _, me := alice.do("GET", "/api/admin/me", ""); me["operator"] != nil {
		t.Fatalf("alice is no operator: %v", me)
	}
	if _, me := ops.do("GET", "/api/admin/me", ""); me["operator"] != true {
		t.Fatalf("me should flag the operator: %v", me)
	}

	// One launch monthly ($5) and one pro yearly ($10/mo); a canceled one doesn't count.
	a.db.W.Exec(`UPDATE admins SET plan = 'launch', plan_interval = 'month', sub_status = 'active', polar_subscription_id = 's1' WHERE email = 'alice@example.com'`)
	a.db.W.Exec(`UPDATE admins SET plan = 'pro', plan_interval = 'year', sub_status = 'active', polar_subscription_id = 's2' WHERE id = ?`, bobID)
	a.db.W.Exec(`UPDATE admins SET plan = 'free', sub_status = 'canceled', polar_subscription_id = 's3' WHERE email = 'ops@example.com'`)
	p, _, err := a.CreateProject(ctx, bobID, "Orbit", "orbit")
	if err != nil {
		t.Fatal(err)
	}
	signup(t, a, p, "fan@example.com", "", "1.1.1.1")

	code, out := ops.do("GET", "/api/admin/ops", "")
	if code != 200 {
		t.Fatalf("ops: %d %v", code, out)
	}
	tot := out["totals"].(map[string]any)
	paying := tot["paying"].(map[string]any)
	if tot["mrr"] != 15.0 || tot["accounts"] != 3.0 || tot["verified"] != 3.0 || tot["signups"] != 1.0 || tot["signups_7d"] != 1.0 ||
		paying["launch"] != 1.0 || paying["pro"] != 1.0 || len(paying) != 2 || tot["truncated"] != false {
		t.Fatalf("totals: %v", tot)
	}
	list := out["accounts"].([]any)
	if len(list) != 3 {
		t.Fatalf("accounts: %v", list)
	}
	for _, x := range list {
		if row := x.(map[string]any); row["email"] == "bob@example.com" && (row["projects"] != 1.0 || row["signups"] != 1.0) {
			t.Fatalf("bob's row: %v", row)
		}
	}

	// Suspending blocks the account's broadcast; unsuspending lifts it.
	bob := "/api/admin/ops/accounts/" + strconv.FormatInt(bobID, 10)
	if code, out := ops.do("POST", bob+"/suspend", ""); code != 200 {
		t.Fatalf("suspend: %d %v", code, out)
	}
	if _, err := a.Broadcast(ctx, p, "Hi", "Body", "confirmed"); !errors.Is(err, errSuspended) {
		t.Fatalf("suspended broadcast: %v", err)
	}
	ops.do("POST", bob+"/unsuspend", "")
	if ac, _ := a.Account(ctx, bobID); ac.Suspended {
		t.Fatal("unsuspend should clear it")
	}

	// No suspending yourself.
	var opsID int64
	a.db.W.QueryRow(`SELECT id FROM admins WHERE email = 'ops@example.com'`).Scan(&opsID)
	if code, _ := ops.do("POST", "/api/admin/ops/accounts/"+strconv.FormatInt(opsID, 10)+"/suspend", ""); code != 400 {
		t.Fatalf("self-suspend: %d", code)
	}
	if ac, _ := a.Account(ctx, opsID); ac.Suspended {
		t.Fatal("operator suspended themself")
	}
}

func TestOpsSelfHosted(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Operators = []string{"admin@example.com"}
	ctx := context.Background()
	if err := a.CreateFirstAdmin(ctx, "admin@example.com", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	srv, _ := NewServer(a)
	c := &client{t: t, h: srv.Handler()}
	c.do("POST", "/api/admin/login", `{"email":"admin@example.com","password":"correct horse battery"}`)
	if code, _ := c.do("GET", "/api/admin/ops", ""); code != 404 {
		t.Fatalf("self-hosted ops must 404: %d", code)
	}
	if _, me := c.do("GET", "/api/admin/me", ""); me["operator"] != nil || me["email"] != "admin@example.com" {
		t.Fatalf("self-hosted me: %v", me)
	}
}
