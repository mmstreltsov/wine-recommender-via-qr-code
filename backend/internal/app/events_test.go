package app

import (
	"context"
	"errors"
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

func TestAN01_TaxonomyAcceptsKnownClientNames(t *testing.T) {
	svc := NewEventsService(&fakeEventStore{})
	for _, name := range allClientEventNames {
		if err := svc.RecordClientEvent(context.Background(), "s1", nil, name, map[string]any{}); err != nil {
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

func TestNFR04_PayloadMustBeFlatScalars(t *testing.T) {
	svc := NewEventsService(&fakeEventStore{})
	valid := []map[string]any{
		{},
		nil,
		{"category_id": float64(3)},
		{"item_id": float64(1), "source": "menu"},
		{"flag": true, "note": "x", "empty": nil},
	}
	for i, p := range valid {
		if err := svc.RecordClientEvent(context.Background(), "s1", nil, "category_select", p); err != nil {
			t.Fatalf("valid payload %d rejected: %v", i, err)
		}
	}
	invalid := []map[string]any{
		{"nested": map[string]any{"a": 1}},
		{"list": []any{1, 2}},
		{"deep": []map[string]any{}},
	}
	for i, p := range invalid {
		err := svc.RecordClientEvent(context.Background(), "s1", nil, "category_select", p)
		var ue *UserError
		if !errors.As(err, &ue) || ue.Code != CodeBadRequest {
			t.Fatalf("invalid payload %d must be rejected with bad_request, got %v", i, err)
		}
	}
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
	if err := svc.RecordClientEvent(context.Background(), "s1", nil, "menu_view", border); err != nil {
		t.Fatalf("borderline payload rejected: %v", err)
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
