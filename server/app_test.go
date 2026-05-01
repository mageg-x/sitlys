package main

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeEventType(t *testing.T) {
	t.Run("revenue", func(t *testing.T) {
		got := normalizeEventType(eventPayload{Revenue: &RevenueInput{Amount: 10}}, "")
		if got != "revenue" {
			t.Fatalf("expected revenue, got %s", got)
		}
	})

	t.Run("event", func(t *testing.T) {
		got := normalizeEventType(eventPayload{Name: "signup"}, "")
		if got != "event" {
			t.Fatalf("expected event, got %s", got)
		}
	})

	t.Run("pixel", func(t *testing.T) {
		got := normalizeEventType(eventPayload{}, "pixel-id")
		if got != "pixel" {
			t.Fatalf("expected pixel, got %s", got)
		}
	})

	t.Run("pageview", func(t *testing.T) {
		got := normalizeEventType(eventPayload{}, "")
		if got != "pageview" {
			t.Fatalf("expected pageview, got %s", got)
		}
	})
}

func TestIsBotRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Purpose", "prefetch")
	if !isBotRequest(req) {
		t.Fatal("expected prefetch request to be treated as bot-like traffic")
	}
}

func TestAllowRateLimit(t *testing.T) {
	app := &App{
		rateBuckets: map[string]rateBucket{},
	}
	key := "test"
	if !app.allowRateLimit(key, 2, 1, time.Minute) {
		t.Fatal("first request should pass")
	}
	if !app.allowRateLimit(key, 2, 1, time.Minute) {
		t.Fatal("second request should pass")
	}
	if app.allowRateLimit(key, 2, 1, time.Minute) {
		t.Fatal("third request should be blocked")
	}
}

func TestDetectGeo(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("CF-IPCountry", "US")
	req.Header.Set("CF-Region-Code", "CA")
	req.Header.Set("CF-IPCity", "San Francisco")
	country, region, city := detectGeo(req, eventPayload{})
	if country != "US" || region != "CA" || city != "San Francisco" {
		t.Fatalf("unexpected geo fallback: %q %q %q", country, region, city)
	}
}
