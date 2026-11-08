package app

import (
	"context"
	"encoding/json"
	"math"

	"svoyshef/internal/domain"
	"svoyshef/internal/store"
)

// clientEventNames — события UX, которые фронт может слать через POST /api/v1/events.
var clientEventNames = map[string]map[string]func(any) bool{
	"page_open":       {},
	"menu_view":       {},
	"stub_view":       {},
	"category_select": {"category_id": eventInteger(0, maxEventInteger)},
	"dish_card_open": {
		"item_id": eventInteger(1, maxEventInteger),
		"source":  eventEnum("menu"),
	},
	"rec_feed_visible": {
		"dish_id":    eventInteger(1, maxEventInteger),
		"recs_count": eventInteger(0, 13),
	},
	"rec_impression": {
		"context":     eventEnum("dish", "cart"),
		"rec_item_id": eventInteger(1, maxEventInteger),
		"rec_source":  eventEnum("expert", "upsell", "heuristic"),
	},
	"cart_open": {
		"lines_count": eventInteger(0, maxEventInteger),
		"total":       eventInteger(0, maxEventInteger),
	},
	"pay_click": {
		"lines_count": eventInteger(0, maxEventInteger),
		"total":       eventInteger(0, maxEventInteger),
	},
}

// serverEventNames — события API-действий, их пишет только сервер (events.md §1);
// приём от клиента запрещён (подделка метрик исследования).
var serverEventNames = map[string]struct{}{
	"menu_add_click":   {},
	"rec_add_click":    {},
	"cart_qty_change":  {},
	"cart_item_remove": {},
}

const (
	maxPayloadBytes = 1024
	maxEventInteger = 1<<53 - 1
)

func eventInteger(min, max float64) func(any) bool {
	return func(value any) bool {
		n, ok := value.(float64)
		return ok && n >= min && n <= max && math.Trunc(n) == n
	}
}

func eventEnum(values ...string) func(any) bool {
	return func(value any) bool {
		s, ok := value.(string)
		if !ok {
			return false
		}
		for _, allowed := range values {
			if s == allowed {
				return true
			}
		}
		return false
	}
}

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
	fields, ok := clientEventNames[name]
	if !ok {
		return badRequest(msgUnknownEvent)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if len(payload) != len(fields) {
		return badRequest(msgInvalidPayload)
	}
	for key, valid := range fields {
		if !valid(payload[key]) {
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
