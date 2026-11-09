package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"svoyshef/internal/domain"
	"svoyshef/internal/store"
)

type fakeCartStore struct {
	lines  []domain.CartLine
	calls  []string
	addErr error
}

func (f *fakeCartStore) Lines(ctx context.Context, sessionID string) ([]domain.CartLine, error) {
	f.calls = append(f.calls, "lines")
	out := make([]domain.CartLine, len(f.lines))
	copy(out, f.lines)
	return out, nil
}

func (f *fakeCartStore) AddItem(ctx context.Context, sessionID string, itemID int64, addedFrom string) (int, error) {
	f.calls = append(f.calls, "add")
	if f.addErr != nil {
		return 0, f.addErr
	}
	for i := range f.lines {
		if f.lines[i].ItemID == itemID {
			if f.lines[i].Qty >= 20 {
				return 0, store.ErrQtyLimit
			}
			f.lines[i].Qty++
			return f.lines[i].Qty, nil
		}
	}
	f.lines = append(f.lines, domain.CartLine{ItemID: itemID, Qty: 1, AddedFrom: addedFrom, Available: true})
	return 1, nil
}

func (f *fakeCartStore) SetQty(ctx context.Context, sessionID string, itemID int64, qty int) (bool, int, error) {
	f.calls = append(f.calls, "setqty")
	for i := range f.lines {
		if f.lines[i].ItemID == itemID {
			prev := f.lines[i].Qty
			f.lines[i].Qty = qty
			return true, prev, nil
		}
	}
	return false, 0, nil
}

