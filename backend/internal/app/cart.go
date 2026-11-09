package app

import (
	"context"
	"errors"
	"log"

	"svoyshef/internal/domain"
	"svoyshef/internal/recommender"
	"svoyshef/internal/store"
)

type EventRecorder interface {
	RecordServerEvent(ctx context.Context, sessionID string, tableID *int, name string, payload map[string]any) error
}

type CartService struct {
	menu   *MenuService
	carts  store.CartStore
	events EventRecorder
	rec    recommender.Recommender
	avail  AvailabilityChecker
}

func NewCartService(menu *MenuService, carts store.CartStore, events EventRecorder, rec recommender.Recommender, avail AvailabilityChecker) *CartService {
	return &CartService{menu: menu, carts: carts, events: events, rec: rec, avail: avail}
}

var validSources = map[string]struct{}{
	"menu":     {},
	"rec_feed": {},
	"rec_cart": {},
}

func (s *CartService) Add(ctx context.Context, sessionID string, tableID *int, itemID int64, source string) (*CartView, error) {
	if _, ok := validSources[source]; !ok {
		return nil, badRequest(msgInvalidSource)
	}
	if _, ok := s.menu.Lookup(itemID); !ok {
		return nil, notFound(msgItemNotFound)
	}
	if !s.avail.IsAvailable(ctx, itemID) {
		return nil, itemUnavailable(itemID)
	}
	if _, err := s.carts.AddItem(ctx, sessionID, itemID, source); err != nil {
		if errors.Is(err, store.ErrQtyLimit) {
			return nil, badRequest(msgInvalidQty)
		}
		return nil, err
	}
	if err := s.recordAddEvent(ctx, sessionID, tableID, itemID, source); err != nil {
		log.Print("ERROR operation=record_add_event code=internal")
	}
	return s.View(ctx, sessionID)
}

func (s *CartService) recordAddEvent(ctx context.Context, sessionID string, tableID *int, itemID int64, source string) error {
	if source == "menu" {
		return s.events.RecordServerEvent(ctx, sessionID, tableID, "menu_add_click", map[string]any{"item_id": itemID})
	}
	contextValue := "dish"
	if source == "rec_cart" {
		contextValue = "cart"
	}
	return s.events.RecordServerEvent(ctx, sessionID, tableID, "rec_add_click", map[string]any{
		"item_id": itemID,
		"context": contextValue,
	})
}

func (s *CartService) SetQty(ctx context.Context, sessionID string, tableID *int, itemID int64, qty int) (*CartView, error) {
	if qty < 1 || qty > 20 {
		return nil, badRequest(msgInvalidQty)
	}
	found, prev, err := s.carts.SetQty(ctx, sessionID, itemID, qty)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, notFound(msgCartLineNotFound)
	}
	if err := s.events.RecordServerEvent(ctx, sessionID, tableID, "cart_qty_change", map[string]any{
		"item_id": itemID,
		"from":    prev,
		"to":      qty,
	}); err != nil {
		log.Print("ERROR operation=record_cart_qty_change code=internal")
	}
	return s.View(ctx, sessionID)
}

func (s *CartService) Remove(ctx context.Context, sessionID string, tableID *int, itemID int64) (*CartView, error) {
	found, err := s.carts.RemoveItem(ctx, sessionID, itemID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, notFound(msgCartLineNotFound)
	}
	if err := s.events.RecordServerEvent(ctx, sessionID, tableID, "cart_item_remove", map[string]any{
		"item_id": itemID,
	}); err != nil {
		log.Print("ERROR operation=record_cart_item_remove code=internal")
	}
	return s.View(ctx, sessionID)
}

func (s *CartService) View(ctx context.Context, sessionID string) (*CartView, error) {
	lines, err := s.carts.Lines(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	views := make([]CartLineView, 0, len(lines))
	exclude := make([]int64, 0, len(lines))
	var lastLines []domain.CartLine
	total := 0
	for _, l := range lines {
		exclude = append(exclude, l.ItemID)
		item, ok := s.menu.Lookup(l.ItemID)
		if !ok {
			continue
		}
		l.Available = s.avail.IsAvailable(ctx, l.ItemID)
		total += item.Price * l.Qty
		views = append(views, CartLineView{
			Item:      toSummary(item),
			Qty:       l.Qty,
			AddedFrom: l.AddedFrom,
			Available: l.Available,
		})
		lastLines = append(lastLines, l)
	}
	view := &CartView{Lines: views, Total: total, Recommendation: nil}
	if len(lastLines) == 0 {
		return view, nil
	}
	rec, err := s.rec.ForCart(ctx, lastLines, exclude)
	if err != nil {
		return nil, err
	}
	view.Recommendation = s.recView(ctx, rec)
	return view, nil
}

func (s *CartService) recView(ctx context.Context, rec *recommender.Rec) *RecommendationView {
	if rec == nil {
		return nil
	}
	if !s.menu.HasItem(rec.ItemID) || !s.avail.IsAvailable(ctx, rec.ItemID) {
		return nil
	}
	item, _ := s.menu.Lookup(rec.ItemID)
	return &RecommendationView{Item: toSummary(item), ReasonText: rec.ReasonText, Source: rec.Source}
}
