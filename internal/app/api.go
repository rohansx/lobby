package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ctxKey int

const projectKey ctxKey = 0

func (s *Server) requireKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(tok, "sk_") {
			jsonError(w, http.StatusUnauthorized, "send the project secret key as Authorization: Bearer sk_...")
			return
		}
		p, err := s.app.ProjectBySecret(r.Context(), tok)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid secret key")
			return
		}
		if plan, _ := s.app.planForOwner(r.Context(), p.OwnerID); !plan.Webhooks {
			jsonError(w, http.StatusPaymentRequired, "the REST API comes with the Launch plan")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), projectKey, p)))
	})
}

func apiProject(r *http.Request) *Project { return r.Context().Value(projectKey).(*Project) }

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func decodeJSON(r *http.Request, v any) error {
	if r.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func (s *Server) apiProject(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, apiProject(r))
}

func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.Stats(r.Context(), apiProject(r))
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

type signupOut struct {
	*Signup
	Position     any    `json:"position"`
	StatusURL    string `json:"status_url"`
	ReferralLink string `json:"referral_link"`
}

func (s *Server) signupOut(r *http.Request, p *Project, su *Signup, withPosition bool) signupOut {
	o := signupOut{Signup: su, StatusURL: s.app.cfg.BaseURL + "/s/" + s.app.StatusToken(su), ReferralLink: s.app.ReferralLink(p, su)}
	if withPosition && su.Status == StatusConfirmed {
		if pl, err := s.app.PlaceOf(r.Context(), p, su); err == nil {
			o.Position = pl.Position
		}
	}
	return o
}

func (s *Server) apiListSignups(w http.ResponseWriter, r *http.Request) {
	p := apiProject(r)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	cursor, _ := strconv.ParseInt(q.Get("cursor"), 10, 64)
	list, err := s.app.ListSignups(r.Context(), p, SignupFilter{Status: q.Get("status"), Query: q.Get("q"), Before: cursor, Limit: limit})
	if err != nil {
		limitError(w, err)
		return
	}
	out := make([]signupOut, 0, len(list))
	for _, su := range list {
		out = append(out, s.signupOut(r, p, su, false))
	}
	var next any
	if len(list) > 0 && (limit <= 0 && len(list) == 50 || len(list) == limit) {
		next = strconv.FormatInt(list[len(list)-1].ID, 10)
	}
	writeJSON(w, 200, map[string]any{"data": out, "next_cursor": next})
}

func (s *Server) apiGetSignup(w http.ResponseWriter, r *http.Request) {
	p := apiProject(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, 400, "bad id")
		return
	}
	su, err := s.app.Signup(r.Context(), p.ID, id)
	if err != nil {
		jsonError(w, 404, "not found")
		return
	}
	writeJSON(w, 200, s.signupOut(r, p, su, true))
}

func (s *Server) apiRemoveSignup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		jsonError(w, 400, "bad id")
		return
	}
	err := s.app.Remove(r.Context(), apiProject(r), id, r.URL.Query().Get("hard") == "1")
	if errors.Is(err, ErrNotFound) {
		jsonError(w, 404, "not found")
		return
	} else if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) apiJoined(w http.ResponseWriter, r *http.Request) {
	p := apiProject(r)
	id, ok := pathID(r)
	if !ok {
		jsonError(w, 400, "bad id")
		return
	}
	su, err := s.app.MarkJoined(r.Context(), p, id)
	if errors.Is(err, ErrNotFound) {
		jsonError(w, 404, "not found")
		return
	} else if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.signupOut(r, p, su, false))
}

func (s *Server) apiInvite(w http.ResponseWriter, r *http.Request) {
	p := apiProject(r)
	var body struct {
		Count int     `json:"count"`
		IDs   []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	var list []*Signup
	var err error
	switch {
	case len(body.IDs) > 0:
		list, err = s.app.InviteIDs(r.Context(), p, body.IDs)
	case body.Count > 0:
		list, err = s.app.InviteNext(r.Context(), p, body.Count)
	default:
		jsonError(w, 400, `send {"count": N} or {"ids": [...]}`)
		return
	}
	if err != nil {
		limitError(w, err)
		return
	}
	out := make([]signupOut, 0, len(list))
	for _, su := range list {
		out = append(out, s.signupOut(r, p, su, false))
	}
	writeJSON(w, 200, map[string]any{"invited": len(out), "data": out})
}

func (s *Server) apiRedeem(w http.ResponseWriter, r *http.Request) {
	p := apiProject(r)
	var body struct {
		Token string `json:"token"`
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if body.Token == "" && (body.Email == "" || body.Code == "") {
		jsonError(w, 400, `send {"token": "..."} or {"email": "...", "code": "..."}`)
		return
	}
	su, err := s.app.Redeem(r.Context(), p, body.Token, body.Email, body.Code)
	if errors.Is(err, ErrInvalidInvite) {
		jsonError(w, 404, err.Error())
		return
	} else if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.signupOut(r, p, su, false))
}

var csvHeader = []string{"id", "email", "status", "base_position", "points", "referral_code", "referred_by", "referrals_credited",
	"source_url", "referrer", "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "consent_version",
	"created_at", "confirmed_at", "invited_at", "joined_at", "unsubscribed_at"}

func isoTime(t *int64) string {
	if t == nil {
		return ""
	}
	return time.Unix(*t, 0).UTC().Format(time.RFC3339)
}

// csvSafe neutralises spreadsheet formula injection from visitor-supplied fields.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func (s *Server) creditedCounts(r *http.Request, p *Project) map[int64]int64 {
	out := map[int64]int64{}
	rows, err := s.app.db.R.QueryContext(r.Context(), `SELECT referred_by, COUNT(*) FROM signups
		WHERE project_id = ? AND referral_credited = 1 GROUP BY referred_by`, p.ID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, n int64
		rows.Scan(&id, &n)
		out[id] = n
	}
	return out
}

