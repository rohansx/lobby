package app

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Measurement for Launch Pack campaigns (cloud only): visits to the hosted
// page counted per project, UTC day and utm_source, the optional
// qualification question, and the campaign link for each task.

const (
	maxQuestionChars = 200
	maxAnswerChars   = 300
	otherSource      = "(other)" // visits whose utm_source doesn't look like a tag
	maxDaySources    = 50        // distinct sources per project per day; later ones fold into otherSource
	maxIPVisits      = 20        // counted visits per address per project per hour
)

var sourceRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// visitSource buckets a utm_source: ” when absent, the tag when it looks like
// one, otherSource for anything else.
func visitSource(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || sourceRe.MatchString(s) {
		return s
	}
	return otherSource
}

var botMarks = []string{"bot", "crawl", "spider", "slurp", "preview", "facebookexternalhit", "headless", "lighthouse", "curl", "wget", "python-requests", "go-http-client"}

// countable is a GET from something that looks like a person's browser
// actually opening the page: not HEAD, not a prefetch, not a bot or link preview.
func countable(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	for _, h := range []string{"Purpose", "Sec-Purpose", "X-Purpose", "X-Moz"} {
		if v := strings.ToLower(r.Header.Get(h)); strings.Contains(v, "prefetch") || strings.Contains(v, "preview") || strings.Contains(v, "prerender") {
			return false
		}
	}
	ua := strings.ToLower(r.UserAgent())
	if ua == "" {
		return false
	}
	for _, m := range botMarks {
		if strings.Contains(ua, m) {
			return false
		}
	}
	return true
}

// countVisit adds one to today's visits for the request's source. A source
// past the day's cap counts as otherSource, so the table can't be flooded
// with made-up tags. A failed write is logged and never breaks the page.
// ponytail: one small write per counted view; batch in memory if the writer ever feels it.
func (a *App) countVisit(r *http.Request, p *Project) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 500*time.Millisecond)
	defer cancel()
	day := time.Now().UTC().Format("2006-01-02")
	src := visitSource(r.URL.Query().Get("utm_source"))
	if _, err := a.db.W.ExecContext(ctx, `INSERT INTO visits (project_id, day, source, count)
		SELECT ?1, ?2, CASE WHEN ?3 IN ('', ?4)
			OR EXISTS (SELECT 1 FROM visits WHERE project_id = ?1 AND day = ?2 AND source = ?3)
			OR (SELECT COUNT(*) FROM visits WHERE project_id = ?1 AND day = ?2) < ?5 THEN ?3 ELSE ?4 END, 1 WHERE true
		ON CONFLICT (project_id, day, source) DO UPDATE SET count = count + 1`, p.ID, day, src, otherSource, maxDaySources); err != nil {
		a.log.Warn("count visit", "project", p.ID, "err", err)
	}
}

// SetRelevant records the founder's review of a signup: true, false, or nil to clear.
func (a *App) SetRelevant(ctx context.Context, p *Project, id int64, relevant *bool) error {
	res, err := a.db.W.ExecContext(ctx, `UPDATE signups SET relevant = ? WHERE project_id = ? AND id = ?`, relevant, p.ID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CampaignLink is the tagged hosted-page link for one destination on a campaign.
func (a *App) CampaignLink(projectSlug, destinationSlug string, campaignID int64) string {
	if projectSlug == "" {
		return ""
	}
	return a.cfg.BaseURL + "/w/" + projectSlug + "?utm_source=" + url.QueryEscape(destinationSlug) +
		"&utm_campaign=c" + strconv.FormatInt(campaignID, 10)
}

func (s *Server) adminSignupRelevant(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	var body struct {
		Relevant *bool `json:"relevant"` // null clears
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "Send {\"relevant\": true, false or null}.")
		return
	}
	if err := s.app.SetRelevant(r.Context(), p, pathInt(r, "id"), body.Relevant); err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "relevant": body.Relevant})
}
