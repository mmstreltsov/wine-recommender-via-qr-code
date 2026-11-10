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
	catIDs           []int
	catName          map[int]string
	catID            map[string]int
	items            map[int64]domain.MenuItem
	catItems         map[int][]domain.MenuItem
	expert           map[int64][]matrixPair
	upsellOpts       map[int64][]int64
	heurToList       map[int][]int
	priority         [][]int
	displayRank      map[int]int
	soupCategory     int
	bakery           map[int]bool
	bakeryPreferred  int
	substantialCats  map[int]bool
	substantialItems map[int64]bool
	noMainAnchors    map[int]bool
}

var _ recommender.Recommender = (*engine)(nil)

func NewRecommender(categories []domain.Category, items []domain.MenuItem, matrix Matrix, heuristics Heuristics, upsellMap UpsellMap, forcart CoursePriorities, policy RecommendationPolicy) recommender.Recommender {
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
	e.applyPolicy(policy)
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
	policy, err := loadPolicy(filepath.Join(dir, "recommendation_policy.json"), menu.Categories, menu.Items)
	if err != nil {
		return nil, fmt.Errorf("recommendation_policy.json: %w", err)
	}
	return NewRecommender(menu.Categories, menu.Items, matrix, heuristics, upsellMap, forcart, policy), nil
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
	if dish.CategoryID == e.soupCategory {
		cands = e.soupBakerySlot(cands, excludeInCart)
	}
	sort.Slice(cands, func(i, j int) bool {
		if a, b := e.rank(cands[i].item), e.rank(cands[j].item); a != b {
			return a < b
		}
		return displayBetter(cands[i], cands[j])
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
	excluded := make(map[int64]bool, len(lines)+len(excludeInCart))
	for _, id := range excludeInCart {
		excluded[id] = true
	}
	var anchors []domain.MenuItem
	cartCats := make(map[int]bool)
	hasMain := false
	for _, l := range lines {
		excluded[l.ItemID] = true
		if it, ok := e.items[l.ItemID]; ok && l.Available {
			anchors = append(anchors, it)
			cartCats[it.CategoryID] = true
			hasMain = hasMain || e.substantial(it)
		}
	}
	permitted := func(item domain.MenuItem) bool { return !excluded[item.ID] && (!hasMain || !e.substantial(item)) }
	for _, group := range e.priority {
		covered := false
		groupSet := make(map[int]bool, len(group))
		for _, id := range group {
			groupSet[id] = true
			covered = covered || cartCats[id]
		}
		if covered {
			continue
		}
		if c, ok := e.cartCascade(anchors, func(cat int) bool { return groupSet[cat] }, permitted); ok {
			return toRec(c), nil
		}
	}
	if !hasMain {
		var eligible []domain.MenuItem
		for _, anchor := range anchors {
			if e.noMainAnchors[anchor.CategoryID] {
				eligible = append(eligible, anchor)
			}
		}
		if c, ok := e.cartCascade(eligible, nil, func(item domain.MenuItem) bool { return !excluded[item.ID] && e.substantial(item) }); ok {
			return toRec(c), nil
		}
	}
	return nil, nil
}

func displayBetter(a, b candidate) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if a.catID != b.catID {
		return a.catID < b.catID
	}
	if a.item.SortOrder != b.item.SortOrder {
		return a.item.SortOrder < b.item.SortOrder
	}
	return a.item.ID < b.item.ID
}

func (e *engine) soupBakerySlot(cands []candidate, excluded []int64) []candidate {
	suppressed := false
	for _, id := range excluded {
		if e.bakery[e.items[id].CategoryID] {
			suppressed = true
		}
	}
	var winner candidate
	found := false
	result := make([]candidate, 0, len(cands))
	for _, c := range cands {
		if !e.bakery[c.item.CategoryID] {
			result = append(result, c)
			continue
		}
		preferred := c.item.CategoryID == e.bakeryPreferred
		bestPreferred := winner.item.CategoryID == e.bakeryPreferred
		if !found || (preferred && !bestPreferred) || (preferred == bestPreferred && displayBetter(c, winner)) {
			winner, found = c, true
		}
	}
	if found && !suppressed {
		result = append(result, winner)
	}
	return result
}

// FR-19/21: source precedence is global to a missing course, not per anchor.
func (e *engine) cartCascade(anchors []domain.MenuItem, allowed func(int) bool, permitted func(domain.MenuItem) bool) (candidate, bool) {
	var best candidate
	found := false
	bestAnchor := -1
	for i, anchor := range anchors {
		for _, pair := range e.expert[anchor.ID] {
			item, ok := e.items[pair.RecID]
			if !ok || !permitted(item) || (allowed != nil && !allowed(e.pairCategory(pair, item))) {
				continue
			}
			c := candidate{item: item, catID: e.pairCategory(pair, item), score: pair.Score, source: "expert", text: pair.ReasonText}
			if found && !cartExpertBetter(c, i, best, bestAnchor) {
				continue
			}
			best = c
			bestAnchor, found = i, true
		}
	}
	if found {
		return best, true
	}
	for i := len(anchors) - 1; i >= 0; i-- {
		anchor := anchors[i]
		for _, id := range e.upsellOpts[anchor.ID] {
			item, ok := e.items[id]
			if ok && permitted(item) && (allowed == nil || allowed(item.CategoryID)) {
				return candidate{item: item, catID: item.CategoryID, score: scoreUpsell, source: "upsell", text: e.textFor(anchor, item)}, true
			}
		}
	}
	for i := len(anchors) - 1; i >= 0; i-- {
		anchor := anchors[i]
		for _, catID := range e.catIDs {
			if allowed != nil && !allowed(catID) {
				continue
			}
			if c, ok := e.heurTop(anchor, catID); ok && permitted(c.item) {
				return c, true
			}
		}
	}
	return candidate{}, false
}

func cartExpertBetter(a candidate, anchorA int, b candidate, anchorB int) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if anchorA != anchorB {
		return anchorA > anchorB
	}
	if a.item.SortOrder != b.item.SortOrder {
		return a.item.SortOrder < b.item.SortOrder
	}
	return a.item.ID < b.item.ID
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
