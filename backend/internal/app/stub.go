package app

import (
	"context"

	"svoyshef/internal/domain"
	"svoyshef/internal/recommender"
)

type StubRecommender struct{}

func (StubRecommender) ForDish(ctx context.Context, itemID int64, excludeInCart []int64) ([]recommender.Rec, error) {
	return nil, nil
}

func (StubRecommender) ForCart(ctx context.Context, lines []domain.CartLine, excludeInCart []int64) (*recommender.Rec, error) {
	return nil, nil
}
