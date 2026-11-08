package app

import (
	"context"
	"errors"
	"maps"
	"math"
	"strings"
	"testing"

	"svoyshef/internal/domain"
	"svoyshef/internal/store"
)

type fakeEventStore struct {
	events []domain.Event
}

func (f *fakeEventStore) Insert(ctx context.Context, e domain.Event) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeEventStore) InsertBatch(ctx context.Context, events []domain.Event) error {
	f.events = append(f.events, events...)
	return nil
}

var allClientEventNames = []string{
	"page_open", "menu_view", "category_select", "dish_card_open",
	"rec_feed_visible", "rec_impression", "cart_open", "pay_click", "stub_view",
}

var allServerEventNames = []string{
	"menu_add_click", "rec_add_click", "cart_qty_change", "cart_item_remove",
}

var validClientPayloads = map[string]map[string]any{
	"page_open": {}, "menu_view": {}, "stub_view": {},
	"category_select":  {"category_id": float64(0)},
	"dish_card_open":   {"item_id": float64(1), "source": "menu"},
	"rec_feed_visible": {"dish_id": float64(1), "recs_count": float64(13)},
	"rec_impression":   {"context": "dish", "rec_item_id": float64(999999), "rec_source": "expert"},
	"cart_open":        {"lines_count": float64(0), "total": float64(0)},
	"pay_click":        {"lines_count": float64(1), "total": float64(100)},
}

func TestAN01_TaxonomyAcceptsKnownClientNames(t *testing.T) {
	svc := NewEventsService(&fakeEventStore{})
	for _, name := range allClientEventNames {
		if err := svc.RecordClientEvent(context.Background(), "s1", nil, name, validClientPayloads[name]); err != nil {
			t.Fatalf("client name %q must be accepted: %v", name, err)
		}
	}
	if len(clientEventNames) != len(allClientEventNames) {
		t.Fatalf("client taxonomy size mismatch: map=%d list=%d", len(clientEventNames), len(allClientEventNames))
	}
}

// AN-01 + events.md §1: серверные имена таксономии от клиента → 400 bad_request
// (ровно один источник; подделка метрик исследования запрещена).
func TestAN01_ServerNamesRejectedFromClient(t *testing.T) {
	for _, name := range allServerEventNames {
		st := &fakeEventStore{}
		svc := NewEventsService(st)
		err := svc.RecordClientEvent(context.Background(), "s1", nil, name, map[string]any{})
		var ue *UserError
		if !errors.As(err, &ue) || ue.Code != CodeBadRequest {
			t.Fatalf("server name %q must be rejected with bad_request, got %v", name, err)
		}
		if len(st.events) != 0 {
			t.Fatalf("server name %q must not be stored", name)
		}
	}
	if len(serverEventNames) != len(allServerEventNames) {
		t.Fatalf("server taxonomy size mismatch: map=%d list=%d", len(serverEventNames), len(allServerEventNames))
	}
}

func TestAN01_UnknownEventNameRejected(t *testing.T) {
	svc := NewEventsService(&fakeEventStore{})
	for _, name := range []string{"", "click", "Menu_View", "menu_view ", "custom_event"} {
		err := svc.RecordClientEvent(context.Background(), "s1", nil, name, map[string]any{})
		var ue *UserError
		if !errors.As(err, &ue) || ue.Code != CodeBadRequest {
			t.Fatalf("name %q must be rejected with bad_request, got %v", name, err)
		}
	}
}

func TestAN05_PayloadWhitelistAndRequiredFields(t *testing.T) {
	for name, valid := range validClientPayloads {
		t.Run(name, func(t *testing.T) {
			unexpected := maps.Clone(valid)
			unexpected["note"] = "synthetic free text"
			assertClientPayloadRejected(t, name, unexpected)
			for field := range valid {
				missing := maps.Clone(valid)
				delete(missing, field)
				assertClientPayloadRejected(t, name, missing)
				for _, invalid := range []any{nil, true, "synthetic free text", map[string]any{"nested": "text"}, []any{1}} {
					payload := maps.Clone(valid)
					payload[field] = invalid
					assertClientPayloadRejected(t, name, payload)
				}
			}
		})
	}
}

