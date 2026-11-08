package app

import (
	"context"
	"errors"
	"testing"

	"svoyshef/internal/domain"
)

func TestFR37_MenuExcludesUnavailableAndEmptyCategories(t *testing.T) {
	cats := []domain.Category{{ID: 1, Name: "Салаты"}, {ID: 2, Name: "Супы"}, {ID: 3, Name: "Соусы"}}
	items := []domain.MenuItem{
		{ID: 1, CategoryID: 1, Name: "A", Price: 100, SortOrder: 1},
		{ID: 2, CategoryID: 1, Name: "B", Price: 200, SortOrder: 0},
		{ID: 3, CategoryID: 2, Name: "C", Price: 300, SortOrder: 0},
		{ID: 4, CategoryID: 3, Name: "D", Price: 400, SortOrder: 0},
	}
	avail := &fakeAvailability{off: map[int64]struct{}{2: {}, 3: {}}}
	svc := NewMenuService(NewStaticMenu(cats, items), avail, nil)
	view := svc.Menu(context.Background())
	if len(view.Categories) != 2 {
		t.Fatalf("expected 2 categories left (Супы emptied, Соусы has item D), got %d: %+v", len(view.Categories), view.Categories)
	}
	if view.Categories[0].ID != 1 || len(view.Categories[0].Items) != 1 || view.Categories[0].Items[0].ID != 1 {
		t.Fatalf("wrong first category content: %+v", view.Categories[0])
	}
	if view.Categories[1].ID != 3 || len(view.Categories[1].Items) != 1 || view.Categories[1].Items[0].ID != 4 {
		t.Fatalf("wrong second category content: %+v", view.Categories[1])
	}
}

func TestFR37_MenuItemsSortedBySortOrder(t *testing.T) {
	cats := []domain.Category{{ID: 1, Name: "Салаты"}}
	items := []domain.MenuItem{
		{ID: 1, CategoryID: 1, Name: "second", SortOrder: 5},
		{ID: 2, CategoryID: 1, Name: "first", SortOrder: 1},
	}
	svc := NewMenuService(NewStaticMenu(cats, items), &fakeAvailability{off: map[int64]struct{}{}}, nil)
	view := svc.Menu(context.Background())
	if view.Categories[0].Items[0].ID != 2 {
		t.Fatalf("expected SortOrder ASC, got %+v", view.Categories[0].Items)
	}
}

func TestFR37_ItemCard404WhenMissingOrUnavailable(t *testing.T) {
	items := []domain.MenuItem{{ID: 1, CategoryID: 1, Name: "A", Price: 100}}
	avail := &fakeAvailability{off: map[int64]struct{}{1: {}}}
	svc := NewMenuService(NewStaticMenu([]domain.Category{{ID: 1, Name: "С"}}, items), avail, nil)
	_, err := svc.Item(context.Background(), 1)
	var ue *UserError
	if !errors.As(err, &ue) || ue.Code != CodeNotFound {
		t.Fatalf("unavailable item must 404, got %v", err)
	}
	_, err = svc.Item(context.Background(), 999)
	if !errors.As(err, &ue) || ue.Code != CodeNotFound {
		t.Fatalf("missing item must 404, got %v", err)
	}
}

func TestFR37_ItemCardFiltersUnavailableUpsellOptions(t *testing.T) {
	items := []domain.MenuItem{
		{ID: 1, CategoryID: 1, Name: "A", Price: 100, UpsellOptions: []domain.Upsell{
			{Name: "Креветки", AddPrice: 100},
			{Name: "Сыр", AddPrice: 50},
		}},
		{ID: 2, CategoryID: 1, Name: "Креветки блюдо", Price: 500},
	}
	avail := &fakeAvailability{off: map[int64]struct{}{2: {}}}
	upsellMap := map[string]int64{"Креветки": 2}
	svc := NewMenuService(NewStaticMenu([]domain.Category{{ID: 1, Name: "С"}}, items), avail, upsellMap)
	view, err := svc.Item(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.UpsellOptions) != 1 || view.UpsellOptions[0].Name != "Сыр" {
		t.Fatalf("expected only available upsell option, got %+v", view.UpsellOptions)
	}
}

func TestFR37_ItemCardFullSchema(t *testing.T) {
	items := []domain.MenuItem{{
		ID: 1, CategoryID: 2, Name: "A", Price: 100,
		Description: "600 гр.", KBJU: "Калорийность: 100 ккал",
		PhotoURL: "https://cdn/x.jpg", PhotoFallback: "/static/r/photos/x.jpg",
	}}
	svc := NewMenuService(NewStaticMenu([]domain.Category{{ID: 2, Name: "С"}}, items), &fakeAvailability{off: map[int64]struct{}{}}, nil)
	view, err := svc.Item(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if view.Description != "600 гр." || view.KBJU != "Калорийность: 100 ккал" || view.Price != 100 || view.CategoryID != 2 {
		t.Fatalf("full item schema wrong: %+v", view)
	}
}

func TestNFR04_PhotoFallbackSanitizedAgainstTraversal(t *testing.T) {
	views := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/static/r/photos/dishes/x.jpg", "/static/r/photos/dishes/x.jpg"},
		{"/static/r/photos/../menu.json", ""},
		{"/static/r/photos/../../secrets.txt", ""},
		{"../etc/passwd", ""},
	}
	for _, v := range views {
		if got := safeFallback(v.in); got != v.want {
			t.Fatalf("safeFallback(%q) = %q, want %q", v.in, got, v.want)
		}
	}
}

func TestFR37_Recommendations404AndStubEmpty(t *testing.T) {
	menu := testMenu()
	avail := &fakeAvailability{off: map[int64]struct{}{}}
	recs := NewRecommendations(menu, StubRecommender{}, &fakeCartStore{}, avail)
	view, err := recs.ForDish(context.Background(), "s", 101)
	if err != nil {
		t.Fatal(err)
	}
	if view.Recs == nil || len(view.Recs) != 0 {
		t.Fatalf("stub recommender must yield empty recs array, got %+v", view.Recs)
	}
	_, err = recs.ForDish(context.Background(), "s", 999)
	var ue *UserError
	if !errors.As(err, &ue) || ue.Code != CodeNotFound {
		t.Fatalf("recommendations for missing item must 404, got %v", err)
	}
}
