package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// The weekly feedback digest: what users reported, what got fixed, and, with
// Claude, a short read of where to look first. It goes to the project's
// notification address, only in weeks with feedback activity.

const digestEvery = 7 * 86400

// scheduleDigests runs from maintain (hourly). A project's first digest goes
// out a week after its clock starts, so nobody gets one minutes after setup.
func (a *App) scheduleDigests(ctx context.Context) {
	t := now()
	a.db.W.ExecContext(ctx, `INSERT INTO feedback_settings (project_id, digest_at)
		SELECT DISTINCT project_id, ? FROM feedback_reports
		WHERE project_id NOT IN (SELECT project_id FROM feedback_settings)`, t)
	a.db.W.ExecContext(ctx, `UPDATE feedback_settings SET digest_at = ? WHERE digest_at IS NULL`, t)
	rows, err := a.db.W.QueryContext(ctx, `SELECT f.project_id FROM feedback_settings f JOIN projects p ON p.id = f.project_id
		WHERE f.digest = 1 AND p.notify_email != '' AND f.digest_at <= ?
		AND (EXISTS (SELECT 1 FROM feedback_reports r WHERE r.project_id = f.project_id AND r.created_at > ?)
		  OR EXISTS (SELECT 1 FROM feedback_issues i WHERE i.project_id = f.project_id AND i.fixed_at > ?))`,
		t-digestEvery, t-digestEvery, t-digestEvery)
	if err != nil {
		a.log.Error("digest", "err", err)
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		payload, _ := json.Marshal(aiIssueJob{ProjectID: id})
		a.db.W.ExecContext(ctx, `UPDATE feedback_settings SET digest_at = ? WHERE project_id = ?`, t, id)
		a.db.W.ExecContext(ctx, `INSERT INTO jobs (kind, payload, run_at, created_at) VALUES ('feedback.digest', ?, ?, ?)`, string(payload), t, t)
	}
	if len(ids) > 0 {
		a.kick()
	}
}

type digestStats struct {
	Reports, NewIssues, Fixed, Works, Broken, Open int64
	Top                                            []*Issue
}

func (a *App) digestStats(ctx context.Context, p *Project) (*digestStats, error) {
	since := now() - digestEvery
	st := &digestStats{}
	err := a.db.R.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM feedback_reports WHERE project_id = ? AND created_at > ?),
		(SELECT COUNT(*) FROM feedback_issues WHERE project_id = ? AND state != 'merged' AND created_at > ?),
		(SELECT COUNT(*) FROM feedback_issues WHERE project_id = ? AND state = 'fixed' AND fixed_at > ?),
		(SELECT COUNT(*) FROM feedback_replies r JOIN feedback_issues i ON i.id = r.issue_id WHERE i.project_id = ? AND r.answer = 'works' AND r.answered_at > ?),
		(SELECT COUNT(*) FROM feedback_replies r JOIN feedback_issues i ON i.id = r.issue_id WHERE i.project_id = ? AND r.answer = 'broken' AND r.answered_at > ?),
		(SELECT COUNT(*) FROM feedback_issues WHERE project_id = ? AND state IN ('open', 'approved'))`,
		p.ID, since, p.ID, since, p.ID, since, p.ID, since, p.ID, since, p.ID).
		Scan(&st.Reports, &st.NewIssues, &st.Fixed, &st.Works, &st.Broken, &st.Open)
	if err != nil {
		return nil, err
	}
	for _, state := range []string{IssueOpen, IssueApproved} {
		list, err := a.Issues(ctx, p, IssueFilter{State: state})
		if err != nil {
			return nil, err
		}
		st.Top = append(st.Top, list...)
	}
	// Most reporters first, across both states; keep five.
	for i := 1; i < len(st.Top); i++ {
		for j := i; j > 0 && st.Top[j].Reporters > st.Top[j-1].Reporters; j-- {
			st.Top[j], st.Top[j-1] = st.Top[j-1], st.Top[j]
		}
	}
	if len(st.Top) > 5 {
		st.Top = st.Top[:5]
	}
	return st, nil
}

var digestSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["overview", "focus"],
  "properties": {
    "overview": {"type": "string", "description": "2-4 sentences, under 600 characters: the main things users ran into this week and how it's going."},
    "focus": {"type": "array", "description": "Up to 3 short, specific suggestions for what to look at first, most important first. Each names the issue number.", "items": {"type": "string"}}
  }
}`)

