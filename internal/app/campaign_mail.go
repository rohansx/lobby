package app

import (
	"context"
	"encoding/json"
	"strconv"
)

// Emails. These are account mail from Lobby, like verification and resets,
// so they go through the job queue but don't count against the plan's allowance.

func (a *App) mailLobby(ctx context.Context, to, subject string, e email) {
	e.Project = "Lobby"
	text, htm := e.render()
	b, _ := json.Marshal(&Message{From: a.lobbyFrom(), To: to, Subject: subject, Text: text, HTML: htm})
	if _, err := a.db.W.ExecContext(ctx, `INSERT INTO jobs (kind, payload, run_at, created_at) VALUES ('email.lobby', ?, ?, ?)`, string(b), now(), now()); err != nil {
		a.log.Error("queue lobby email", "err", err)
		return
	}
	a.kick()
}

func (a *App) mailOperators(ctx context.Context, subject string, e email) {
	for _, to := range a.cfg.Operators {
		a.mailLobby(ctx, to, subject, e)
	}
}

func (a *App) campaignURL(c *Campaign) string {
	return a.cfg.BaseURL + "/campaigns/" + strconv.FormatInt(c.ID, 10)
}
func (a *App) opsCampaignURL(c *Campaign) string {
	return a.cfg.BaseURL + "/ops/campaigns/" + strconv.FormatInt(c.ID, 10)
}

func (a *App) mailCampaignStarted(ctx context.Context, c *Campaign) {
	a.mailLobby(ctx, c.OwnerEmail, "Your Launch Pack: describe your product", email{
		Preheader: "First step: fill in your product profile.",
		Heading:   "Your launch campaign has started",
		Paras: []string{
			"Thanks for buying the Launch Pack. The first step is yours: choose the waitlist for the product, fill in the product profile and upload your logo and screenshots.",
			"Leave out anything you don't know yet. We list what's missing rather than guess, and nothing is submitted anywhere until you approve the plan.",
		},
		CTA: "Fill in the profile", URL: a.campaignURL(c),
	})
	a.mailOperators(ctx, "New Launch Pack: "+c.OwnerEmail, email{
		Heading: "New campaign", Paras: []string{c.OwnerEmail + " started a Launch Pack campaign. It's in intake until they submit the profile."},
		CTA: "Open campaign", URL: a.opsCampaignURL(c),
	})
}

func (a *App) mailCampaignMove(ctx context.Context, from string, c *Campaign, note string, founderAction bool) {
	switch {
	case from == statusIntake && c.Status == statusReadiness:
		a.mailOperators(ctx, "Intake submitted: "+c.OwnerEmail, email{
			Heading: "Intake submitted", Paras: []string{c.OwnerEmail + " submitted their product profile. The readiness review is next."},
			CTA: "Review", URL: a.opsCampaignURL(c),
		})
	case c.Status == statusAwaitingApproval:
		a.mailLobby(ctx, c.OwnerEmail, "Your launch plan is ready to approve", email{
			Preheader: "Review the destinations and approve the plan.",
			Heading:   "Your launch plan is ready",
			Paras: []string{
				"We've picked the destinations that suit your product, with the reason for each, any platform fees and who does the final step.",
				"Nothing is submitted until you approve. If something looks wrong, send it back with a note.",
			},
			CTA: "Review the plan", URL: a.campaignURL(c),
		})
	case c.Status == statusExecuting:
		a.mailOperators(ctx, "Plan approved: "+c.OwnerEmail, email{
			Heading: "Plan approved", Paras: []string{c.OwnerEmail + " approved the launch plan. The tasks are ready to work."},
			CTA: "Open campaign", URL: a.opsCampaignURL(c),
		})
		if founderAction {
			a.mailFounderAction(ctx, c)
		}
	case from == statusAwaitingApproval && c.Status == statusPlanning:
		paras := []string{c.OwnerEmail + " sent the launch plan back."}
		if note != "" {
			paras = append(paras, "Their note: "+note)
		}
		a.mailOperators(ctx, "Changes requested: "+c.OwnerEmail, email{
			Heading: "Changes requested", Paras: paras, CTA: "Open campaign", URL: a.opsCampaignURL(c),
		})
	case c.Status == statusDone:
		a.mailLobby(ctx, c.OwnerEmail, "Your launch report is ready", email{
			Preheader: "What was submitted, what went live and where your signups came from.",
			Heading:   "Your launch report is ready",
			Paras:     []string{"The report lists every destination with its outcome and evidence, the visits and signups we could attribute to each, and what we'd try next."},
			CTA:       "Read the report", URL: a.campaignURL(c) + "/report",
		})
	}
}

func (a *App) mailFounderAction(ctx context.Context, c *Campaign) {
	a.mailLobby(ctx, c.OwnerEmail, "Your launch needs a step from you", email{
		Preheader: "Some destinations need you to post from your own account.",
		Heading:   "A step for you",
		Paras: []string{
			"Some destinations in your plan have to be done from your own account. Each one has instructions and the text to use.",
			"When you've done one, mark it submitted and paste the link so we can track it.",
		},
		CTA: "See your tasks", URL: a.campaignURL(c),
	})
}
