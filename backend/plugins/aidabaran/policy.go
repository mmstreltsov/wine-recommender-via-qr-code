package aidabaran

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"svoyshef/internal/domain"
)

const fallbackDisplayRank = 4

// RecommendationPolicy is plugin data; display groups and cart roles are independent.
type RecommendationPolicy struct {
	DisplayGroups          []displayGroup `json:"display_groups"`
	SoupBakery             *soupBakery    `json:"soup_bakery,omitempty"`
	SubstantialCategories  []string       `json:"substantial_categories"`
	SubstantialItemIDs     []int64        `json:"substantial_item_ids,omitempty"`
	NoMainAnchorCategories []string       `json:"no_main_anchor_categories"`
}

type displayGroup struct {
	Rank       int      `json:"rank"`
	Categories []string `json:"categories"`
}

type soupBakery struct {
	SourceCategory    string   `json:"source_category"`
	TargetCategories  []string `json:"target_categories"`
	PreferredCategory string   `json:"preferred_category"`
}

func loadPolicy(path string, categories []domain.Category, items []domain.MenuItem) (RecommendationPolicy, error) {
	var policy RecommendationPolicy
	data, err := os.ReadFile(path)
	if err != nil {
		return policy, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return policy, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return policy, fmt.Errorf("expected a single policy object")
	}
	return policy, policy.validate(categories, items)
}

func (p RecommendationPolicy) validate(categories []domain.Category, items []domain.MenuItem) error {
	idx := buildMenuIndex(categories, items)
	checkNames := func(names []string) (map[string]bool, error) {
		seen := make(map[string]bool, len(names))
		for _, name := range names {
			if _, ok := idx.catIDByName[name]; !ok || seen[name] {
				return nil, fmt.Errorf("unknown or duplicate category %q", name)
			}
			seen[name] = true
		}
		return seen, nil
	}
	seenCategories, seenRanks := map[string]bool{}, map[int]bool{}
	for _, group := range p.DisplayGroups {
		if group.Rank < 0 || group.Rank > 6 || group.Rank == fallbackDisplayRank || seenRanks[group.Rank] || len(group.Categories) == 0 {
			return fmt.Errorf("invalid or duplicate display rank %d", group.Rank)
		}
		seenRanks[group.Rank] = true
		if _, err := checkNames(group.Categories); err != nil {
			return err
		}
		for _, name := range group.Categories {
			if seenCategories[name] {
				return fmt.Errorf("duplicate display category %q", name)
			}
			seenCategories[name] = true
		}
	}
	for _, name := range parts {
		if _, present := idx.catIDByName[name]; present && !seenCategories[name] {
			return fmt.Errorf("missing display category %q", name)
		}
	}
	if len(p.SubstantialCategories) == 0 || len(p.NoMainAnchorCategories) == 0 {
		return fmt.Errorf("substantial and no-main anchor categories must not be empty")
	}
	main, err := checkNames(p.SubstantialCategories)
	if err != nil {
		return err
	}
	anchors, err := checkNames(p.NoMainAnchorCategories)
	if err != nil {
		return err
	}
	for name := range anchors {
		if main[name] {
			return fmt.Errorf("conflicting cart roles for %q", name)
		}
	}
	seenIDs := map[int64]bool{}
	for _, id := range p.SubstantialItemIDs {
		item, ok := idx.itemByID[id]
		if !ok || id <= 0 || seenIDs[id] || idx.catNameByID[item.CategoryID] != "Горячие закуски" || main[idx.catNameByID[item.CategoryID]] || anchors[idx.catNameByID[item.CategoryID]] {
			return fmt.Errorf("invalid or duplicate substantial item ID %d", id)
		}
		seenIDs[id] = true
	}
	if b := p.SoupBakery; b != nil {
		if _, ok := idx.catIDByName[b.SourceCategory]; !ok || len(b.TargetCategories) < 2 {
			return fmt.Errorf("invalid bakery source or targets")
		}
		targets, err := checkNames(b.TargetCategories)
		if err != nil {
			return err
		}
		if targets[b.SourceCategory] || !targets[b.PreferredCategory] {
			return fmt.Errorf("conflicting bakery source or preference")
		}
	} else {
		return fmt.Errorf("bakery group is required")
	}
	return nil
}

func (e *engine) applyPolicy(p RecommendationPolicy) {
	e.displayRank = map[int]int{}
	e.bakery = map[int]bool{}
	e.substantialCats = map[int]bool{}
	e.substantialItems = map[int64]bool{}
	e.noMainAnchors = map[int]bool{}
	for _, group := range p.DisplayGroups {
		for _, name := range group.Categories {
			e.displayRank[e.catID[name]] = group.Rank
		}
	}
	if b := p.SoupBakery; b != nil {
		e.soupCategory = e.catID[b.SourceCategory]
		e.bakeryPreferred = e.catID[b.PreferredCategory]
		for _, name := range b.TargetCategories {
			e.bakery[e.catID[name]] = true
		}
	}
	for _, name := range p.SubstantialCategories {
		e.substantialCats[e.catID[name]] = true
	}
	for _, id := range p.SubstantialItemIDs {
		e.substantialItems[id] = true
	}
	for _, name := range p.NoMainAnchorCategories {
		e.noMainAnchors[e.catID[name]] = true
	}
}

func (e *engine) rank(item domain.MenuItem) int {
	if rank, ok := e.displayRank[item.CategoryID]; ok {
		return rank
	}
	return fallbackDisplayRank
}

func (e *engine) substantial(item domain.MenuItem) bool {
	return e.substantialCats[item.CategoryID] || e.substantialItems[item.ID]
}