func (f *fakeCartStore) RemoveItem(ctx context.Context, sessionID string, itemID int64) (bool, error) {
	f.calls = append(f.calls, "remove")
	for i := range f.lines {
		if f.lines[i].ItemID == itemID {
			f.lines = append(f.lines[:i], f.lines[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

type fakeEventRecorder struct {
	events []domain.Event
}

func (f *fakeEventRecorder) RecordServerEvent(ctx context.Context, sessionID string, tableID *int, name string, payload map[string]any) error {
	f.events = append(f.events, domain.Event{SessionID: sessionID, TableID: tableID, Name: name, Payload: payload})
	return nil
}

type failingEventRecorder struct{ err error }

func (f failingEventRecorder) RecordServerEvent(ctx context.Context, sessionID string, tableID *int, name string, payload map[string]any) error {
	if f.err != nil {
		return f.err
	}
	return errors.New("events store down")
}

type fakeAvailability struct {
	off map[int64]struct{}
}

func (f *fakeAvailability) IsAvailable(ctx context.Context, itemID int64) bool {
	_, disabled := f.off[itemID]
	return !disabled
}

func (f *fakeAvailability) UnavailableIDs(ctx context.Context) map[int64]struct{} {
	cp := make(map[int64]struct{}, len(f.off))
	for id := range f.off {
		cp[id] = struct{}{}
	}
	return cp
}

func testMenu() *MenuService {
	cats := []domain.Category{{ID: 1, Name: "Салаты"}, {ID: 2, Name: "Супы"}}
	items := []domain.MenuItem{
		{ID: 101, CategoryID: 1, Name: "Салат", Price: 500, SortOrder: 0},
		{ID: 102, CategoryID: 1, Name: "Салат второй", Price: 600, SortOrder: 1},
		{ID: 201, CategoryID: 2, Name: "Суп", Price: 700, SortOrder: 0},
	}
	return NewMenuService(NewStaticMenu(cats, items), &fakeAvailability{off: map[int64]struct{}{}}, nil)
}

func newTestCart(menu *MenuService, avail AvailabilityChecker, carts store.CartStore, rec *fakeEventRecorder) *CartService {
	return NewCartService(menu, carts, rec, StubRecommender{}, avail)
}

func TestFR37_AddUnavailableItemReturns409WithItemID(t *testing.T) {
	avail := &fakeAvailability{off: map[int64]struct{}{101: {}}}
	menu2 := NewMenuService(NewStaticMenu([]domain.Category{{ID: 1, Name: "Салаты"}}, []domain.MenuItem{{ID: 101, CategoryID: 1, Name: "Салат", Price: 500}}), avail, nil)
	carts := &fakeCartStore{}
	rec := &fakeEventRecorder{}
	svc := newTestCart(menu2, avail, carts, rec)
	_, err := svc.Add(context.Background(), "s1", nil, 101, "menu")
	var ue *UserError
	if !errors.As(err, &ue) {
		t.Fatalf("expected UserError, got %v", err)
	}
	if ue.Code != CodeItemUnavailable {
		t.Fatalf("expected item_unavailable, got %s", ue.Code)
	}
	if ue.ItemID == nil || *ue.ItemID != 101 {
		t.Fatalf("expected item_id=101 in error, got %v", ue.ItemID)
	}
	if len(carts.calls) != 0 {
		t.Fatalf("store must not be called on unavailable item, calls=%v", carts.calls)
	}
	if len(rec.events) != 0 {
		t.Fatalf("no events expected on failure")
	}
}

func TestFR37_CartLineAvailableFalseWhenDisabledAfterAdd(t *testing.T) {
	menu := testMenu()
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	carts := &fakeCartStore{lines: []domain.CartLine{{ItemID: 101, Qty: 2, AddedFrom: "menu"}}}
	rec := &fakeEventRecorder{}
	svc := newTestCart(menu, avail, carts, rec)
	avail.off[101] = struct{}{}
	view, err := svc.View(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Lines) != 1 || view.Lines[0].Available {
		t.Fatalf("expected available=false, got %+v", view.Lines)
	}
	if view.Total != 1000 {
		t.Fatalf("expected total 1000, got %d", view.Total)
	}
	if view.Recommendation != nil {
		t.Fatalf("expected nil recommendation from stub, got %+v", view.Recommendation)
	}
}

func TestAN02_ServerWritesCartActionEvents(t *testing.T) {
	menu := testMenu()
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	carts := &fakeCartStore{}
	rec := &fakeEventRecorder{}
	svc := newTestCart(menu, avail, carts, rec)
	table := 12
	if _, err := svc.Add(context.Background(), "s1", &table, 101, "menu"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(context.Background(), "s1", &table, 201, "rec_feed"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(context.Background(), "s1", &table, 102, "rec_cart"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetQty(context.Background(), "s1", &table, 101, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Remove(context.Background(), "s1", &table, 201); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name    string
		payload map[string]any
	}{
		{"menu_add_click", map[string]any{"item_id": int64(101)}},
		{"rec_add_click", map[string]any{"item_id": int64(201), "context": "dish"}},
		{"rec_add_click", map[string]any{"item_id": int64(102), "context": "cart"}},
		{"cart_qty_change", map[string]any{"item_id": int64(101), "from": 1, "to": 3}},
		{"cart_item_remove", map[string]any{"item_id": int64(201)}},
	}
	if len(rec.events) != len(want) {
		t.Fatalf("expected %d events, got %d: %+v", len(want), len(rec.events), rec.events)
	}
	for i, w := range want {
		e := rec.events[i]
		if e.Name != w.name || e.SessionID != "s1" || e.TableID == nil || *e.TableID != 12 {
			t.Fatalf("event %d: got %+v", i, e)
		}
		for k, v := range w.payload {
			if e.Payload[k] != v {
				t.Fatalf("event %d payload[%s]: got %v want %v", i, k, e.Payload[k], v)
			}
		}
	}
}

func TestNFR04_CartValidations(t *testing.T) {
	ctx := context.Background()
	menu := testMenu()
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	carts := &fakeCartStore{lines: []domain.CartLine{{ItemID: 101, Qty: 20, AddedFrom: "menu"}}}
	rec := &fakeEventRecorder{}
	svc := newTestCart(menu, avail, carts, rec)

	cases := []struct {
		name string
		fn   func() error
		code string
	}{
		{"invalid source", func() error { _, err := svc.Add(ctx, "s", nil, 101, "banner"); return err }, CodeBadRequest},
		{"unknown item", func() error { _, err := svc.Add(ctx, "s", nil, 999, "menu"); return err }, CodeNotFound},
		{"qty over limit on add", func() error { _, err := svc.Add(ctx, "s", nil, 101, "menu"); return err }, CodeBadRequest},
		{"qty zero", func() error { _, err := svc.SetQty(ctx, "s", nil, 101, 0); return err }, CodeBadRequest},
		{"qty over limit", func() error { _, err := svc.SetQty(ctx, "s", nil, 101, 21); return err }, CodeBadRequest},
		{"patch missing line", func() error { _, err := svc.SetQty(ctx, "s", nil, 201, 5); return err }, CodeNotFound},
		{"delete missing line", func() error { _, err := svc.Remove(ctx, "s", nil, 201); return err }, CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			var ue *UserError
			if !errors.As(err, &ue) || ue.Code != tc.code {
				t.Fatalf("expected code %s, got %v", tc.code, err)
			}
		})
	}
}

func TestFR37_PatchDeleteUnavailableLineStillWorks(t *testing.T) {
	ctx := context.Background()
	menu := testMenu()
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	carts := &fakeCartStore{lines: []domain.CartLine{{ItemID: 101, Qty: 1, AddedFrom: "menu"}}}
	rec := &fakeEventRecorder{}
	svc := newTestCart(menu, avail, carts, rec)
	avail.off[101] = struct{}{}
	view, err := svc.SetQty(ctx, "s", nil, 101, 2)
	if err != nil {
		t.Fatalf("patch of unavailable line must succeed: %v", err)
	}
	if view.Lines[0].Qty != 2 {
		t.Fatalf("expected qty 2, got %d", view.Lines[0].Qty)
	}
	if _, err := svc.Remove(ctx, "s", nil, 101); err != nil {
		t.Fatalf("delete of unavailable line must succeed: %v", err)
	}
}

func TestAN03_CartViewDropsItemsMissingFromMenu(t *testing.T) {
	menu := testMenu()
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	carts := &fakeCartStore{lines: []domain.CartLine{
		{ItemID: 101, Qty: 1, AddedFrom: "menu"},
		{ItemID: 555, Qty: 5, AddedFrom: "menu"},
	}}
	rec := &fakeEventRecorder{}
	svc := newTestCart(menu, avail, carts, rec)
	view, err := svc.View(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Lines) != 1 || view.Lines[0].Item.ID != 101 {
		t.Fatalf("expected only existing item, got %+v", view.Lines)
	}
}

// AN-02/events.md §2: сбой записи события после успешной мутации не ломает UX —
// успешный view вместо 500.
func TestAN02_EventFailureDoesNotBreakCartMutation(t *testing.T) {
	ctx := context.Background()
	menu := testMenu()
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	carts := &fakeCartStore{lines: []domain.CartLine{{ItemID: 101, Qty: 1, AddedFrom: "menu"}}}
	svc := NewCartService(menu, carts, failingEventRecorder{}, StubRecommender{}, avail)

	view, err := svc.Add(ctx, "s1", nil, 201, "menu")
	if err != nil {
		t.Fatalf("add must succeed despite event failure: %v", err)
	}
	if len(view.Lines) != 2 {
		t.Fatalf("add result wrong: %+v", view.Lines)
	}
	view, err = svc.SetQty(ctx, "s1", nil, 101, 3)
	if err != nil {
		t.Fatalf("patch must succeed despite event failure: %v", err)
	}
	if view.Lines[0].Qty != 3 {
		t.Fatalf("patch result wrong: %+v", view.Lines)
	}
	view, err = svc.Remove(ctx, "s1", nil, 101)
	if err != nil {
		t.Fatalf("delete must succeed despite event failure: %v", err)
	}
	if len(view.Lines) != 1 || view.Lines[0].Item.ID != 201 {
		t.Fatalf("delete result wrong: %+v", view.Lines)
	}
}

func TestNFR04_CartAndAvailabilityLogsOmitCredentials(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	sessionID := rand.Text()
	driverData := rand.Text()
	err := &pgconn.PgError{Code: "23505", Message: sessionID + driverData, Detail: driverData}
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	carts := &fakeCartStore{lines: []domain.CartLine{{ItemID: 101, Qty: 1, AddedFrom: "menu"}}}
	svc := NewCartService(testMenu(), carts, failingEventRecorder{err: err}, StubRecommender{}, avail)
	ctx := context.Background()
	if _, err := svc.Add(ctx, sessionID, nil, 201, "menu"); err != nil {
		t.Fatal("add must succeed")
	}
	if _, err := svc.SetQty(ctx, sessionID, nil, 101, 2); err != nil {
		t.Fatal("set qty must succeed")
	}
	if _, err := svc.Remove(ctx, sessionID, nil, 101); err != nil {
		t.Fatal("remove must succeed")
	}
	cache := NewAvailabilityCache(&fakeAvailabilityStore{err: err}, time.Minute)
	cache.refresh()
	log.SetOutput(previous)
	out := logs.String()
	if strings.Contains(out, sessionID) || strings.Contains(out, driverData) {
		t.Fatal("log contains a session credential or raw database error")
	}
	for _, operation := range []string{"record_add_event", "record_cart_qty_change", "record_cart_item_remove", "availability_refresh"} {
		if !strings.Contains(out, "operation="+operation+" code=internal") {
			t.Errorf("missing safe log for operation %s", operation)
		}
	}
}
