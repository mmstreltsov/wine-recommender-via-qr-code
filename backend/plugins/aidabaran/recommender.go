package aidabaran

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"svoyshef/internal/domain"
	"svoyshef/internal/recommender"
)

const (
	forDishLimit   = 13
	scoreUpsell    = 0.6
	scoreHeuristic = 0.4
)

type Matrix = matrixFile
type Heuristics = heuristicsFile
type UpsellMap = upsellMapFile
type CoursePriorities = forcartFile

type engine struct {
	catIDs     []int
	catName    map[int]string
	catID      map[string]int
	items      map[int64]domain.MenuItem
	catItems   map[int][]domain.MenuItem
	expert     map[int64][]matrixPair
	upsellOpts map[int64][]int64
	heurToList map[int][]int
	priority   [][]int
}

var _ recommender.Recommender = (*engine)(nil)

func NewRecommender(categories []domain.Category, items []domain.MenuItem, matrix Matrix, heuristics Heuristics, upsellMap UpsellMap, forcart CoursePriorities) recommender.Recommender {
	e := &engine{
		catName:    make(map[int]string, len(categories)),
		catID:      make(map[string]int, len(categories)),
		items:      make(map[int64]domain.MenuItem, len(items)),
		catItems:   make(map[int][]domain.MenuItem),
		expert:     make(map[int64][]matrixPair),
		upsellOpts: make(map[int64][]int64),
		heurToList: make(map[int][]int),
	}
	for _, c := range categories {
		e.catIDs = append(e.catIDs, c.ID)
		e.catName[c.ID] = c.Name
		e.catID[c.Name] = c.ID
	}
	sort.Ints(e.catIDs)
	for _, it := range items {
		e.items[it.ID] = it
		e.catItems[it.CategoryID] = append(e.catItems[it.CategoryID], it)
	}
	for id, list := range e.catItems {
		sort.Slice(list, func(i, j int) bool {
			if list[i].SortOrder != list[j].SortOrder {
				return list[i].SortOrder < list[j].SortOrder
			}
			return list[i].ID < list[j].ID
		})
		e.catItems[id] = list
	}
	for _, p := range matrix.Pairs {
		e.expert[p.DishID] = append(e.expert[p.DishID], p)
	}
	resolved := resolveUpsellMap(upsellMap.Map)
	for _, it := range items {
		for _, opt := range it.UpsellOptions {
			if id, ok := resolved[normalizeUpsellText(opt.Name)]; ok {
				e.upsellOpts[it.ID] = append(e.upsellOpts[it.ID], id)
			}
		}
	}
	for _, r := range heuristics.Rules {
		fromID, ok := e.catID[r.FromCategory]
		if !ok {
			continue
		}
		for _, name := range r.ToCategories {
			if id, ok := e.catID[name]; ok {
				e.heurToList[fromID] = append(e.heurToList[fromID], id)
			}
		}
	}
	for _, group := range forcart.CoursePriority {
		ids := make([]int, 0, len(group))
		for _, name := range group {
			if id, ok := e.catID[name]; ok {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		e.priority = append(e.priority, ids)
	}
	return e
}

func LoadRecommender(dataDir, restaurant string) (recommender.Recommender, error) {
	menu, err := LoadMenu(dataDir, restaurant)
	if err != nil {
		return nil, fmt.Errorf("load menu: %w", err)
	}
	dir := filepath.Join(dataDir, restaurant, "recommender")
	var matrix Matrix
	if err := loadJSONFile(filepath.Join(dir, "matrix.json"), &matrix); err != nil {
		return nil, fmt.Errorf("matrix.json: %w", err)
	}
	var heuristics Heuristics
	if err := loadJSONFile(filepath.Join(dir, "heuristics.json"), &heuristics); err != nil {
		return nil, fmt.Errorf("heuristics.json: %w", err)
	}
	var upsellMap UpsellMap
	if err := loadJSONFile(filepath.Join(dir, "upsell_map.json"), &upsellMap); err != nil {
		return nil, fmt.Errorf("upsell_map.json: %w", err)
	}
	var forcart CoursePriorities
	if err := loadJSONFile(filepath.Join(dir, "forcart_priorities.json"), &forcart); err != nil {
		return nil, fmt.Errorf("forcart_priorities.json: %w", err)
	}
	return NewRecommender(menu.Categories, menu.Items, matrix, heuristics, upsellMap, forcart), nil
}

func normalizeUpsellText(s string) string {
	return strings.TrimSpace(upsellPriceRe.ReplaceAllString(strings.TrimSpace(s), ""))
}

func resolveUpsellMap(m map[string]int64) map[string]int64 {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	resolved := make(map[string]int64, len(m))
	for _, k := range keys {
		nk := normalizeUpsellText(k)
		if nk == "" {
			continue
		}
		if _, dup := resolved[nk]; !dup {
			resolved[nk] = m[k]
		}
	}
	return resolved
}

type candidate struct {
	item   domain.MenuItem
	catID  int
	score  float64
	source string
	text   string
}

func (e *engine) ForDish(ctx context.Context, itemID int64, excludeInCart []int64) ([]recommender.Rec, error) {
	dish, ok := e.items[itemID]
	if !ok {
		return []recommender.Rec{}, nil
	}
	excluded := make(map[int64]bool, len(excludeInCart)+1)
	for _, id := range excludeInCart {
		excluded[id] = true
	}
	excluded[itemID] = true
	var cands []candidate
	for _, catID := range e.catIDs {
		if catID == dish.CategoryID {
			continue
		}
		cand, ok := e.cascade(dish, catID)
		if !ok || excluded[cand.item.ID] {
			continue
		}
		cands = append(cands, cand)
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		if cands[i].catID != cands[j].catID {
			return cands[i].catID < cands[j].catID
		}
		if cands[i].item.SortOrder != cands[j].item.SortOrder {
			return cands[i].item.SortOrder < cands[j].item.SortOrder
		}
		return cands[i].item.ID < cands[j].item.ID
	})
	if len(cands) > forDishLimit {
		cands = cands[:forDishLimit]
	}
	recs := make([]recommender.Rec, 0, len(cands))
	for _, c := range cands {
		recs = append(recs, recommender.Rec{ItemID: c.item.ID, ReasonText: c.text, Score: c.score, Source: c.source})
	}
	return recs, nil
}

func (e *engine) ForCart(ctx context.Context, lines []domain.CartLine, excludeInCart []int64) (*recommender.Rec, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	excluded := make(map[int64]bool, len(lines)+len(excludeInCart))
	for _, id := range excludeInCart {
		excluded[id] = true
	}
	for _, l := range lines {
		excluded[l.ItemID] = true
	}
	last, ok := e.items[lines[len(lines)-1].ItemID]
	if !ok {
		return nil, nil
	}
	cartCats := make(map[int]bool)
	for _, l := range lines {
		if it, ok := e.items[l.ItemID]; ok {
			cartCats[it.CategoryID] = true
		}
	}
	if group := e.firstUncoveredGroup(cartCats); group != nil {
		groupSet := make(map[int]bool, len(group))
		for _, id := range group {
			groupSet[id] = true
		}
		allowed := func(c int) bool { return groupSet[c] }
		if c, ok := e.expertBest(last, allowed, excluded); ok {
			return toRec(c), nil
		}
		if c, ok := e.upsellBest(last, allowed, excluded); ok {
			return toRec(c), nil
		}
		if c, ok := e.heurGroupTop(last, group, excluded); ok {
			return toRec(c), nil
		}
		return nil, nil
	}
	if c, ok := e.expertBest(last, nil, excluded); ok {
		return toRec(c), nil
	}
	if c, ok := e.heurFirstFrom(last, excluded); ok {
		return toRec(c), nil
	}
	return nil, nil
}

func toRec(c candidate) *recommender.Rec {
	return &recommender.Rec{ItemID: c.item.ID, ReasonText: c.text, Score: c.score, Source: c.source}
}

func (e *engine) cascade(dish domain.MenuItem, catID int) (candidate, bool) {
	exact := func(c int) bool { return c == catID }
	if c, ok := e.expertBest(dish, exact, nil); ok {
		return c, true
	}
	if c, ok := e.upsellBest(dish, exact, nil); ok {
		return c, true
	}
	return e.heurTop(dish, catID)
}

func (e *engine) expertBest(dish domain.MenuItem, allowed func(int) bool, excluded map[int64]bool) (candidate, bool) {
	var bestPair matrixPair
	var bestRec domain.MenuItem
	found := false
	for _, p := range e.expert[dish.ID] {
		rec, ok := e.items[p.RecID]
		if !ok {
			continue
		}
		catID := e.pairCategory(p, rec)
		if allowed != nil && !allowed(catID) {
			continue
		}
		if excluded != nil && excluded[rec.ID] {
			continue
		}
		if found && !e.pairBetter(p, rec, bestPair, bestRec) {
			continue
		}
		bestPair, bestRec, found = p, rec, true
	}
	if !found {
		return candidate{}, false
	}
	source := bestPair.Source
	if source == "" {
		source = "expert"
	}
	return candidate{item: bestRec, catID: e.pairCategory(bestPair, bestRec), score: bestPair.Score, source: source, text: bestPair.ReasonText}, true
}

func (e *engine) pairCategory(p matrixPair, rec domain.MenuItem) int {
	if p.CategoryOverride != "" {
		if id, ok := e.catID[p.CategoryOverride]; ok {
			return id
		}
	}
	return rec.CategoryID
}

func (e *engine) pairBetter(a matrixPair, aRec domain.MenuItem, b matrixPair, bRec domain.MenuItem) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if aRec.SortOrder != bRec.SortOrder {
		return aRec.SortOrder < bRec.SortOrder
	}
	return a.RecID < b.RecID
}