// Exports default to confirmed-and-beyond so unconfirmed addresses don't leak
// into a newsletter tool; ?status=all includes everyone.
func exportStatus(r *http.Request) []string {
	switch st := r.URL.Query().Get("status"); st {
	case "all":
		return []string{""}
	case "":
		return []string{StatusConfirmed, StatusInvited, StatusJoined}
	default:
		return []string{st}
	}
}

func (s *Server) apiExportCSV(w http.ResponseWriter, r *http.Request) {
	s.export(w, r, apiProject(r), "csv")
}

func (s *Server) apiExportNDJSON(w http.ResponseWriter, r *http.Request) {
	s.export(w, r, apiProject(r), "ndjson")
}

func (s *Server) export(w http.ResponseWriter, r *http.Request, p *Project, format string) {
	statuses := exportStatus(r)
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+p.Slug+`-waitlist.csv"`)
		cw := csv.NewWriter(w)
		cw.Write(csvHeader)
		cw.Flush()
		for _, st := range statuses {
			s.writeCSVRows(w, r, p, st)
		}
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", `attachment; filename="`+p.Slug+`-waitlist.ndjson"`)
	enc := json.NewEncoder(w)
	for _, st := range statuses {
		s.app.EachSignup(r.Context(), p, st, func(su *Signup) error { return enc.Encode(su) })
	}
}

func (s *Server) writeCSVRows(w http.ResponseWriter, r *http.Request, p *Project, status string) {
	cw := csv.NewWriter(w)
	refs := s.creditedCounts(r, p)
	s.app.EachSignup(r.Context(), p, status, func(su *Signup) error {
		refBy := ""
		if su.ReferredBy != nil {
			refBy = strconv.FormatInt(*su.ReferredBy, 10)
		}
		created := su.CreatedAt
		return cw.Write([]string{strconv.FormatInt(su.ID, 10), csvSafe(su.Email), su.Status, strconv.FormatInt(su.BasePosition, 10),
			strconv.FormatInt(su.Points, 10), su.ReferralCode, refBy, strconv.FormatInt(refs[su.ID], 10), csvSafe(su.SourceURL),
			csvSafe(su.Referrer), csvSafe(su.UTMSource), csvSafe(su.UTMMedium), csvSafe(su.UTMCampaign), csvSafe(su.UTMTerm),
			csvSafe(su.UTMContent), strconv.Itoa(su.ConsentVersion), isoTime(&created), isoTime(su.ConfirmedAt),
			isoTime(su.InvitedAt), isoTime(su.JoinedAt), isoTime(su.UnsubscribedAt)})
	})
	cw.Flush()
}

func (s *Server) apiListWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.Webhooks(r.Context(), apiProject(r))
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if list == nil {
		list = []*Webhook{}
	}
	writeJSON(w, 200, map[string]any{"data": list})
}

func (s *Server) apiCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL    string   `json:"url"`
		Kind   string   `json:"kind"`
		Events []string `json:"events"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	wh, err := s.app.CreateWebhook(r.Context(), apiProject(r), body.URL, body.Kind, body.Events)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, wh)
}

func (s *Server) apiDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r)
	if err := s.app.DeleteWebhook(r.Context(), apiProject(r), id); err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) apiTestWebhook(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r)
	if err := s.app.TestWebhook(r.Context(), apiProject(r), id); err != nil {
		jsonError(w, 404, "not found")
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true})
}

func (s *Server) apiDeliveries(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.Deliveries(r.Context(), apiProject(r), 100)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if list == nil {
		list = []*Delivery{}
	}
	writeJSON(w, 200, map[string]any{"data": list})
}

func (s *Server) apiRedeliver(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r)
	if err := s.app.Redeliver(r.Context(), apiProject(r), id); err != nil {
		jsonError(w, 404, "not found")
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true})
}

func (s *Server) apiBroadcast(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		Audience string `json:"audience"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if body.Audience == "" {
		body.Audience = "confirmed"
	}
	n, err := s.app.Broadcast(r.Context(), apiProject(r), body.Subject, body.Body, body.Audience)
	if err != nil {
		limitError(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "recipients": n})
}
