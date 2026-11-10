package aidabaran

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"svoyshef/internal/domain"
	"svoyshef/internal/recommender"
)

func policyFixture(t *testing.T) ([]domain.Category, []domain.MenuItem, RecommendationPolicy) {
	t.Helper()
	var cats []domain.Category
	var items []domain.MenuItem
	for i, name := range parts {
		cat := i + 1
		cats = append(cats, domain.Category{ID: cat, Name: name})
		for n := 0; n < 3; n++ {
			items = append(items, domain.MenuItem{ID: int64(cat*100 + n), CategoryID: cat, Name: name, SortOrder: n})
		}
	}
	p, err := loadPolicy("../../../data/aidabaran/recommender/recommendation_policy.json", cats, items)
	if err != nil {
		t.Fatal(err)
	}
	return cats, items, p
}

func policyEngine(t *testing.T, pairs []matrixPair, rules []heuristicRule) *engine {
	t.Helper()
	cats, items, p := policyFixture(t)
	return NewRecommender(cats, items, Matrix{Pairs: pairs}, Heuristics{Rules: rules}, UpsellMap{}, CoursePriorities{CoursePriority: [][]string{
		{"Напитки"}, {"Десерты"}, {"Салаты", "Холодные закуски"}, {"Овощи на углях и гарниры", "Соусы"},
	}}, p).(*engine)
}

func expertPair(from, to int64, score float64, reason string) matrixPair {
	return matrixPair{DishID: from, RecID: to, Score: score, Source: "expert", ReasonText: reason}
}

func requireCartRec(t *testing.T, e *engine, lines []domain.CartLine, excluded []int64, want *recommender.Rec) {
	t.Helper()
	got, err := e.ForCart(context.Background(), lines, excluded)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ForCart = %+v, %v; want %+v", got, err, want)
	}
}

func TestFR10_PolicyDisplayAllCategoriesAndUnknown(t *testing.T) {
	e := policyEngine(t, nil, nil)
	e.catIDs = append(e.catIDs, 15, 16)
	e.items[1500] = domain.MenuItem{ID: 1500, CategoryID: 15}
	e.items[1600] = domain.MenuItem{ID: 1600, CategoryID: 16}
	for _, id := range []int64{1400, 1200, 1500, 1600, 300, 400, 500, 600, 700, 800, 900, 200, 1000, 1300, 1100} {
		e.expert[100] = append(e.expert[100], expertPair(100, id, 0.9, "unchanged"))
	}
	// ForDish remains capped at 13; test complete ordering without truncation in two calls.
	got, err := e.ForDish(context.Background(), 100, []int64{400, 500})
	want := []int64{1100, 1000, 1300, 200, 600, 300, 700, 800, 900, 1500, 1600, 1200, 1400}
	if err != nil || !reflect.DeepEqual(recIDs(got), want) {
		t.Fatalf("ForDish = %v, %v; want %v", recIDs(got), err, want)
	}
	for _, rec := range got {
		if rec.Score != 0.9 || rec.Source != "expert" || rec.ReasonText != "unchanged" {
			t.Fatalf("pair modified: %+v", rec)
		}
	}
	if e.rank(e.items[400]) != 3 || e.rank(e.items[500]) != 3 {
		t.Fatal("substantial display categories lost")
	}
}

