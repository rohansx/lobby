package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// composeFeedbackNotify tells the founder a new issue was opened.
func (a *App) composeFeedbackNotify(ctx context.Context, p *Project, s *Signup, issueID int64) (*Message, error) {
	iss, err := a.Issue(ctx, p, issueID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil // merged or deleted since
	} else if err != nil {
		return nil, err
	}
	var comment string
	a.db.R.QueryRowContext(ctx, `SELECT comment FROM feedback_reports WHERE issue_id = ? ORDER BY id LIMIT 1`, issueID).Scan(&comment)
	rows := [][2]string{{"From", s.Email}, {"Type", iss.Type}, {"Page", iss.Path}}
	if iss.Context != "" {
		rows = append(rows, [2]string{"On the page", iss.Context})
	}
	rows = append(rows, [2]string{"Element", iss.Selector})
	if iss.ElementText != "" {
		rows = append(rows, [2]string{"Element text", iss.ElementText})
	}
	rows = append(rows, [2]string{"Comment", clipRunes(comment, 500)})
	text, htm := email{
		Project:   p.Name,
		Preheader: s.Email + " reported a problem in " + p.Name + ".",
		Heading:   "New feedback issue",
		Paras:     []string{iss.Title},
		Rows:      rows,
		CTA:       "Open the issue",
		URL:       fmt.Sprintf("%s/p/%s/feedback/%d", a.cfg.BaseURL, p.Slug, iss.ID),
		Footer:    "New issues are gathered for 10 minutes into one email, and repeat reports don't send one. Change the address in the project's settings.",
	}.render()
	return &Message{
		From: (&mail.Address{Name: p.Name + " feedback", Address: a.senderEmail()}).String(),
		To:   p.NotifyEmail, ReplyTo: s.Email,
		Subject: "New feedback on " + p.Name + ": " + iss.Title, Text: text, HTML: htm,
	}, nil
}

// feedbackBatchWindow is how long the founder's "new feedback" email waits
// after the first new issue, so a testing session arrives as one email.
const (
	feedbackBatchWindow = 10 * 60
	feedbackBatchList   = 20 // issues listed in one email; the rest are a count
)

type feedbackBatchJob struct {
	ProjectID int64 `json:"project_id"`
	Since     int64 `json:"since"` // issues opened in [since, until)
	Until     int64 `json:"until"`
}

