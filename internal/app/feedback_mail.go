package app

import (
	"context"
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
		Footer:    "You get one of these for each new issue, not for every report. Change the address in the project's settings.",
	}.render()
	return &Message{
		From: (&mail.Address{Name: p.Name + " feedback", Address: a.senderEmail()}).String(),
		To:   p.NotifyEmail, ReplyTo: s.Email,
		Subject: "New feedback on " + p.Name + ": " + iss.Title, Text: text, HTML: htm,
	}, nil
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