func TestFR28_SoupBakeryPreferenceSuppressionAndRestoration(t *testing.T) {
	pairs := []matrixPair{expertPair(300, 1300, 0.1, "bread"), expertPair(300, 400, 0.99, "khachapuri"), expertPair(300, 700, 0.9, "cheburek"), expertPair(300, 1100, 0.5, "sauce")}
	e := policyEngine(t, pairs, nil)
	for _, tc := range []struct {
		name    string
		exclude []int64
		want    []int64
	}{
		{"bread_preferred_regardless_score", nil, []int64{1100, 1300, 700}},
		{"other_bread_in_cart", []int64{1301}, []int64{1100, 700}},
		{"other_khachapuri_in_cart", []int64{401}, []int64{1100, 700}},
		{"selected_bread_no_replacement", []int64{1300}, []int64{1100, 700}},
		{"ordinary_category_not_suppressed", []int64{1101}, []int64{1100, 1300, 700}},
		{"restored", nil, []int64{1100, 1300, 700}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.ForDish(context.Background(), 300, tc.exclude)
			if err != nil || !reflect.DeepEqual(recIDs(got), tc.want) {
				t.Fatalf("got %v, %v; want %v", recIDs(got), err, tc.want)
			}
		})
	}
	for _, source := range []int64{500, 700} {
		e.expert[source] = []matrixPair{expertPair(source, 1300, 0.1, "bread"), expertPair(source, 400, 0.99, "khachapuri")}
		got, err := e.ForDish(context.Background(), source, nil)
		if err != nil || !reflect.DeepEqual(recIDs(got), []int64{1300, 400}) {
			t.Fatalf("non-soup must retain both bakery categories: %v, %v", recIDs(got), err)
		}
	}
}

func TestFR19_GlobalCascadeAcrossAnchorsAndWinningPair(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pairs      []matrixPair
		wantID     int64
		wantScore  float64
		wantReason string
	}{
		{"old_expert_beats_new_upsell", []matrixPair{expertPair(500, 1400, 0.1, "old expert")}, 1400, 0.1, "old expert"},
		{"score_before_recency", []matrixPair{expertPair(500, 1400, 0.9, "high old"), expertPair(300, 1401, 0.8, "low new")}, 1400, 0.9, "high old"},
		{"recency_before_target_sort", []matrixPair{expertPair(500, 1400, 0.9, "old"), expertPair(300, 1401, 0.9, "new")}, 1401, 0.9, "new"},
		{"same_target_preserves_winning_reason", []matrixPair{expertPair(500, 1400, 0.9, "old same target"), expertPair(300, 1400, 0.9, "new same target")}, 1400, 0.9, "new same target"},
		{"target_sort_then_id", []matrixPair{expertPair(300, 1402, 0.9, "second sort"), expertPair(300, 1401, 0.9, "first sort")}, 1401, 0.9, "first sort"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := policyEngine(t, tc.pairs, []heuristicRule{{FromCategory: "Супы", ToCategories: []string{"Напитки"}}})
			e.upsellOpts[300] = []int64{1402}
			requireCartRec(t, e, cartLines(500, 300), nil, &recommender.Rec{ItemID: tc.wantID, Score: tc.wantScore, Source: "expert", ReasonText: tc.wantReason})
		})
	}
	e := policyEngine(t, []matrixPair{expertPair(300, 1402, 0.9, "larger id"), expertPair(300, 1401, 0.9, "smaller id")}, nil)
	item := e.items[1402]
	item.SortOrder = e.items[1401].SortOrder
	e.items[1402] = item
	requireCartRec(t, e, cartLines(300), nil, &recommender.Rec{ItemID: 1401, Score: 0.9, Source: "expert", ReasonText: "smaller id"})
}

func TestFR19_UpsellHeuristicGlobalOrderAndExclusions(t *testing.T) {
	e := policyEngine(t, nil, []heuristicRule{{FromCategory: "Супы", ToCategories: []string{"Напитки"}}})
	e.upsellOpts[500], e.upsellOpts[300] = []int64{1400}, []int64{1402, 1401}
	check := func(lines []domain.CartLine, excluded []int64, id int64, source string, anchor int64) {
		t.Helper()
		score := scoreUpsell
		if source == "heuristic" {
			score = scoreHeuristic
		}
		requireCartRec(t, e, lines, excluded, &recommender.Rec{ItemID: id, Source: source, Score: score, ReasonText: e.textFor(e.items[anchor], e.items[id])})
	}
	check(cartLines(500, 300), nil, 1402, "upsell", 300)
	check(cartLines(500, 300), []int64{1402}, 1401, "upsell", 300)
	delete(e.upsellOpts, 300)
	check(cartLines(500, 300), nil, 1400, "upsell", 500) // old upsell beats new heuristic
	delete(e.upsellOpts, 500)
	check(cartLines(500, 300), nil, 1400, "heuristic", 300)
	requireCartRec(t, e, cartLines(500, 300), []int64{1400}, nil) // no second drink
	// Heuristic anchor recency precedes category ID; excluded top-1 skips category.
	e.priority = [][]int{{1, 2}}
	e.heurToList[5], e.heurToList[3] = []int{1}, []int{2, 1}
	check(cartLines(500, 300), nil, 100, "heuristic", 300)
	check(cartLines(500, 300), []int64{100}, 200, "heuristic", 300)
	e.heurToList[3] = []int{2}
	check(cartLines(500, 300), nil, 200, "heuristic", 300)
}

