package app

import (
	"context"
	"sort"

	"svoyshef/internal/domain"
)

type MenuProvider interface {
	Categories() []domain.Category
	Items() []domain.MenuItem
	Item(id int64) (domain.MenuItem, bool)
}

type StaticMenu struct {
	categories []domain.Category
	items      []domain.MenuItem
	byID       map[int64]domain.MenuItem
}

func NewStaticMenu(categories []domain.Category, items []domain.MenuItem) *StaticMenu {
	byID := make(map[int64]domain.MenuItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	return &StaticMenu{categories: categories, items: items, byID: byID}
}

func (m *StaticMenu) Categories() []domain.Category {
	return m.categories
}

func (m *StaticMenu) Items() []domain.MenuItem {
	return m.items
}

func (m *StaticMenu) Item(id int64) (domain.MenuItem, bool) {
	it, ok := m.byID[id]
	return it, ok
}

type AvailabilityChecker interface {
	IsAvailable(ctx context.Context, itemID int64) bool
	UnavailableIDs(ctx context.Context) map[int64]struct{}
}

type MenuService struct {
	provider   MenuProvider
	avail      AvailabilityChecker
	upsellItem map[string]int64
}

func NewMenuService(provider MenuProvider, avail AvailabilityChecker, upsellItem map[string]int64) *MenuService {
	return &MenuService{provider: provider, avail: avail, upsellItem: upsellItem}
}

func (s *MenuService) Lookup(id int64) (domain.MenuItem, bool) {
	return s.provider.Item(id)
}

func (s *MenuService) HasItem(id int64) bool {
	_, ok := s.provider.Item(id)
	return ok
}

func (s *MenuService) Menu(ctx context.Context) MenuView {
	off := s.avail.UnavailableIDs(ctx)
	byCat := make(map[int][]domain.MenuItem)
	for _, it := range s.provider.Items() {
		if _, disabled := off[it.ID]; disabled {
			continue
		}
		byCat[it.CategoryID] = append(byCat[it.CategoryID], it)
	}
	categories := make([]MenuCategoryView, 0, len(s.provider.Categories()))
	for _, c := range s.provider.Categories() {
		items := byCat[c.ID]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].SortOrder < items[j].SortOrder })
		views := make([]ItemSummaryView, 0, len(items))
		for _, it := range items {
			views = append(views, toSummary(it))
		}
		categories = append(categories, MenuCategoryView{ID: c.ID, Name: c.Name, Items: views})
	}
	return MenuView{Categories: categories}
}

func (s *MenuService) Item(ctx context.Context, id int64) (ItemFullView, error) {
	it, ok := s.provider.Item(id)
	if !ok {
		return ItemFullView{}, notFound(msgItemNotFound)
	}
	if !s.avail.IsAvailable(ctx, id) {
		return ItemFullView{}, notFound(msgItemNotFound)
	}
	upsells := make([]UpsellView, 0, len(it.UpsellOptions))
	for _, u := range it.UpsellOptions {
		if optItemID, mapped := s.upsellItem[u.Name]; mapped && !s.avail.IsAvailable(ctx, optItemID) {
			continue
		}
		upsells = append(upsells, UpsellView{Name: u.Name, AddPrice: u.AddPrice})
	}
	return ItemFullView{
		ItemSummaryView: toSummary(it),
		Description:     it.Description,
		KBJU:            it.KBJU,
		UpsellOptions:   upsells,
	}, nil
}
