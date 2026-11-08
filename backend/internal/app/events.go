package app

import (
	"context"
	"encoding/json"

	"svoyshef/internal/domain"
	"svoyshef/internal/store"
)

// clientEventNames — события UX, которые фронт может слать через POST /api/v1/events.
var clientEventNames = map[string]struct{}{
	"page_open":        {},
	"menu_view":        {},
	"category_select":  {},
	"dish_card_open":   {},
	"rec_feed_visible": {},
	"rec_impression":   {},
	"cart_open":        {},
	"pay_click":        {},
	"stub_view":        {},
}

// serverEventNames — события API-действий, их пишет только сервер (events.md §1);
// приём от клиента запрещён (подделка метрик исследования).
var serverEventNames = map[string]struct{}{
	"menu_add_click":   {},
	"rec_add_click":    {},
	"cart_qty_change":  {},
	"cart_item_remove": {},
}

const maxPayloadBytes = 1024

type EventsService struct {
	events store.EventStore
}

func NewEventsService(events store.EventStore) *EventsService {
	return &EventsService{events: events}
}

func (s *EventsService) RecordServerEvent(ctx context.Context, sessionID string, tableID *int, name string, payload map[string]any) error {
	return s.events.Insert(ctx, domain.Event{SessionID: sessionID, TableID: tableID, Name: name, Payload: payload})
}

func (s *EventsService) RecordClientEvent(ctx context.Context, sessionID string, tableID *int, name string, payload map[string]any) error {
	if _, ok := clientEventNames[name]; !ok {
		return badRequest(msgUnknownEvent)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	for _, v := range payload {
		switch v.(type) {
		case string, float64, bool, nil:
		default:
			return badRequest(msgInvalidPayload)
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return badRequest(msgInvalidPayload)
	}
	if len(raw) > maxPayloadBytes {
		return badRequest(msgInvalidPayload)
	}
	return s.events.Insert(ctx, domain.Event{SessionID: sessionID, TableID: tableID, Name: name, Payload: payload})
}