func TestFR37_UnavailableAnchorsCoverageAndMainRole(t *testing.T) {
	e := policyEngine(t, []matrixPair{expertPair(500, 1400, 0.9, "old anchor")}, nil)
	want := &recommender.Rec{ItemID: 1400, Source: "expert", Score: 0.9, ReasonText: "old anchor"}
	requireCartRec(t, e, cartLines(500, 300), nil, want)
	lines := cartLines(500, 300)
	lines[1].Available = false
	requireCartRec(t, e, lines, nil, want)
	lines[0].Available, lines[1].Available = false, true
	requireCartRec(t, e, lines, nil, nil)
	lines[1].Available = false
	requireCartRec(t, e, lines, nil, nil)
	lines = cartLines(500, 1401)
	lines[1].Available = false
	requireCartRec(t, e, lines, nil, want) // unavailable drink gives no coverage
	e = policyEngine(t, []matrixPair{expertPair(1100, 500, 1, "excluded unavailable main"), expertPair(1100, 800, 0.8, "allowed main")}, nil)
	lines = cartLines(1100, 500)
	lines[1].Available = false
	requireCartRec(t, e, lines, nil, &recommender.Rec{ItemID: 800, Source: "expert", Score: 0.8, ReasonText: "allowed main"})
}

func TestFR21_SubstantialGuardUsesActualCategoryInEverySource(t *testing.T) {
	for _, source := range []string{"expert", "upsell", "heuristic"} {
		t.Run(source, func(t *testing.T) {
			e := policyEngine(t, nil, nil)
			e.priority = [][]int{{8, 11}}
			switch source {
			case "expert":
				e.expert[300] = []matrixPair{expertPair(300, 800, 1, "second main"), expertPair(300, 1100, 0.5, "allowed sauce")}
			case "upsell":
				e.upsellOpts[300] = []int64{800, 1100}
			case "heuristic":
				e.heurToList[3] = []int{8, 11}
			}
			got, err := e.ForCart(context.Background(), cartLines(500, 300), nil)
			if err != nil || got == nil || got.ItemID != 1100 || got.Source != source {
				t.Fatalf("must continue past second main in %s: %+v, %v", source, got, err)
			}
		})
	}
	// Effective drink membership must not disguise an actual substantial target.
	p := expertPair(300, 800, 1, "main disguised as drink")
	p.CategoryOverride = "Напитки"
	e := policyEngine(t, []matrixPair{p, expertPair(300, 1400, 0.2, "actual drink")}, nil)
	requireCartRec(t, e, cartLines(500, 300), nil, &recommender.Rec{ItemID: 1400, Source: "expert", Score: 0.2, ReasonText: "actual drink"})
	// Soup/hot starter do not acquire a substantial role from display group or name.
	for _, id := range []int64{300, 600, 1400, 1200} {
		if e.substantial(e.items[id]) {
			t.Fatalf("unexpected substantial role: %d", id)
		}
	}
	for _, id := range []int64{400, 500, 700, 800, 900} {
		if !e.substantial(e.items[id]) {
			t.Fatalf("missing substantial role: %d", id)
		}
	}
}

