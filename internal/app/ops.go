package app

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

// The operator page: instance-wide numbers and account suspension for whoever
// runs a Lobby Cloud instance. Operators are the verified accounts whose email
// is in LOBBY_OPERATORS; everyone else gets a 404, so the page isn't advertised.

const opsAccountLimit = 500

func (a *App) isOperator(ac *Account) bool {
	if !a.cfg.Cloud || ac == nil || !ac.Verified {
		return false
	}
	for _, e := range a.cfg.Operators {
		if strings.EqualFold(e, ac.Email) {
			return true
		}
	}
	return false
}

func (s *Server) operatorOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ac, err := s.app.Account(r.Context(), adminID(r)); err != nil || !s.app.isOperator(ac) {
			jsonError(w, http.StatusNotFound, "not found")
			return
		}
		next(w, r)
	}
}

type OpsTotals struct {
	Accounts      int64            `json:"accounts"`
	Verified      int64            `json:"verified"`
	Paying        map[string]int64 `json:"paying"` // by plan id
	MRR           int64            `json:"mrr"`    // USD estimate
	Signups       int64            `json:"signups"`
	Signups7d     int64            `json:"signups_7d"`
	EmailsMonth   int64            `json:"emails_month"`
	QueuedJobs    int              `json:"queued_jobs"`
	FailedJobs    int              `json:"failed_jobs"`
	AccountsShown int              `json:"accounts_shown"`
	Truncated     bool             `json:"truncated"`
}

type OpsAccount struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	Verified    bool   `json:"verified"`
	Plan        string `json:"plan"`
	Interval    string `json:"interval"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
	Projects    int64  `json:"projects"`
	Signups     int64  `json:"signups"`
	EmailsMonth int64  `json:"emails_month"`
	Suspended   bool   `json:"suspended"`
}

// OpsOverview returns instance totals and the newest accounts.
func (a *App) OpsOverview(ctx context.Context) (*OpsTotals, []OpsAccount, error) {
	t := &OpsTotals{Paying: map[string]int64{}}
	m := month()
	err := a.db.R.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM admins),
		(SELECT COUNT(*) FROM admins WHERE verified_at IS NOT NULL),
		(SELECT COUNT(*) FROM signups WHERE status != ?),
		(SELECT COUNT(*) FROM signups WHERE status != ? AND created_at > ?),
		(SELECT COALESCE(SUM(sent), 0) FROM email_usage WHERE month = ?)`,
		StatusRemoved, StatusRemoved, now()-7*86400, m).Scan(&t.Accounts, &t.Verified, &t.Signups, &t.Signups7d, &t.EmailsMonth)
	if err != nil {
		return nil, nil, err
	}

	rows, err := a.db.R.QueryContext(ctx, `SELECT plan, plan_interval, sub_status, COUNT(*) FROM admins
		WHERE polar_subscription_id != '' GROUP BY 1, 2, 3`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var plan, interval, status string
		var n int64
		if err := rows.Scan(&plan, &interval, &status, &n); err != nil {
			rows.Close()
			return nil, nil, err
		}
		p, ok := plans[plan]
		if !ok || !paidStatus(status) || p.Monthly == 0 {
			continue
		}
		t.Paying[plan] += n
		if interval == "year" {
			t.MRR += n * int64(p.YearlyPer)
		} else {
			t.MRR += n * int64(p.Monthly)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	t.QueuedJobs, t.FailedJobs = a.QueueStats(ctx, 0)

	rows, err = a.db.R.QueryContext(ctx, `SELECT a.id, a.email, a.verified_at IS NOT NULL, a.plan, a.plan_interval, a.sub_status,
		a.created_at, a.suspended_at IS NOT NULL,
		(SELECT COUNT(*) FROM projects WHERE owner_id = a.id),
		(SELECT COUNT(*) FROM signups s JOIN projects p ON p.id = s.project_id WHERE p.owner_id = a.id AND s.status != ?),
		COALESCE((SELECT sent FROM email_usage WHERE admin_id = a.id AND month = ?), 0)
		FROM admins a ORDER BY a.created_at DESC, a.id DESC LIMIT ?`, StatusRemoved, m, opsAccountLimit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	list := []OpsAccount{}
	for rows.Next() {
		var o OpsAccount
		if err := rows.Scan(&o.ID, &o.Email, &o.Verified, &o.Plan, &o.Interval, &o.Status, &o.CreatedAt, &o.Suspended,
			&o.Projects, &o.Signups, &o.EmailsMonth); err != nil {
			return nil, nil, err
		}
		list = append(list, o)
	}
	if len(list) > opsAccountLimit {
		list, t.Truncated = list[:opsAccountLimit], true
	}
	t.AccountsShown = len(list)
	return t, list, rows.Err()
}

func (s *Server) adminOps(w http.ResponseWriter, r *http.Request) {
	totals, accounts, err := s.app.OpsOverview(r.Context())
	if err != nil {
		s.app.log.Error("ops overview", "err", err)
		jsonError(w, 500, "Couldn't load the numbers.")
		return
	}
	writeJSON(w, 200, map[string]any{"totals": totals, "accounts": accounts})
}

func (s *Server) adminOpsAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, 404, "account not found")
		return
	}
	if _, err := s.app.Account(r.Context(), id); err != nil {
		jsonError(w, 404, "account not found")
		return
	}
	switch r.PathValue("action") {
	case "suspend":
		if id == adminID(r) {
			jsonError(w, 400, "You can't suspend your own account.")
			return
		}
		err = s.app.SuspendAccount(r.Context(), id)
	case "unsuspend":
		err = s.app.UnsuspendAccount(r.Context(), id)
	default:
		jsonError(w, 404, "not found")
		return
	}
	if err != nil {
		s.app.log.Error("ops action", "err", err)
		jsonError(w, 500, "Couldn't update the account.")
		return
	}
	s.app.log.Info("ops: account updated", "operator", adminID(r), "account", id, "action", r.PathValue("action"))
	writeJSON(w, 200, map[string]any{"ok": true})
}