func assertClientPayloadRejected(t *testing.T, name string, payload map[string]any) {
	t.Helper()
	st := &fakeEventStore{}
	err := NewEventsService(st).RecordClientEvent(context.Background(), "s1", nil, name, payload)
	var ue *UserError
	if !errors.As(err, &ue) || ue.Code != CodeBadRequest || ue.Message != msgInvalidPayload {
		t.Fatalf("%s: expected generic bad_request, got %v", name, err)
	}
	if len(st.events) != 0 {
		t.Fatal("invalid event must not be stored")
	}
}

func TestNFR04_EventIntegerBounds(t *testing.T) {
	for name, valid := range validClientPayloads {
		for field, value := range valid {
			if _, numeric := value.(float64); !numeric {
				continue
			}
			t.Run(name+"/"+field, func(t *testing.T) {
				min, max := float64(0), float64(maxEventInteger)
				if field == "item_id" || field == "dish_id" || field == "rec_item_id" {
					min = 1
				}
				if field == "recs_count" {
					max = 13
				}
				for _, invalid := range []float64{-1, min - 1, 0.5, max + 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
					payload := maps.Clone(valid)
					payload[field] = invalid
					assertClientPayloadRejected(t, name, payload)
				}
				for _, boundary := range []float64{min, max} {
					payload := maps.Clone(valid)
					payload[field] = boundary
					if err := NewEventsService(&fakeEventStore{}).RecordClientEvent(context.Background(), "s1", nil, name, payload); err != nil {
						t.Fatalf("valid boundary rejected: %v", err)
					}
				}
			})
		}
	}
}

func TestAN05_EventEnumsExcludeFreeText(t *testing.T) {
	for _, contextValue := range []string{"dish", "cart"} {
		for _, source := range []string{"expert", "upsell", "heuristic"} {
			payload := maps.Clone(validClientPayloads["rec_impression"])
			payload["context"], payload["rec_source"] = contextValue, source
			if err := NewEventsService(&fakeEventStore{}).RecordClientEvent(context.Background(), "s1", nil, "rec_impression", payload); err != nil {
				t.Fatalf("valid enum rejected: %v", err)
			}
		}
	}
	for _, field := range []string{"context", "rec_source"} {
		for _, invalid := range []any{"", "menu", "Expert", float64(1)} {
			payload := maps.Clone(validClientPayloads["rec_impression"])
			payload[field] = invalid
			assertClientPayloadRejected(t, "rec_impression", payload)
		}
	}
	payload := maps.Clone(validClientPayloads["dish_card_open"])
	payload["source"] = "rec_feed"
	assertClientPayloadRejected(t, "dish_card_open", payload)
}

func TestNFR04_PayloadSizeLimit1KB(t *testing.T) {
	svc := NewEventsService(&fakeEventStore{})
	big := map[string]any{"blob": strings.Repeat("a", 1100)}
	err := svc.RecordClientEvent(context.Background(), "s1", nil, "menu_view", big)
	var ue *UserError
	if !errors.As(err, &ue) || ue.Code != CodeBadRequest {
		t.Fatalf("oversized payload must be rejected with bad_request, got %v", err)
	}
	border := map[string]any{"blob": strings.Repeat("a", 1000)}
	raw := len(`{"blob":"`) + 1000 + len(`"}`)
	if raw > maxPayloadBytes {
		t.Fatalf("test fixture must fit limit: %d", raw)
	}
	assertClientPayloadRejected(t, "menu_view", border)
	if err := svc.RecordClientEvent(context.Background(), "s1", nil, "pay_click", validClientPayloads["pay_click"]); err != nil {
		t.Fatalf("valid small payload rejected: %v", err)
	}
}

func TestAN03_ClientEventStoredWithSessionAndTable(t *testing.T) {
	st := &fakeEventStore{}
	svc := NewEventsService(st)
	table := 7
	if err := svc.RecordClientEvent(context.Background(), "sid", &table, "page_open", nil); err != nil {
		t.Fatal(err)
	}
	if len(st.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(st.events))
	}
	e := st.events[0]
	if e.SessionID != "sid" || e.TableID == nil || *e.TableID != 7 || e.Name != "page_open" {
		t.Fatalf("event fields wrong: %+v", e)
	}
}

func TestAN03_ServerEventNotValidatedAgainstTaxonomy(t *testing.T) {
	st := &fakeEventStore{}
	svc := NewEventsService(st)
	if err := svc.RecordServerEvent(context.Background(), "sid", nil, "menu_add_click", map[string]any{"item_id": int64(1)}); err != nil {
		t.Fatal(err)
	}
	if len(st.events) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(st.events))
	}
}

var _ store.EventStore = (*fakeEventStore)(nil)