func TestFR21_BoundedNoMainOnlyAfterGroupsAndAllowedConnections(t *testing.T) {
	for _, source := range []string{"expert", "upsell", "heuristic"} {
		for _, anchor := range []int64{1000, 1100, 1300, 300, 600, 1200, 1400, 100} {
			t.Run(source+"/"+parts[int(anchor/100)-1], func(t *testing.T) {
				e := policyEngine(t, nil, nil)
				switch source {
				case "expert":
					e.expert[anchor] = []matrixPair{expertPair(anchor, 800, 0.9, "existing link")}
				case "upsell":
					e.upsellOpts[anchor] = []int64{800}
				case "heuristic":
					e.heurToList[int(anchor/100)] = []int{8}
				}
				got, err := e.ForCart(context.Background(), cartLines(anchor), nil)
				eligible := anchor == 1000 || anchor == 1100 || anchor == 1300
				if err != nil || (eligible && (got == nil || got.ItemID != 800 || got.Source != source)) || (!eligible && got != nil) {
					t.Fatalf("bounded fallback: %+v, %v; eligible=%v", got, err, eligible)
				}
				if eligible {
					e.priority = nil // all groups covered/exhausted is also allowed
					again, err := e.ForCart(context.Background(), cartLines(anchor), nil)
					if err != nil || !reflect.DeepEqual(got, again) {
						t.Fatalf("all-covered fallback differs: %+v, %v", again, err)
					}
				}
			})
		}
	}
	e := policyEngine(t, nil, nil)
	requireCartRec(t, e, cartLines(1100), nil, nil) // eligible anchor without connection is insufficient
	e.expert[1100] = []matrixPair{expertPair(1100, 800, 1, "main"), expertPair(1100, 100, 0.1, "missing salad")}
	requireCartRec(t, e, cartLines(1100), nil, &recommender.Rec{ItemID: 100, Score: 0.1, Source: "expert", ReasonText: "missing salad"})
	requireCartRec(t, e, cartLines(1100, 500), nil, &recommender.Rec{ItemID: 100, Score: 0.1, Source: "expert", ReasonText: "missing salad"})
}

