//go:build live

package news

import (
	"context"
	"testing"
	"time"
)

func TestLiveRSSSmoke(t *testing.T) {
	urls := []string{
		"https://www.vedomosti.ru/rss/news",
		"https://www.finmarket.ru/rss/mainnews.asp",
		"https://www.moex.com/export/news.aspx?cat=101",
	}
	client := NewHTTPClient()
	for _, url := range urls {
		t.Run(url, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			items, err := Fetch(ctx, client, url)
			if err != nil || len(items) == 0 {
				t.Fatalf("items=%d err=%v", len(items), err)
			}
		})
	}
}