type digestAI struct {
	Overview string   `json:"overview"`
	Focus    []string `json:"focus"`
}

// sendDigest runs the feedback.digest job.
func (a *App) sendDigest(ctx context.Context, projectID int64) error {
	p, err := a.ProjectByID(ctx, projectID)
	if errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if p.NotifyEmail == "" || a.ownerSuspended(ctx, p.OwnerID) {
		return nil
	}
	st, err := a.digestStats(ctx, p)
	if err != nil {
		return err
	}
	var ai *digestAI
	if a.AIEnabled(ctx, p) && len(st.Top) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "<week reports=\"%d\" new_issues=\"%d\" fixed=\"%d\" works_now_replies=\"%d\" still_broken_replies=\"%d\" open_issues=\"%d\"/>\n<other_issues>\n",
			st.Reports, st.NewIssues, st.Fixed, st.Works, st.Broken, st.Open)
		for _, i := range st.Top {
			fmt.Fprintf(&b, "<issue id=\"%d\" state=\"%s\" reporters=\"%d\" page=\"%s\"><title>%s</title><summary>%s</summary></issue>\n",
				i.ID, i.State, i.Reporters, x(i.Path), x(i.Title), x(i.AISummary))
		}
		b.WriteString("</other_issues>\n\nWrite this week's overview of the feedback for the founder, and what to look at first.")
		var out digestAI
		if err := a.ai(ctx, aiSystem, b.String(), nil, digestSchema, &out); err == nil {
			ai = &out
		} else {
			a.log.Warn("digest: no AI overview", "project", p.ID, "err", err) // the numbers still go out
		}
	}
	return a.mailer.Send(ctx, a.composeDigest(p, st, ai))
}

func (a *App) composeDigest(p *Project, st *digestStats, ai *digestAI) *Message {
	paras := []string{fmt.Sprintf("This week: %s, %s, %s fixed.", plural(st.Reports, "report"), plural(st.NewIssues, "new issue"), plural(st.Fixed, "issue"))}
	if st.Works+st.Broken > 0 {
		paras = append(paras, fmt.Sprintf("After fixes, %d said it works now and %d said it's still not right.", st.Works, st.Broken))
	}
	if ai != nil && strings.TrimSpace(ai.Overview) != "" {
		paras = append(paras, clipRunes(ai.Overview, 800))
		for i, f := range ai.Focus {
			if i == 3 {
				break
			}
			paras = append(paras, "→ "+clipRunes(f, 240))
		}
	}
	var rows [][2]string
	for _, i := range st.Top {
		rows = append(rows, [2]string{plural(i.Reporters, "user"), fmt.Sprintf("#%d %s", i.ID, i.Title)})
	}
	footer := "You get this once a week when there's feedback. Turn it off on the Feedback page."
	if ai != nil {
		footer = "The overview is written by Claude from your users' reports. " + footer
	}
	text, htm := email{
		Project:   p.Name,
		Preheader: paras[0],
		Heading:   fmt.Sprintf("Feedback this week: %d open", st.Open),
		Paras:     paras,
		Rows:      rows,
		CTA:       "Open feedback",
		URL:       a.cfg.BaseURL + "/p/" + p.Slug + "/feedback",
		Footer:    footer,
	}.render()
	return &Message{
		From:    (&mail.Address{Name: p.Name + " feedback", Address: a.senderEmail()}).String(),
		To:      p.NotifyEmail,
		Subject: fmt.Sprintf("%s feedback this week: %s, %d fixed", p.Name, plural(st.NewIssues, "new issue"), st.Fixed),
		Text:    text, HTML: htm,
	}
}