func (e *engine) upsellBest(dish domain.MenuItem, allowed func(int) bool, excluded map[int64]bool) (candidate, bool) {
	for _, optID := range e.upsellOpts[dish.ID] {
		it, ok := e.items[optID]
		if !ok {
			continue
		}
		if allowed != nil && !allowed(it.CategoryID) {
			continue
		}
		if excluded != nil && excluded[it.ID] {
			continue
		}
		return candidate{item: it, catID: it.CategoryID, score: scoreUpsell, source: "upsell", text: e.textFor(dish, it)}, true
	}
	return candidate{}, false
}

func (e *engine) heurTop(dish domain.MenuItem, catID int) (candidate, bool) {
	if !e.heurHas(dish.CategoryID, catID) {
		return candidate{}, false
	}
	return e.heurCand(dish, catID)
}

func (e *engine) heurGroupTop(dish domain.MenuItem, group []int, excluded map[int64]bool) (candidate, bool) {
	for _, catID := range group {
		if !e.heurHas(dish.CategoryID, catID) {
			continue
		}
		tops := e.catItems[catID]
		if len(tops) == 0 || excluded[tops[0].ID] {
			continue
		}
		return e.heurCand(dish, catID)
	}
	return candidate{}, false
}

func (e *engine) heurFirstFrom(dish domain.MenuItem, excluded map[int64]bool) (candidate, bool) {
	for _, catID := range e.heurToList[dish.CategoryID] {
		tops := e.catItems[catID]
		if len(tops) == 0 || excluded[tops[0].ID] {
			continue
		}
		return e.heurCand(dish, catID)
	}
	return candidate{}, false
}

func (e *engine) heurCand(dish domain.MenuItem, catID int) (candidate, bool) {
	tops := e.catItems[catID]
	if len(tops) == 0 {
		return candidate{}, false
	}
	return candidate{item: tops[0], catID: catID, score: scoreHeuristic, source: "heuristic", text: e.textFor(dish, tops[0])}, true
}

func (e *engine) heurHas(fromCat, toCat int) bool {
	for _, id := range e.heurToList[fromCat] {
		if id == toCat {
			return true
		}
	}
	return false
}

func (e *engine) firstUncoveredGroup(cartCats map[int]bool) []int {
	for _, group := range e.priority {
		covered := false
		for _, id := range group {
			if cartCats[id] {
				covered = true
				break
			}
		}
		if !covered {
			return group
		}
	}
	return nil
}