func TestFR10_PolicyValidationAndStartupFailClosed(t *testing.T) {
	cats, items, valid := policyFixture(t)
	for _, tc := range []struct {
		name   string
		mutate func(*RecommendationPolicy)
	}{
		{"unknown_category", func(p *RecommendationPolicy) { p.DisplayGroups[0].Categories[0] = "unknown" }},
		{"duplicate_category", func(p *RecommendationPolicy) {
			p.DisplayGroups[1].Categories = append(p.DisplayGroups[1].Categories, "Соусы")
		}},
		{"missing_known_category", func(p *RecommendationPolicy) { p.DisplayGroups = p.DisplayGroups[1:] }},
		{"duplicate_rank", func(p *RecommendationPolicy) { p.DisplayGroups[1].Rank = 0 }},
		{"fallback_rank_reserved", func(p *RecommendationPolicy) { p.DisplayGroups[1].Rank = 4 }},
		{"negative_rank", func(p *RecommendationPolicy) { p.DisplayGroups[1].Rank = -1 }},
		{"out_of_range_rank", func(p *RecommendationPolicy) { p.DisplayGroups[1].Rank = 7 }},
		{"empty_group", func(p *RecommendationPolicy) { p.DisplayGroups[0].Categories = nil }},
		{"empty_substantial", func(p *RecommendationPolicy) { p.SubstantialCategories = nil }},
		{"empty_no_main_anchors", func(p *RecommendationPolicy) { p.NoMainAnchorCategories = nil }},
		{"missing_bakery", func(p *RecommendationPolicy) { p.SoupBakery = nil }},
		{"empty_bakery", func(p *RecommendationPolicy) { p.SoupBakery = &soupBakery{} }},
		{"single_bakery_target", func(p *RecommendationPolicy) { p.SoupBakery.TargetCategories = p.SoupBakery.TargetCategories[:1] }},
		{"unknown_role", func(p *RecommendationPolicy) { p.SubstantialCategories = []string{"unknown"} }},
		{"duplicate_role", func(p *RecommendationPolicy) {
			p.SubstantialCategories = append(p.SubstantialCategories, p.SubstantialCategories[0])
		}},
		{"conflicting_role", func(p *RecommendationPolicy) {
			p.NoMainAnchorCategories = append(p.NoMainAnchorCategories, p.SubstantialCategories[0])
		}},
		{"unknown_uid", func(p *RecommendationPolicy) { p.SubstantialItemIDs = []int64{999999} }},
		{"duplicate_uid", func(p *RecommendationPolicy) { p.SubstantialItemIDs = []int64{600, 600} }},
		{"non_hotstarter_uid", func(p *RecommendationPolicy) { p.SubstantialItemIDs = []int64{300} }},
		{"uid_in_substantial_category", func(p *RecommendationPolicy) {
			p.SubstantialCategories = append(p.SubstantialCategories, "Горячие закуски")
			p.SubstantialItemIDs = []int64{600}
		}},
		{"uid_in_anchor_category", func(p *RecommendationPolicy) {
			p.NoMainAnchorCategories = append(p.NoMainAnchorCategories, "Горячие закуски")
			p.SubstantialItemIDs = []int64{600}
		}},
		{"unknown_bakery_source", func(p *RecommendationPolicy) { p.SoupBakery.SourceCategory = "unknown" }},
		{"duplicate_bakery", func(p *RecommendationPolicy) {
			p.SoupBakery.TargetCategories = []string{"Хачапури", "Хачапури"}
		}},
		{"unknown_bakery_preference", func(p *RecommendationPolicy) { p.SoupBakery.PreferredCategory = "Чебуреки" }},
		{"bakery_source_target_overlap", func(p *RecommendationPolicy) { p.SoupBakery.SourceCategory = "Хачапури" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(valid)
			if err != nil {
				t.Fatal(err)
			}
			var p RecommendationPolicy
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&p)
			if err := p.validate(cats, items); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
	// Unknown menu categories are allowed, not guessed into any role.
	cats = append(cats, domain.Category{ID: 15, Name: "New category"})
	if err := valid.validate(cats, items); err != nil {
		t.Fatal(err)
	}
	// Ranks have no semantic labels in this schema; valid policy changes remain configurable.
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var reordered RecommendationPolicy
	if err := json.Unmarshal(data, &reordered); err != nil {
		t.Fatal(err)
	}
	reordered.DisplayGroups[0].Rank, reordered.DisplayGroups[5].Rank = reordered.DisplayGroups[5].Rank, reordered.DisplayGroups[0].Rank
	if err := reordered.validate(cats, items); err != nil {
		t.Fatalf("configurable display ranks rejected: %v", err)
	}
	// LoadRecommender and the standalone validator share required policy validation.
	root := t.TempDir()
	rest := filepath.Join(root, "rec")
	if err := os.MkdirAll(filepath.Join(rest, "recommender"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"menu.json", "recommender/matrix.json", "recommender/heuristics.json", "recommender/upsell_map.json", "recommender/forcart_priorities.json"} {
		data, err := os.ReadFile(filepath.Join("testdata/rec", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rest, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	policyPath := filepath.Join(rest, "recommender/recommendation_policy.json")
	fixturePolicy, err := os.ReadFile(filepath.Join("testdata/rec/recommender", "recommendation_policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture RecommendationPolicy
	if err := json.Unmarshal(fixturePolicy, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, fixturePolicy, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecommender(root, "rec"); err != nil {
		t.Fatalf("valid synthetic policy rejected at startup: %v", err)
	}
	invalidPolicies := []string{"", `{}`, `{"unknown_field":1}`, `{} {}`}
	for _, mutate := range []func(*RecommendationPolicy){
		func(p *RecommendationPolicy) { p.SoupBakery = nil },
		func(p *RecommendationPolicy) { p.SubstantialCategories = nil },
		func(p *RecommendationPolicy) { p.NoMainAnchorCategories = nil },
	} {
		p := fixture
		mutate(&p)
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		invalidPolicies = append(invalidPolicies, string(raw))
	}
	for _, raw := range invalidPolicies {
		if raw == "" {
			if err := os.Remove(policyPath); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(policyPath, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := LoadRecommender(root, "rec"); err == nil {
			t.Fatalf("startup accepted policy %q", raw)
		}
		report, err := ValidateFiles(root, "rec")
		if err != nil || !report.HasErrors() {
			t.Fatalf("validator accepted policy: %+v, %v", report, err)
		}
	}
}
