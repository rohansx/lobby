package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"text/tabwriter"
)

// runClaim: lobby claim create|list|revoke.
func runClaim(c *client, tw *tabwriter.Writer, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lobby claim create --ref ID [--email E] [--label L] | list | revoke ID")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("claim create", flag.ExitOnError)
		ref := fs.String("ref", "", "your product's id for the account (required)")
		email := fs.String("email", "", "only this address can claim it")
		label := fs.String("label", "", "who it's for, shown on the claim page")
		days := fs.Int("days", 0, "days until the link expires (default 30)")
		fs.Parse(args[1:])
		if *ref == "" {
			return errors.New("--ref is required")
		}
		var out struct {
			ID       int64  `json:"id"`
			ClaimURL string `json:"claim_url"`
		}
		body := map[string]any{"account_ref": *ref, "email": *email, "label": *label, "expires_in": *days * 86400}
		if err := c.do("POST", "/v1/claims", body, &out); err != nil {
			return err
		}
		fmt.Printf("claim %d: %s\n", out.ID, out.ClaimURL)
	case "list":
		var res struct {
			Data []struct {
				ID         int64  `json:"id"`
				AccountRef string `json:"account_ref"`
				Label      string `json:"label"`
				Email      string `json:"email"`
				State      string `json:"state"`
			} `json:"data"`
		}
		if err := c.do("GET", "/v1/claims", nil, &res); err != nil {
			return err
		}
		for _, cl := range res.Data {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", cl.ID, cl.AccountRef, cl.Label, cl.State, cl.Email)
		}
	case "revoke":
		if len(args) != 2 {
			return errors.New("usage: lobby claim revoke ID")
		}
		if err := c.do("DELETE", "/v1/claims/"+url.PathEscape(args[1]), nil, nil); err != nil {
			return err
		}
		fmt.Println("revoked")
	default:
		return fmt.Errorf("unknown claim command %q", args[0])
	}
	return nil
}
