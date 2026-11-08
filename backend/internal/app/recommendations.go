package app

import (
	"context"

	"svoyshef/internal/recommender"
	"svoyshef/internal/store"
)

type Recommendations struct {
	menu  *MenuService
	rec   recommender.Recommender
	carts store.CartStore
	avail AvailabilityChecker
}

func NewRecommendations(menu *MenuService, rec recommender.Recommender, carts store.CartStore, avail AvailabilityChecker) *Recommendations {
	return &Recommendations{menu: menu, rec: rec, carts: carts, avail: avail}
}

func (s *Recommendations) ForDish(ctx context.Context, sessionID string, itemID int64) (RecsView, error) {
	if _, err := s.menu.Item(ctx, itemID); err != nil {
		return RecsView{}, err
	}
	lines, err := s.carts.Lines(ctx, sessionID)
	if err != nil {
		return RecsView{}, err
	}
	exclude := make([]int64, 0, len(lines))
	for _, l := range lines {
		exclude = append(exclude, l.ItemID)
	}
	recs, err := s.rec.ForDish(ctx, itemID, exclude)
	if err != nil {
		return RecsView{}, err
	}
	views := make([]RecommendationView, 0, len(recs))
	for _, r := range recs {
		item, ok := s.menu.Lookup(r.ItemID)
		if !ok || !s.avail.IsAvailable(ctx, r.ItemID) {
			continue
		}
		views = append(views, RecommendationView{Item: toSummary(item), ReasonText: r.ReasonText, Source: r.Source})
	}
	return RecsView{Recs: views}, nil
}
