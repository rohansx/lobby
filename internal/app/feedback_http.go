package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---- public: the widget ----

func (s *Server) feedbackJS(w http.ResponseWriter, r *http.Request) {
	b, err := staticFS.ReadFile("web/static/fb.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(b)
}

// feedbackPreflight can't see the project (a preflight has no body), so it
// allows any origin; the POST itself checks the project's allowed origins.
func (s *Server) feedbackPreflight(w http.ResponseWriter, r *http.Request) {
	if o := r.Header.Get("Origin"); o != "" {
		w.Header().Set("Access-Control-Allow-Origin", o)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicFeedback(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Project  string `json:"project"`
		Reporter string `json:"reporter"`
		ReportInput
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	p, err := s.app.ProjectByPublicKey(r.Context(), body.Project)
	if err != nil {
		jsonError(w, 404, "unknown project")
		return
	}
	if !s.cors(w, r, p) {
		jsonError(w, 403, "this site isn't on the project's allowed origins")
		return
	}
	if s.app.ownerSuspended(r.Context(), p.OwnerID) {
		jsonError(w, 403, "feedback is turned off for this project")
		return
	}
	if !s.app.spam.Allow("fb:"+strconv.FormatInt(p.ID, 10)+":"+s.clientIP(r), 30, time.Hour) {
		jsonError(w, 429, "too many reports from this network; please try again later")
		return
	}
	body.ReportInput.Origin = r.Header.Get("Origin")
	rep, iss, err := s.app.SubmitReport(r.Context(), p, body.Reporter, body.ReportInput)
	var ve *ValidationError
	var le *ErrLimit
	switch {
	case err == nil:
		writeJSON(w, 201, map[string]any{"ok": true, "issue": iss.ID, "message": wordsFor(rep.Type).Ack})
	case errors.As(err, &ve):
		jsonError(w, 400, ve.Msg)
	case errors.As(err, &le):
		jsonError(w, 402, le.Msg)
	case errors.Is(err, ErrNotReporter):
		jsonError(w, 401, err.Error())
	case errors.Is(err, ErrNoFeedback):
		jsonError(w, 403, "feedback isn't turned on for you here")
	case errors.Is(err, ErrReporterLimit):
		jsonError(w, 429, err.Error())
	default:
		s.app.log.Error("feedback", "err", err)
		jsonError(w, 500, "something went wrong")
	}
}

// publicFeedbackAccess tells the widget whether to show itself for this
// reporter. It only ever answers true or false.
func (s *Server) publicFeedbackAccess(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := s.app.ProjectByPublicKey(r.Context(), q.Get("project"))
	if err != nil {
		jsonError(w, 404, "unknown project")
		return
	}
	if !s.cors(w, r, p) {
		jsonError(w, 403, "this site isn't on the project's allowed origins")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ok := !s.app.ownerSuspended(r.Context(), p.OwnerID) && s.app.ReporterAllowed(r.Context(), p, q.Get("reporter"))
	writeJSON(w, 200, map[string]any{"enabled": ok})
}

// adminSignupFeedback is the per-person switch on the Signups page:
// {"access": true|false|null}, null going back to the project default.
func (s *Server) adminSignupFeedback(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	id, _ := pathID(r)
	var body struct {
		Access *bool `json:"access"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, `send {"access": true|false|null}`)
		return
	}
	if n, err := s.app.SetFeedbackAccess(r.Context(), p, []int64{id}, body.Access); err != nil || n == 0 {
		jsonError(w, 404, "signup not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- public: the reporter's answer and unsubscribe ----

func (s *Server) fixAnswerPage(w http.ResponseWriter, r *http.Request) {
	answer := r.PathValue("answer")
	if answer != "works" && answer != "broken" {
		http.NotFound(w, r)
		return
	}
	// A button, not a GET that acts: mail scanners open every link.
	title, msg, btn := "All good now?", "Tell the team it's right now.", "Yes, it's right now"
	if answer == "broken" {
		title, msg, btn = "Not right yet?", "Tell the team it still isn't right. They'll take another look.", "It still isn't right"
	}
	s.render(w, 200, "confirm", map[string]any{"Title": title, "Message": msg, "Button": btn,
		"Action": "/f/" + r.PathValue("token") + "/" + answer})
}

func (s *Server) fixAnswerPost(w http.ResponseWriter, r *http.Request) {
	answer := r.PathValue("answer")
	p, err := s.app.AnswerFix(r.Context(), r.PathValue("token"), answer)
	if err != nil {
		http.Error(w, "invalid link", http.StatusNotFound)
		return
	}
	msg := "Glad it's sorted. The team can see your answer."
	if answer == "broken" {
		msg = "Sorry about that. It's open again and the team will take another look."
	}
	s.render(w, 200, "message", map[string]any{"Title": "Thanks for checking", "Message": msg, "Project": p})
}

func (s *Server) feedbackUnsubPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, 200, "confirm", map[string]any{"Title": "Stop feedback emails?",
		"Message": "You won't hear when things you reported are fixed. Launch and invite emails aren't affected.",
		"Button":  "Unsubscribe", "Action": "/fu/" + r.PathValue("token")})
}

func (s *Server) feedbackUnsubPost(w http.ResponseWriter, r *http.Request) {
	if err := s.app.FeedbackUnsubscribe(r.Context(), r.PathValue("token")); err != nil {
		http.Error(w, "invalid link", http.StatusNotFound)
		return
	}
	if r.FormValue("List-Unsubscribe") == "One-Click" {
		w.WriteHeader(http.StatusOK)
		return
	}
	s.render(w, 200, "message", map[string]any{"Title": "You're unsubscribed", "Message": "You won't get any more feedback emails."})
}

// ---- API: secret key or agent key ----

type keyRole struct{}

const (
	roleSecret = "secret"
	roleAgent  = "agent"
)

func keyRoleOf(r *http.Request) string { return r.Context().Value(keyRole{}).(string) }

// requireIssueKey accepts the project secret key (everything) or the agent
// key (approved issues and notes only).
func (s *Server) requireIssueKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		var p *Project
		var err error
		role := roleSecret
		switch {
		case strings.HasPrefix(tok, "sk_"):
			p, err = s.app.ProjectBySecret(r.Context(), tok)
		case strings.HasPrefix(tok, "ak_"):
			role = roleAgent
			p, err = s.app.ProjectByAgentKey(r.Context(), tok)
		default:
			jsonError(w, 401, "send the agent key (ak_...) or the secret key (sk_...) as Authorization: Bearer ...")
			return
		}
		if err != nil {
			jsonError(w, 401, "invalid key")
			return
		}
		if plan, _ := s.app.planForOwner(r.Context(), p.OwnerID); !plan.Webhooks {
			jsonError(w, http.StatusPaymentRequired, "the feedback API, CLI and agent key come with a Launch Pack; Copy as prompt in the dashboard is free")
			return
		}
		ctx := context.WithValue(context.WithValue(r.Context(), projectKey, p), keyRole{}, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// apiIssue loads {id}. An agent only ever sees approved issues; anything
// else reads as not found.
func (s *Server) apiIssue(w http.ResponseWriter, r *http.Request) *Issue {
	id, ok := pathID(r)
	if !ok {
		jsonError(w, 404, "issue not found")
		return nil
	}
	iss, err := s.app.Issue(r.Context(), apiProject(r), id)
	if err == nil && keyRoleOf(r) == roleAgent && iss.State != IssueApproved {
		err = ErrNotFound
	}
	if err != nil {
		jsonError(w, 404, "issue not found")
		return nil
	}
	return iss
}

func (s *Server) apiIssues(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if keyRoleOf(r) == roleAgent {
		if state != "" && state != IssueApproved {
			jsonError(w, 403, "the agent key only sees approved issues")
			return
		}
		state = IssueApproved
	}
	if state != "" && !contains([]string{IssueOpen, IssueApproved, IssueFixed, IssueDismissed}, state) {
		jsonError(w, 400, "state must be open, approved, fixed or dismissed")
		return
	}
	list, err := s.app.Issues(r.Context(), apiProject(r), IssueFilter{State: state, Type: r.URL.Query().Get("type")})
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"data": list})
}

func (s *Server) apiPacket(w http.ResponseWriter, r *http.Request) {
	iss := s.apiIssue(w, r)
	if iss == nil {
		return
	}
	pk, err := s.app.BuildPacket(r.Context(), apiProject(r), iss)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	s.app.RecordHandoff(r.Context(), apiProject(r), iss.ID)
	if r.URL.Query().Get("format") == "md" || strings.Contains(r.Header.Get("Accept"), "text/markdown") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Write([]byte(pk.Markdown()))
		return
	}
	writeJSON(w, 200, pk)
}

func (s *Server) apiIssueNote(w http.ResponseWriter, r *http.Request) {
	iss := s.apiIssue(w, r)
	if iss == nil {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	out, err := s.app.NoteIssue(r.Context(), apiProject(r), iss.ID, body.Note)
	s.issueResult(w, out, err)
}

func (s *Server) apiIssueState(w http.ResponseWriter, r *http.Request) {
	if keyRoleOf(r) != roleSecret {
		jsonError(w, 403, "only the secret key can change an issue's state; the founder marks issues fixed")
		return
	}
	iss := s.apiIssue(w, r)
	if iss == nil {
		return
	}
	var body struct {
		State string `json:"state"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if !contains([]string{IssueOpen, IssueApproved, IssueFixed, IssueDismissed}, body.State) {
		jsonError(w, 400, "state must be open, approved, fixed or dismissed")
		return
	}
	out, err := s.app.MoveIssue(r.Context(), apiProject(r), iss.ID, body.State)
	s.issueResult(w, out, err)
}

func (s *Server) issueResult(w http.ResponseWriter, iss *Issue, err error) {
	var ve *ValidationError
	switch {
	case err == nil:
		writeJSON(w, 200, iss)
	case errors.As(err, &ve):
		jsonError(w, 400, ve.Msg)
	case errors.Is(err, ErrNotFound):
		jsonError(w, 404, "issue not found")
	case errors.Is(err, ErrBadIssueMove):
		jsonError(w, 409, err.Error())
	default:
		jsonError(w, 500, err.Error())
	}
}

// apiReporter mints a reporter token for an invited person. It needs the
// secret key but no paid plan: the widget is free.
func (s *Server) apiReporter(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	p, err := s.app.ProjectBySecret(r.Context(), tok)
	if !strings.HasPrefix(tok, "sk_") || err != nil {
		jsonError(w, 401, "send the project secret key as Authorization: Bearer sk_...")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Email == "" {
		jsonError(w, 400, `send {"email": "..."}`)
		return
	}
	rt, su, err := s.app.MintReporter(r.Context(), p, body.Email)
	switch {
	case err == nil:
		writeJSON(w, 200, map[string]any{"reporter_token": rt, "signup_id": su.ID, "expires_in": reporterTTL})
	case errors.Is(err, ErrNotFound):
		jsonError(w, 404, "no one with that email is on this waitlist")
	case errors.Is(err, ErrNoFeedback):
		jsonError(w, 403, err.Error())
	default:
		jsonError(w, 500, err.Error())
	}
}

// ---- dashboard ----

func (s *Server) adminFeedback(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	q := r.URL.Query()
	list, err := s.app.Issues(r.Context(), p, IssueFilter{State: q.Get("state"), Type: q.Get("type"), Wave: q.Get("wave"), Source: q.Get("source")})
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	waves, sources, err := s.app.IssueFacets(r.Context(), p)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	plan, _ := s.app.planForOwner(r.Context(), p.OwnerID)
	var reports int64
	s.app.db.R.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM feedback_reports WHERE project_id = ?`, p.ID).Scan(&reports)
	limit := int64(-1)
	if s.app.cfg.Cloud && !plan.Webhooks {
		limit = freeReportsPerProject
	}
	writeJSON(w, 200, map[string]any{
		"data": list, "waves": waves, "sources": sources,
		"settings": s.app.FeedbackSettings(r.Context(), p),
		"api":      plan.Webhooks, "reports": reports, "report_limit": limit,
		"ai_available": s.app.ai != nil,
	})
}

func (s *Server) adminIssue(w http.ResponseWriter, r *http.Request) (*Project, *Issue) {
	p := s.adminProject(w, r)
	if p == nil {
		return nil, nil
	}
	id, _ := pathID(r)
	iss, err := s.app.Issue(r.Context(), p, id)
	if err != nil {
		jsonError(w, 404, "issue not found")
		return nil, nil
	}
	return p, iss
}

func (s *Server) adminGetIssue(w http.ResponseWriter, r *http.Request) {
	p, iss := s.adminIssue(w, r)
	if iss == nil {
		return
	}
	reps, err := s.app.Reports(r.Context(), p, iss.ID)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	pk, err := s.app.BuildPacket(r.Context(), p, iss)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	replies, err := s.app.IssueReplies(r.Context(), iss.ID)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	blocked := map[int64]bool{}
	rows, err := s.app.db.R.QueryContext(r.Context(), `SELECT DISTINCT s.id FROM feedback_reports f JOIN signups s ON s.id = f.signup_id
		WHERE f.issue_id = ? AND s.feedback_access = 0`, iss.ID)
	if err == nil {
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			blocked[id] = true
		}
		rows.Close()
	}
	blockedIDs := []int64{}
	for id := range blocked {
		blockedIDs = append(blockedIDs, id)
	}
	suggestions, err := s.app.Suggestions(r.Context(), p, iss.ID)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"issue": iss, "reports": reps, "packet": pk, "packet_md": pk.Markdown(),
		"replies": replies, "blocked": blockedIDs, "suggestions": suggestions})
}

func (s *Server) adminIssueAction(w http.ResponseWriter, r *http.Request) {
	p, iss := s.adminIssue(w, r)
	if iss == nil {
		return
	}
	var body struct {
		Into    int64   `json:"into"`
		Reports []int64 `json:"reports"`
		Title   string  `json:"title"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	ctx := r.Context()
	var out *Issue
	var err error
	switch r.PathValue("action") {
	case "approve":
		out, err = s.app.MoveIssue(ctx, p, iss.ID, IssueApproved)
	case "dismiss":
		out, err = s.app.MoveIssue(ctx, p, iss.ID, IssueDismissed)
	case "fix":
		out, err = s.app.MoveIssue(ctx, p, iss.ID, IssueFixed)
	case "reopen":
		out, err = s.app.MoveIssue(ctx, p, iss.ID, IssueOpen)
	case "merge":
		out, err = s.app.MergeIssues(ctx, p, iss.ID, body.Into)
	case "split":
		out, err = s.app.SplitIssue(ctx, p, iss.ID, body.Reports)
	case "title":
		out, err = s.app.SetIssueTitle(ctx, p, iss.ID, body.Title)
	case "not-duplicate":
		if err := s.app.DismissSuggestion(ctx, p, iss.ID, body.Into); err != nil {
			jsonError(w, 404, "issue not found")
			return
		}
		out, err = s.app.Issue(ctx, p, iss.ID)
	case "copied":
		s.app.RecordHandoff(ctx, p, iss.ID)
		out, err = s.app.Issue(ctx, p, iss.ID)
	default:
		jsonError(w, 404, "unknown action")
		return
	}
	s.issueResult(w, out, err)
}

func (s *Server) adminFeedbackSettings(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	var body struct {
		Console  *bool   `json:"console"`
		Audience *string `json:"audience"`
		AI       *bool   `json:"ai"`
		Digest   *bool   `json:"digest"`
	}
	if err := decodeJSON(r, &body); err != nil || (body.Console == nil && body.Audience == nil && body.AI == nil && body.Digest == nil) {
		jsonError(w, 400, `send any of {"console": bool, "audience": "invited"|"selected", "ai": bool, "digest": bool}`)
		return
	}
	for col, v := range map[string]*bool{"ai": body.AI, "digest": body.Digest} {
		if v != nil {
			if err := s.app.SetFeedbackFlag(r.Context(), p, col, *v); err != nil {
				jsonError(w, 500, err.Error())
				return
			}
		}
	}
	if body.Console != nil {
		if err := s.app.SetFeedbackConsole(r.Context(), p, *body.Console); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
	}
	if body.Audience != nil {
		if err := s.app.SetFeedbackAudience(r.Context(), p, *body.Audience); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
	}
	writeJSON(w, 200, s.app.FeedbackSettings(r.Context(), p))
}

func (s *Server) adminAgentKey(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	k, err := s.app.RotateAgentKey(r.Context(), p)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"agent_key": k})
}

// adminReporter blocks or unblocks a reporter, or mints a token so the
// founder can try the widget as one of their invited users.
func (s *Server) adminReporter(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	id, _ := pathID(r)
	switch r.PathValue("action") {
	case "block", "unblock":
		var on *bool // unblock: back to following the project's audience
		if r.PathValue("action") == "block" {
			off := false
			on = &off
		}
		if n, err := s.app.SetFeedbackAccess(r.Context(), p, []int64{id}, on); err != nil || n == 0 {
			jsonError(w, 404, "signup not found")
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case "token":
		su, err := s.app.Signup(r.Context(), p.ID, id)
		if err != nil {
			jsonError(w, 404, "signup not found")
			return
		}
		access, _ := s.app.feedbackAccess(r.Context(), s.app.db.R, su.ID)
		if !canReport(su.Status, access, s.app.FeedbackSettings(r.Context(), p).Audience) {
			jsonError(w, 409, ErrNoFeedback.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"reporter_token": s.app.ReporterToken(su)})
	default:
		jsonError(w, 404, "unknown action")
	}
}
