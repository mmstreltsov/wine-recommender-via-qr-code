package recommender

import (
	"context"

	"svoyshef/internal/domain"
)

type Rec struct {
	ItemID     int64
	ReasonText string
	Score      float64
	Source     string
}

type Recommender interface {
	ForDish(ctx context.Context, itemID int64, excludeInCart []int64) ([]Rec, error)
	ForCart(ctx context.Context, lines []domain.CartLine, excludeInCart []int64) (*Rec, error)
}