// queueFeedbackBatch schedules one email for the issues opened in the next
// window, unless a window is already waiting. A window whose job has started
// (run_at passed) is closed: this issue starts the next one.
func (a *App) queueFeedbackBatch(ctx context.Context, tx *sql.Tx, p *Project, at int64) error {
	var open int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE kind = 'email.feedback_batch' AND status = 'queued'
		AND run_at > ? AND json_extract(payload, '$.project_id') = ?`, at, p.ID).Scan(&open)
	if err != nil || open > 0 {
		return err
	}
	until := at + feedbackBatchWindow
	return a.enqueueAt(ctx, tx, "email.feedback_batch", feedbackBatchJob{ProjectID: p.ID, Since: at, Until: until}, until)
}

func (a *App) sendFeedbackBatch(ctx context.Context, payload string) error {
	var j feedbackBatchJob
	if err := json.Unmarshal([]byte(payload), &j); err != nil {
		return fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	p, err := a.ProjectByID(ctx, j.ProjectID)
	if errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if p.NotifyEmail == "" {
		return nil
	}
	if a.ownerSuspended(ctx, p.OwnerID) {
		return fmt.Errorf("%w: account suspended, email not sent", ErrPermanent)
	}
	rows, err := a.db.W.QueryContext(ctx, `SELECT i.id, i.type, i.title, i.path,
		(SELECT s.email FROM feedback_reports r JOIN signups s ON s.id = r.signup_id WHERE r.issue_id = i.id ORDER BY r.id LIMIT 1),
		(SELECT r.comment FROM feedback_reports r WHERE r.issue_id = i.id ORDER BY r.id LIMIT 1)
		FROM feedback_issues i WHERE i.project_id = ? AND i.created_at >= ? AND i.created_at < ? AND i.merged_into IS NULL
		ORDER BY i.id`, p.ID, j.Since, j.Until)
	if err != nil {
		return err
	}
	type item struct {
		id                         int64
		kind, title, path, from, c string
	}
	var list []item
	for rows.Next() {
		var it item
		var from, c sql.NullString
		if err := rows.Scan(&it.id, &it.kind, &it.title, &it.path, &from, &c); err != nil {
			rows.Close()
			return err
		}
		it.from, it.c = from.String, c.String
		list = append(list, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	switch len(list) {
	case 0:
		return nil // all merged or deleted since
	case 1: // the detailed single-issue email
		var sid int64
		a.db.W.QueryRowContext(ctx, `SELECT signup_id FROM feedback_reports WHERE issue_id = ? ORDER BY id LIMIT 1`, list[0].id).Scan(&sid)
		s, err := getSignup(ctx, a.db.W, p.ID, sid)
		if err != nil {
			return nil // reporter deleted since
		}
		m, err := a.composeFeedbackNotify(ctx, p, s, list[0].id)
		if err != nil || m == nil {
			return err
		}
		return a.mailer.Send(ctx, m)
	}
	var rs [][2]string
	var links [][2]string
	for i, it := range list {
		if i == feedbackBatchList {
			break
		}
		rs = append(rs, [2]string{it.kind + " · " + it.path, it.title + "\n" + it.from + ": " + clipRunes(it.c, 200)})
		links = append(links, [2]string{clipRunes(it.title, 60), fmt.Sprintf("%s/p/%s/feedback/%d", a.cfg.BaseURL, p.Slug, it.id)})
	}
	paras := []string{fmt.Sprintf("%d new issues came in over the last few minutes.", len(list))}
	if len(list) > feedbackBatchList {
		paras = append(paras, fmt.Sprintf("The first %d are below; the rest are on the feedback page.", feedbackBatchList))
	}
	text, htm := email{
		Project:   p.Name,
		Preheader: fmt.Sprintf("%d new feedback issues in %s.", len(list), p.Name),
		Heading:   fmt.Sprintf("%d new feedback issues", len(list)),
		Paras:     paras,
		Rows:      rs,
		CTA:       "Open feedback",
		URL:       fmt.Sprintf("%s/p/%s/feedback", a.cfg.BaseURL, p.Slug),
		Links:     links,
		Footer:    "New issues are gathered for 10 minutes into one email. Change the address in the project's settings.",
	}.render()
	return a.mailer.Send(ctx, &Message{
		From:    (&mail.Address{Name: p.Name + " feedback", Address: a.senderEmail()}).String(),
		To:      p.NotifyEmail,
		Subject: fmt.Sprintf("New feedback on %s: %d new issues", p.Name, len(list)), Text: text, HTML: htm,
	})
}

type pendingReply struct {
	id                        int64
	kind, title, path, origin string
	comment                   string
}

// feedbackWords keeps "fixed" for bugs: an idea gets built, a confusing
// screen gets changed.
type feedbackWords struct {
	Ack, Verb, Heading, Done, Works, Broken string
}

var feedbackCopy = map[string]feedbackWords{
	"bug": {Ack: "Thanks. You'll get an email when it's fixed.", Verb: "Fixed", Heading: "what you reported",
		Done: "They've fixed it.", Works: "It works now", Broken: "It's still broken"},
	"confusing": {Ack: "Thanks. You'll get an email when it's been made clearer.", Verb: "Changed", Heading: "what you found confusing",
		Done: "They've changed it.", Works: "It's clear now", Broken: "It's still confusing"},
	"idea": {Ack: "Thanks for the idea. You'll get an email if it gets built.", Verb: "Built", Heading: "your idea",
		Done: "They've built it.", Works: "Looks good", Broken: "Not what I meant"},
}

func wordsFor(kind string) feedbackWords {
	if w, ok := feedbackCopy[kind]; ok {
		return w
	}
	return feedbackCopy["bug"]
}

// sendFixedMail sends one email listing every fix waiting for this reporter.
func (a *App) sendFixedMail(ctx context.Context, p *Project, s *Signup) error {
	var unsub *int64
	if err := a.db.W.QueryRowContext(ctx, `SELECT feedback_unsub_at FROM signups WHERE id = ?`, s.ID).Scan(&unsub); err != nil {
		return err
	}
	if unsub != nil {
		return nil
	}
	// A reply is only sent while its issue is still fixed: a fix that was
	// reopened before the email went out says nothing.
	rows, err := a.db.W.QueryContext(ctx, `SELECT r.id, i.type, i.title, i.path,
		(SELECT origin FROM feedback_reports WHERE issue_id = i.id AND signup_id = r.signup_id ORDER BY id DESC LIMIT 1),
		(SELECT comment FROM feedback_reports WHERE issue_id = i.id AND signup_id = r.signup_id ORDER BY id LIMIT 1)
		FROM feedback_replies r JOIN feedback_issues i ON i.id = r.issue_id
		WHERE r.signup_id = ? AND r.sent_at IS NULL AND i.state = ? ORDER BY r.id`, s.ID, IssueFixed)
	if err != nil {
		return err
	}
	var list []pendingReply
	for rows.Next() {
		var r pendingReply
		var origin, comment *string
		if err := rows.Scan(&r.id, &r.kind, &r.title, &r.path, &origin, &comment); err != nil {
			rows.Close()
			return err
		}
		if origin != nil {
			r.origin = *origin
		}
		if comment != nil {
			r.comment = *comment
		}
		list = append(list, r)
	}
	rows.Close()
	if len(list) == 0 {
		return nil
	}
	if err := a.mailer.Send(ctx, a.composeFixed(p, s, list)); err != nil {
		return err
	}
	ids := make([]any, len(list))
	marks := make([]string, len(list))
	for i, r := range list {
		ids[i], marks[i] = r.id, "?"
	}
	args := append([]any{now()}, ids...)
	if _, err := a.db.W.ExecContext(ctx, `UPDATE feedback_replies SET sent_at = ? WHERE id IN (`+strings.Join(marks, ",")+`)`, args...); err != nil {
		return err
	}
	a.countEmail(ctx, p)
	return nil
}

func (a *App) composeFixed(p *Project, s *Signup, list []pendingReply) *Message {
	unsub := a.cfg.BaseURL + "/fu/" + a.FeedbackUnsubscribeToken(s)
	w := wordsFor(list[0].kind)
	heading := w.Verb + ": " + w.Heading
	subject := w.Verb + " in " + p.Name + ": " + w.Heading
	preheader := w.Verb + " in " + p.Name + ": " + w.Heading + "."
	paras := []string{fmt.Sprintf("Thanks for your feedback on %s. %s", p.Name, w.Done)}
	if len(list) > 1 {
		heading = fmt.Sprintf("The team acted on %d things you sent", len(list))
		subject = fmt.Sprintf("Done in %s: %d things you sent", p.Name, len(list))
		preheader = fmt.Sprintf("%d things you sent in %s are done.", len(list), p.Name)
		paras = []string{fmt.Sprintf("Thanks for your feedback on %s. The team has acted on these.", p.Name)}
	}
	paras = append(paras, "Could you take a look? One click tells them whether it's right now.")
	var rows [][2]string
	var links [][2]string
	for _, r := range list {
		rows = append(rows, [2]string{"You said", clipRunes(r.comment, 300)})
		prefix := ""
		if len(list) > 1 {
			prefix = clipRunes(r.comment, 40) + ": "
		}
		if r.origin != "" {
			links = append(links, [2]string{prefix + "Open the page", r.origin + r.path})
		}
		tok, rw := a.replyToken(r.id), wordsFor(r.kind)
		links = append(links,
			[2]string{prefix + rw.Works, a.cfg.BaseURL + "/f/" + tok + "/works"},
			[2]string{prefix + rw.Broken, a.cfg.BaseURL + "/f/" + tok + "/broken"})
	}
	text, htm := email{
		Project:   p.Name,
		Preheader: preheader,
		Heading:   heading,
		Paras:     paras,
		Rows:      rows,
		Links:     links,
		Footer:    "You're getting this because you sent feedback in " + p.Name + ". Unsubscribing stops these emails only, not the launch email.",
		Unsub:     unsub,
	}.render()
	return &Message{
		From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: subject, Text: text, HTML: htm,
		Headers: map[string]string{
			"List-Unsubscribe":      "<" + unsub + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}
}
