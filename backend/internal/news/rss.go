package news

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// Item — запись ленты после разбора (RSS 2.0 или Atom).
type Item struct {
	Title       string
	Link        string
	GUID        string
	Description string
	Categories  []string
	Published   time.Time
}

type rssDoc struct {
	Channel struct {
		Items []struct {
			Title       string   `xml:"title"`
			Link        string   `xml:"link"`
			GUID        string   `xml:"guid"`
			Description string   `xml:"description"`
			Categories  []string `xml:"category"`
			PubDate     string   `xml:"pubDate"`
			Date        string   `xml:"http://purl.org/dc/elements/1.1/ date"`
		} `xml:"item"`
	} `xml:"channel"`
}

type atomDoc struct {
	Entries []struct {
		Title   string `xml:"title"`
		ID      string `xml:"id"`
		Summary string `xml:"summary"`
		Content string `xml:"content"`
		Links   []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
		Categories []struct {
			Term string `xml:"term,attr"`
		} `xml:"category"`
		Updated   string `xml:"updated"`
		Published string `xml:"published"`
	} `xml:"entry"`
}

var dateLayouts = []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 -0700", time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05"}

func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "windows-1251", "cp1251":
		return charmap.Windows1251.NewDecoder().Reader(in), nil
	case "koi8-r":
		return charmap.KOI8R.NewDecoder().Reader(in), nil
	case "utf-8", "utf8", "":
		return in, nil
	}
	return nil, fmt.Errorf("неподдерживаемая кодировка %q", label)
}

// Parse разбирает RSS 2.0 или Atom. Записи без заголовка, ссылки или даты отбрасываются.
func Parse(data []byte) ([]Item, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	dec.Strict = false
	var rss rssDoc
	if err := dec.Decode(&rss); err == nil && len(rss.Channel.Items) > 0 {
		var out []Item
		for _, it := range rss.Channel.Items {
			pub, ok := ParseDate(it.PubDate)
			if !ok {
				pub, ok = ParseDate(it.Date)
			}
			link := strings.TrimSpace(it.Link)
			if !ok || link == "" || strings.TrimSpace(it.Title) == "" {
				continue
			}
			out = append(out, Item{Title: CleanText(it.Title), Link: link, GUID: strings.TrimSpace(it.GUID),
				Description: CleanText(it.Description), Categories: trimAll(it.Categories), Published: pub})
		}
		return out, nil
	}
	dec = xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	dec.Strict = false
	var atom atomDoc
	if err := dec.Decode(&atom); err != nil {
		return nil, fmt.Errorf("не удалось разобрать ленту: %w", err)
	}
	var out []Item
	for _, e := range atom.Entries {
		link := ""
		for _, l := range e.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		pub, ok := ParseDate(e.Published)
		if !ok {
			pub, ok = ParseDate(e.Updated)
		}
		if !ok || link == "" || strings.TrimSpace(e.Title) == "" {
			continue
		}
		var cats []string
		for _, c := range e.Categories {
			cats = append(cats, c.Term)
		}
		desc := e.Summary
		if desc == "" {
			desc = e.Content
		}
		out = append(out, Item{Title: CleanText(e.Title), Link: link, GUID: e.ID, Description: CleanText(desc),
			Categories: trimAll(cats), Published: pub})
	}
	return out, nil
}

func trimAll(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(CleanText(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// MatchesCategories — пустой фильтр пропускает всё.
func MatchesCategories(item Item, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, c := range item.Categories {
		for _, f := range filter {
			if categoryMatches(c, f) {
				return true
			}
		}
	}
	return false
}

// categoryMatches accepts an exact category and its nested RSS categories:
// a filter "Экономика" matches "Экономика / Налоги и сборы", but not a
// different category that merely starts with the same letters.
func categoryMatches(category, filter string) bool {
	category = strings.ToLower(strings.TrimSpace(category))
	filter = strings.ToLower(strings.TrimSpace(filter))
	if category == filter {
		return true
	}
	if filter == "" || !strings.HasPrefix(category, filter) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(category, filter))
	return strings.HasPrefix(rest, "/")
}

const userAgent = "Mozilla/5.0 (compatible; AlfaInsightRSS/0.1; hackathon prototype)"
const maxFeedBytes = 10 << 20 // MOEX embeds tables in RSS descriptions; its official feed is currently ~7 MiB.

// NewHTTPClient allows for slower TLS handshakes of some publisher feeds while
// retaining finite limits for every network operation.
func NewHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 25 * time.Second
	transport.ResponseHeaderTimeout = 35 * time.Second
	return &http.Client{Transport: transport, Timeout: 45 * time.Second}
}

// Fetch загружает ленту с тайм-аутом и ограничением размера.
func Fetch(ctx context.Context, client *http.Client, url string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFeedBytes {
		return nil, fmt.Errorf("RSS-лента превышает лимит %d МБ", maxFeedBytes>>20)
	}
	return Parse(data)
}
