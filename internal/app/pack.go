package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Selling the Launch Pack: a one-time Polar product (POLAR_PRODUCT_LAUNCH_PACK).
// The order.paid webhook opens a campaign through CreateCampaign, which also
// moves the account to the pack plan. Refunds only alert the operators; the
// campaign is left as it is for a person to decide.

var errPackBuyer = errors.New("This account can't buy a Launch Pack. Contact support.")

// PackCheckout returns a Polar checkout URL for one Launch Pack.
func (a *App) PackCheckout(ctx context.Context, ac *Account) (string, error) {
	if !a.cfg.Polar.Enabled() {
		return "", errBillingOff
	}
	if ac.Suspended {
		return "", errPackBuyer
	}
	var out struct {
		URL string `json:"url"`
	}
	err := a.polar(ctx, "/v1/checkouts/", map[string]any{
		"products":             []string{a.cfg.Polar.LaunchPack},
		"customer_email":       ac.Email,
		"external_customer_id": externalID(ac.ID),
		"success_url":          a.cfg.BaseURL + "/campaigns?checkout={CHECKOUT_ID}",
		"return_url":           a.cfg.BaseURL + "/campaigns",
		"metadata":             map[string]string{"account_id": strconv.FormatInt(ac.ID, 10)},
	}, &out)
	return out.URL, err
}

type polarOrder struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"` // pending | paid | refunded | partially_refunded
	Paid       bool           `json:"paid"`
	ProductID  string         `json:"product_id"`
	CustomerID string         `json:"customer_id"`
	NetAmount  int64          `json:"net_amount"` // cents, after discounts, before tax
	Currency   string         `json:"currency"`
	Metadata   map[string]any `json:"metadata"`
	Customer   struct {
		ExternalID *string `json:"external_id"`
	} `json:"customer"`
}

// ApplyOrder opens a campaign for a paid Launch Pack order. Orders for any
// other product (subscription renewals, say) are ignored. Replays are safe:
// CreateCampaign returns the campaign it already made for the order id.
func (a *App) ApplyOrder(ctx context.Context, o *polarOrder) error {
	if o.ProductID != a.cfg.Polar.LaunchPack || a.cfg.Polar.LaunchPack == "" {
		return nil
	}
	// Status, not the paid flag: Polar keeps paid=true after a refund.
	if o.Status != "paid" && o.Status != "partially_refunded" {
		a.log.Warn("polar: launch pack order not paid", "order", o.ID, "status", o.Status)
		return nil
	}
	id := a.polarAccount(ctx, o.Customer.ExternalID, o.Metadata, o.CustomerID)
	if _, err := a.Account(ctx, id); err != nil {
		// Paid but nobody to give it to: a person has to sort this out.
		a.log.Error("polar: launch pack paid for unknown account", "order", o.ID, "customer", o.CustomerID)
		a.mailOperators(ctx, "Launch Pack paid, no matching account", email{
			Heading: "A Launch Pack order has no account",
			Paras:   []string{fmt.Sprintf("Polar order %s (customer %s) was paid, but it doesn't match a Lobby account. No campaign was opened. Find the buyer in Polar and open a manual campaign, or refund.", o.ID, o.CustomerID)},
		})
		return nil
	}
	c, err := a.CreateCampaign(ctx, id, o.NetAmount, o.ID)
	if err != nil {
		return err
	}
	if o.CustomerID != "" {
		a.db.W.ExecContext(ctx, `UPDATE admins SET polar_customer_id = ? WHERE id = ? AND polar_customer_id = ''`, o.CustomerID, id)
	}
	a.log.Info("billing: launch pack paid", "account", id, "order", o.ID, "campaign", c.ID)
	return nil
}

// refundedOrder tells the operators. Nothing is deleted or downgraded.
func (a *App) refundedOrder(ctx context.Context, o *polarOrder) {
	id := a.polarAccount(ctx, o.Customer.ExternalID, o.Metadata, o.CustomerID)
	who := "no matching account"
	if ac, err := a.Account(ctx, id); err == nil {
		who = ac.Email
	}
	what := "a subscription order"
	if o.ProductID != "" && o.ProductID == a.cfg.Polar.LaunchPack {
		what = "a Launch Pack"
	}
	a.log.Warn("billing: order refunded", "order", o.ID, "account", id, "product", o.ProductID, "status", o.Status)
	a.mailOperators(ctx, "Refund: "+who, email{
		Heading: "An order was refunded",
		Paras: []string{
			fmt.Sprintf("Polar order %s for %s (%s) was refunded (status %s).", o.ID, what, who, o.Status),
			"Lobby hasn't changed anything: the campaign and the account's plan stay as they are. Cancel the campaign or change the plan from the operator page if that's the right call.",
		},
		CTA: "Operator page", URL: a.cfg.BaseURL + "/ops",
	})
}

// polarOrderEvent handles order.paid and order.refunded. Like subscriptions,
// it acts on the order as Polar reports it now, not as the event body says.
// It returns an HTTP status to fail the webhook with, or 0 when done.
func (s *Server) polarOrderEvent(ctx context.Context, typ string, data json.RawMessage) int {
	var o polarOrder
	if err := json.Unmarshal(data, &o); err != nil || o.ID == "" {
		return http.StatusBadRequest
	}
	if s.app.cfg.Polar.Token != "" {
		var fresh polarOrder
		if err := s.app.polar(ctx, "/v1/orders/"+url.PathEscape(o.ID), nil, &fresh); err != nil {
			s.app.log.Error("polar webhook: fetch order", "err", err)
			return http.StatusBadGateway
		}
		o = fresh
	}
	if typ == "order.refunded" {
		s.app.refundedOrder(ctx, &o)
		return 0
	}
	if err := s.app.ApplyOrder(ctx, &o); err != nil {
		s.app.log.Error("polar webhook", "type", typ, "err", err)
		return http.StatusInternalServerError
	}
	return 0
}

func (s *Server) adminPackCheckout(w http.ResponseWriter, r *http.Request) {
	ac, err := s.app.Account(r.Context(), adminID(r))
	if err != nil {
		jsonError(w, 404, "not found")
		return
	}
	u, err := s.app.PackCheckout(r.Context(), ac)
	if err != nil {
		s.app.log.Error("pack checkout", "err", err)
		jsonError(w, 400, userMessage(err))
		return
	}
	writeJSON(w, 200, map[string]any{"url": u})
}
