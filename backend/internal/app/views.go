package app

import (
	"strings"

	"svoyshef/internal/domain"
)

type ItemSummaryView struct {
	ID            int64  `json:"id"`
	CategoryID    int    `json:"category_id"`
	Name          string `json:"name"`
	Price         int    `json:"price"`
	PhotoURL      string `json:"photo_url"`
	PhotoFallback string `json:"photo_fallback"`
}

type UpsellView struct {
	Name     string `json:"name"`
	AddPrice int    `json:"add_price"`
}

type ItemFullView struct {
	ItemSummaryView
	Description   string       `json:"description"`
	KBJU          string       `json:"kbju"`
	UpsellOptions []UpsellView `json:"upsell_options"`
}

type MenuCategoryView struct {
	ID    int               `json:"id"`
	Name  string            `json:"name"`
	Items []ItemSummaryView `json:"items"`
}

type MenuView struct {
	Categories []MenuCategoryView `json:"categories"`
}

type RecommendationView struct {
	Item       ItemSummaryView `json:"item"`
	ReasonText string          `json:"reason_text"`
	Source     string          `json:"source"`
}

type RecsView struct {
	Recs []RecommendationView `json:"recs"`
}

type CartLineView struct {
	Item      ItemSummaryView `json:"item"`
	Qty       int             `json:"qty"`
	AddedFrom string          `json:"added_from"`
	Available bool            `json:"available"`
}

type CartView struct {
	Lines          []CartLineView      `json:"lines"`
	Total          int                 `json:"total"`
	Recommendation *RecommendationView `json:"recommendation"`
}

func toSummary(it domain.MenuItem) ItemSummaryView {
	return ItemSummaryView{
		ID:            it.ID,
		CategoryID:    it.CategoryID,
		Name:          it.Name,
		Price:         it.Price,
		PhotoURL:      it.PhotoURL,
		PhotoFallback: safeFallback(it.PhotoFallback),
	}
}

func safeFallback(p string) string {
	if p == "" {
		return ""
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return ""
		}
	}
	return p
}
