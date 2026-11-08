package domain

import "time"

type Category struct {
	ID   int
	Name string
}

type MenuItem struct {
	ID            int64
	CategoryID    int
	Name          string
	Price         int
	Description   string
	KBJU          string
	PhotoURL      string
	PhotoFallback string
	UpsellOptions []Upsell
	SortOrder     int
}

type Upsell struct {
	Name     string
	AddPrice int
}

type CartLine struct {
	ItemID    int64
	Qty       int
	AddedFrom string
	Available bool
}

type Cart struct {
	SessionID string
	Lines     []CartLine
	UpdatedAt time.Time
}

type Event struct {
	SessionID string
	TableID   *int
	Name      string
	Payload   map[string]any
	Ts        time.Time
}
