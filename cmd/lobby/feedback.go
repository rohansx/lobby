package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"text/tabwriter"
)

type issue struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	State     string `json:"state"`
	Path      string `json:"path"`
	Reporters int64  `json:"reporters"`
}

const feedbackUsage = `usage:
  lobby feedback list [--state open|approved|fixed|dismissed]
  lobby feedback approve ID
  lobby feedback next [--format md|json]   the most reported approved issue, as a work packet
  lobby feedback packet ID [--format md|json]
  lobby feedback note ID "what changed"
  lobby feedback fix ID                    mark fixed and email the reporters (secret key only)`

func runFeedback(c *client, tw *tabwriter.Writer, args []string) error {
	if len(args) == 0 {
		return errors.New(feedbackUsage)
	}
	sub, args := args[0], args[1:]
	setState := func(state string) error {
		if len(args) != 1 {
			return errors.New(feedbackUsage)
		}
		var is issue
		if err := c.do("POST", "/v1/issues/"+url.PathEscape(args[0])+"/state", map[string]string{"state": state}, &is); err != nil {
			return err
		}
		fmt.Printf("#%d is now %s\n", is.ID, is.State)
		return nil
	}
	packet := func(id string, rest []string) error {
		format := "md"
		if len(rest) == 2 && rest[0] == "--format" {
			format = rest[1]
		} else if len(rest) != 0 {
			return errors.New(feedbackUsage)
		}
		if format != "md" && format != "json" {
			return errors.New("--format must be md or json")
		}
		return c.do("GET", "/v1/issues/"+url.PathEscape(id)+"/packet?format="+format, nil, os.Stdout)
	}
	switch sub {
	case "list", "ls":
		path := "/v1/issues"
		if len(args) == 2 && args[0] == "--state" {
			path += "?state=" + url.QueryEscape(args[1])
		} else if len(args) != 0 {
			return errors.New(feedbackUsage)
		}
		var res struct {
			Data []issue `json:"data"`
		}
		if err := c.do("GET", path, nil, &res); err != nil {
			return err
		}
		fmt.Fprintln(tw, "ID\tREPORTERS\tSTATE\tTYPE\tTITLE")
		for _, is := range res.Data {
			fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", is.ID, is.Reporters, is.State, is.Type, is.Title)
		}
		return nil
	case "approve":
		return setState("approved")
	case "fix":
		return setState("fixed")
	case "dismiss":
		return setState("dismissed")
	case "reopen":
		return setState("open")
	case "next":
		var res struct {
			Data []issue `json:"data"`
		}
		if err := c.do("GET", "/v1/issues?state=approved", nil, &res); err != nil {
			return err
		}
		if len(res.Data) == 0 {
			fmt.Fprintln(os.Stderr, "no approved issues; approve one in the dashboard or with lobby feedback approve ID")
			return nil
		}
		return packet(fmt.Sprint(res.Data[0].ID), args)
	case "packet":
		if len(args) == 0 {
			return errors.New(feedbackUsage)
		}
		return packet(args[0], args[1:])
	case "note":
		if len(args) != 2 {
			return errors.New(feedbackUsage)
		}
		var is issue
		if err := c.do("POST", "/v1/issues/"+url.PathEscape(args[0])+"/note", map[string]string{"note": args[1]}, &is); err != nil {
			return err
		}
		fmt.Printf("noted on #%d\n", is.ID)
		return nil
	}
	return errors.New(feedbackUsage)
}
