package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type client struct {
	base, key string
	http      *http.Client
}

func newClient() (*client, error) {
	base := strings.TrimRight(os.Getenv("LOBBY_URL"), "/")
	key := os.Getenv("LOBBY_KEY")
	if base == "" || key == "" {
		return nil, errors.New("set LOBBY_URL (e.g. https://wait.example.com) and LOBBY_KEY (the project's sk_ secret key)")
	}
	return &client{base: base, key: key, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

func (c *client) do(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(b, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s (HTTP %d)", e.Error, resp.StatusCode)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if w, ok := out.(io.Writer); ok {
		_, err = io.Copy(w, resp.Body)
		return err
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

type signup struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Status     string `json:"status"`
	Points     int64  `json:"points"`
	UTMSource  string `json:"utm_source"`
	CreatedAt  int64  `json:"created_at"`
	StatusURL  string `json:"status_url"`
	ReferredBy *int64 `json:"referred_by"`
}

func runCLI(cmd string, args []string) error {
	c, err := newClient()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	defer tw.Flush()

	switch cmd {
	case "stats":
		var st struct {
			Total       int64            `json:"total"`
			ByStatus    map[string]int64 `json:"by_status"`
			ConfirmRate float64          `json:"confirm_rate"`
			Referred    int64            `json:"referred"`
			TopSources  []struct {
				Name  string `json:"name"`
				Count int64  `json:"count"`
			} `json:"top_sources"`
		}
		if err := c.do("GET", "/v1/stats", nil, &st); err != nil {
			return err
		}
		fmt.Fprintf(tw, "total\t%d\n", st.Total)
		for _, s := range []string{"pending", "confirmed", "invited", "joined", "removed"} {
			fmt.Fprintf(tw, "%s\t%d\n", s, st.ByStatus[s])
		}
		fmt.Fprintf(tw, "confirm rate\t%.0f%%\n", st.ConfirmRate*100)
		fmt.Fprintf(tw, "via referral\t%d\n", st.Referred)
		if len(st.TopSources) > 0 {
			fmt.Fprintln(tw, "\ntop sources\t")
			for _, s := range st.TopSources {
				fmt.Fprintf(tw, "  %s\t%d\n", s.Name, s.Count)
			}
		}
	case "signups", "list":
		status := fs.String("status", "", "filter by status")
		query := fs.String("q", "", "search")
		limit := fs.Int("limit", 50, "max rows")
		asJSON := fs.Bool("json", false, "print JSON")
		fs.Parse(args)
		v := url.Values{"status": {*status}, "q": {*query}, "limit": {strconv.Itoa(*limit)}}
		if *asJSON {
			return c.do("GET", "/v1/signups?"+v.Encode(), nil, os.Stdout)
		}
		var res struct {
			Data []signup `json:"data"`
		}
		if err := c.do("GET", "/v1/signups?"+v.Encode(), nil, &res); err != nil {
			return err
		}
		fmt.Fprintln(tw, "ID\tEMAIL\tSTATUS\tSOURCE\tSIGNED UP")
		for _, s := range res.Data {
			src := s.UTMSource
			if src == "" && s.ReferredBy != nil {
				src = "referral"
			}
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", s.ID, s.Email, s.Status, src, time.Unix(s.CreatedAt, 0).Format("2006-01-02 15:04"))
		}
	case "invite":
		next := fs.Int("next", 0, "invite the next N people in line")
		ids := fs.String("ids", "", "comma-separated signup ids")
		dry := fs.Bool("dry-run", false, "show who would be invited without sending")
		fs.Parse(args)
		if *dry {
			if *next <= 0 {
				return errors.New("--dry-run works with --next")
			}
			// The line order lives on the server, so a dry run only reports the count.
			var st struct {
				ByStatus map[string]int64 `json:"by_status"`
			}
			if err := c.do("GET", "/v1/stats", nil, &st); err != nil {
				return err
			}
			n := min(int64(*next), st.ByStatus["confirmed"])
			fmt.Printf("would invite %d of %d people waiting\n", n, st.ByStatus["confirmed"])
			return nil
		}
		body := map[string]any{}
		switch {
		case *ids != "":
			var list []int64
			for _, s := range strings.Split(*ids, ",") {
				id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
				if err != nil {
					return fmt.Errorf("bad id %q", s)
				}
				list = append(list, id)
			}
			body["ids"] = list
		case *next > 0:
			body["count"] = *next
		default:
			return errors.New("use --next N or --ids 1,2,3")
		}
		var res struct {
			Invited int      `json:"invited"`
			Data    []signup `json:"data"`
		}
		if err := c.do("POST", "/v1/invites", body, &res); err != nil {
			return err
		}
		for _, s := range res.Data {
			fmt.Fprintf(tw, "%d\t%s\n", s.ID, s.Email)
		}
		fmt.Fprintf(tw, "invited %d\n", res.Invited)
	case "joined":
		if len(args) != 1 {
			return errors.New("usage: lobby joined ID")
		}
		var s signup
		if err := c.do("POST", "/v1/signups/"+url.PathEscape(args[0])+"/joined", nil, &s); err != nil {
			return err
		}
		fmt.Printf("%d %s is now %s\n", s.ID, s.Email, s.Status)
	case "remove":
		hard := fs.Bool("hard", false, "delete permanently and block the address")
		fs.Parse(args)
		if fs.NArg() != 1 {
			return errors.New("usage: lobby remove [--hard] ID")
		}
		path := "/v1/signups/" + url.PathEscape(fs.Arg(0))
		if *hard {
			path += "?hard=1"
		}
		if err := c.do("DELETE", path, nil, nil); err != nil {
			return err
		}
		fmt.Println("removed")
	case "export":
		format := fs.String("format", "csv", "csv or ndjson")
		status := fs.String("status", "", "status filter, or all")
		fs.Parse(args)
		if *format != "csv" && *format != "ndjson" {
			return errors.New("--format must be csv or ndjson")
		}
		path := "/v1/export." + *format
		if *status != "" {
			path += "?status=" + url.QueryEscape(*status)
		}
		return c.do("GET", path, nil, os.Stdout)
	case "webhooks":
		return c.do("GET", "/v1/webhooks", nil, os.Stdout)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}
