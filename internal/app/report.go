package app

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// The campaign results report. Window: plan approval until the report is
// published (or now). A signup belongs to the destination whose slug matches
// the utm_source it arrived with (first-known source); no source is
// "unknown", a source that isn't on the plan is "other". Visits are hosted
// page views counted per UTC day, so the window is rounded to whole days for them.

type ReportRow struct {
	Kind           string   `json:"kind"` // task | unknown | other | total
	TaskID         int64    `json:"task_id,omitempty"`
	Label          string   `json:"label"`
	Source         string   `json:"source"`
	DestinationURL string   `json:"destination_url,omitempty"`
	Owner          string   `json:"owner,omitempty"`
	State          string   `json:"state,omitempty"`
	SubmittedAt    *int64   `json:"submitted_at,omitempty"`
	LiveAt         *int64   `json:"live_at,omitempty"`
	ListingURL     string   `json:"listing_url,omitempty"`
	FeeCents       int64    `json:"fee_cents"`
	Visits         *int64   `json:"visits"` // nil: no visits recorded, which isn't the same as 0
	Signups        int64    `json:"signups"`
	Confirmed      int64    `json:"confirmed"`
	Relevant       int64    `json:"relevant"`
	Conversion     *float64 `json:"conversion"` // signups / visits, only when visits > 0
	visits         int64
}

type Report struct {
	Campaign              *Campaign   `json:"campaign"`
	From                  *int64      `json:"from"` // nil until the plan is approved
	To                    int64       `json:"to"`
	VisitsTracked         bool        `json:"visits_tracked"`
	Rows                  []ReportRow `json:"rows"`
	Unknown               ReportRow   `json:"unknown"`
	Other                 ReportRow   `json:"other"`
	Total                 ReportRow   `json:"total"`
	PackPriceCents        int64       `json:"pack_price_cents"`
	PlatformFeesCents     int64       `json:"platform_fees_cents"`
	AttributedConfirmed   int64       `json:"attributed_confirmed"`
	CostPerConfirmedCents *int64      `json:"cost_per_confirmed_cents"` // (pack + platform fees) / attributed confirmed
}

type sourceCounts struct{ visits, signups, confirmed, relevant int64 }

// CampaignReport computes the report from the campaign's tasks, visits and signups.
func (a *App) CampaignReport(ctx context.Context, c *Campaign) (*Report, error) {
	tasks, err := a.Tasks(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	rep := &Report{Campaign: c, From: c.ApprovedAt, To: now(), PackPriceCents: c.PriceCents, Rows: []ReportRow{}}
	if c.ReportPublishedAt != nil {
		rep.To = *c.ReportPublishedAt
	}
	counts := map[string]*sourceCounts{}
	if c.ProjectID != nil && c.ApprovedAt != nil {
		if counts, err = a.reportCounts(ctx, *c.ProjectID, *c.ApprovedAt, rep.To); err != nil {
			return nil, err
		}
	}
	for _, n := range counts {
		rep.VisitsTracked = rep.VisitsTracked || n.visits > 0
	}
	take := func(src string) sourceCounts {
		n := counts[src]
		delete(counts, src)
		if n == nil {
			return sourceCounts{}
		}
		return *n
	}
	for _, t := range tasks {
		row := ReportRow{Kind: "task", TaskID: t.ID, Label: t.DestinationName, Source: t.DestinationSlug,
			DestinationURL: t.DestinationURL, Owner: t.Owner, State: t.State, SubmittedAt: t.SubmittedAt, LiveAt: t.LiveAt,
			ListingURL: t.ListingURL, FeeCents: t.FeeCents}
		rep.fill(&row, take(strings.ToLower(t.DestinationSlug)))
		rep.PlatformFeesCents += t.FeeCents
		rep.AttributedConfirmed += row.Confirmed
		rep.Rows = append(rep.Rows, row)
	}
	rep.Unknown = ReportRow{Kind: "unknown", Label: "Unknown source"}
	rep.fill(&rep.Unknown, take(""))
	var other sourceCounts
	for _, n := range counts {
		other.visits += n.visits
		other.signups += n.signups
		other.confirmed += n.confirmed
		other.relevant += n.relevant
	}
	rep.Other = ReportRow{Kind: "other", Label: "Other sources"}
	rep.fill(&rep.Other, other)

	var total sourceCounts
	for _, r := range append(append([]ReportRow{}, rep.Rows...), rep.Unknown, rep.Other) {
		total.visits += r.visits
		total.signups += r.Signups
		total.confirmed += r.Confirmed
		total.relevant += r.Relevant
	}
	rep.Total = ReportRow{Kind: "total", Label: "All sources", FeeCents: rep.PlatformFeesCents}
	rep.fill(&rep.Total, total)
	if rep.AttributedConfirmed > 0 {
		cost := (rep.PackPriceCents + rep.PlatformFeesCents) / rep.AttributedConfirmed
		rep.CostPerConfirmedCents = &cost
	}
	return rep, nil
}

func (rep *Report) fill(row *ReportRow, n sourceCounts) {
	row.Signups, row.Confirmed, row.Relevant, row.visits = n.signups, n.confirmed, n.relevant, n.visits
	if !rep.VisitsTracked {
		return
	}
	v := n.visits
	row.Visits = &v
	if v > 0 {
		conv := float64(n.signups) / float64(v)
		row.Conversion = &conv
	}
}

// reportCounts groups the project's visits and signups in the window by source.
func (a *App) reportCounts(ctx context.Context, projectID, from, to int64) (map[string]*sourceCounts, error) {
	out := map[string]*sourceCounts{}
	get := func(src string) *sourceCounts {
		if out[src] == nil {
			out[src] = &sourceCounts{}
		}
		return out[src]
	}
	day := func(t int64) string { return time.Unix(t, 0).UTC().Format("2006-01-02") }
	rows, err := a.db.R.QueryContext(ctx, `SELECT source, SUM(count) FROM visits WHERE project_id = ? AND day >= ? AND day <= ? GROUP BY source`,
		projectID, day(from), day(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		var n int64
		if err := rows.Scan(&src, &n); err != nil {
			return nil, err
		}
		get(src).visits += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Signups are unique per address already; removed ones are left out.
	rows, err = a.db.R.QueryContext(ctx, `SELECT LOWER(TRIM(utm_source)), COUNT(*),
		COALESCE(SUM(confirmed_at IS NOT NULL AND confirmed_at <= ?), 0), COALESCE(SUM(relevant = 1), 0)
		FROM signups WHERE project_id = ? AND created_at >= ? AND created_at <= ? AND status != ?
		GROUP BY LOWER(TRIM(utm_source))`, to, projectID, from, to, StatusRemoved)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		var n sourceCounts
		if err := rows.Scan(&src, &n.signups, &n.confirmed, &n.relevant); err != nil {
			return nil, err
		}
		c := get(src)
		c.signups, c.confirmed, c.relevant = c.signups+n.signups, c.confirmed+n.confirmed, c.relevant+n.relevant
	}
	return out, rows.Err()
}

func (s *Server) adminCampaignReport(w http.ResponseWriter, r *http.Request) {
	c := s.founderCampaign(w, r)
	if c == nil {
		return
	}
	s.writeReport(w, r, c.forFounder())
}

func (s *Server) opsCampaignReport(w http.ResponseWriter, r *http.Request) {
	if c := s.opsLoadCampaign(w, r); c != nil {
		s.writeReport(w, r, c)
	}
}

func (s *Server) writeReport(w http.ResponseWriter, r *http.Request, c *Campaign) {
	rep, err := s.app.CampaignReport(r.Context(), c)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, rep)
}
