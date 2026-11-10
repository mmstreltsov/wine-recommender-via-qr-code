package aidabaran

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"svoyshef/internal/domain"
)

// FR-10/11/12/28/29, О2/О3: regression of the complete edited pilot, not synthetic fixtures.
func TestFR10_RealDataGastronomyAllCards(t *testing.T) {
	r, err := LoadRecommender("../../../data", "aidabaran")
	if err != nil {
		t.Fatal(err)
	}
	menu, err := LoadMenu("../../../data", "aidabaran")
	if err != nil {
		t.Fatal(err)
	}
	var matrix Matrix
	if err := loadJSONFile("../../../data/aidabaran/recommender/matrix.json", &matrix); err != nil {
		t.Fatal(err)
	}
	if len(menu.Items) != 125 || len(menu.Categories) != 14 || len(matrix.Pairs) != 414 {
		t.Fatalf("want 125 items/14 categories/414 pairs, got %d/%d/%d", len(menu.Items), len(menu.Categories), len(matrix.Pairs))
	}
	items := make(map[int64]domain.MenuItem, len(menu.Items))
	for _, item := range menu.Items {
		items[item.ID] = item
	}
	// Explicit menu UID ledger: large/shared portions and whole fish/chicken of unknown yield.
	bannedTargets := map[int64]bool{
		876201943922: true, 569159451603: true, 507171197452: true,
		283776706382: true, 456357387792: true, 306795817632: true,
		717994604062: true, 500004468472: true, 406838418802: true,
		977604609092: true, 292582418782: true, 914964949052: true,
		513971294142: true, 303938634302: true, 988436681252: true,
		759806895422: true, 159551626192: true, 761682796242: true,
		173154936692: true, 293109021772: true, 605443137872: true,
	}
	// Named included sauces; no free-form language/portion inference in this test.
	withSauce := map[int64]bool{
		904076493563: true, 270034433122: true, 988436681252: true,
		759806895422: true, 159551626192: true, 183588997633: true,
		944299729662: true, 357866772752: true, 219414720772: true,
		662047004422: true, 753190587262: true, 797295405062: true,
		269543400942: true, 709247437982: true, 577945503202: true,
		100824680423: true,
	}
	pairs := make(map[[2]int64]matrixPair, len(matrix.Pairs))
	for _, pair := range matrix.Pairs {
		key := [2]int64{pair.DishID, pair.RecID}
		from, fromOK := items[pair.DishID]
		to, toOK := items[pair.RecID]
		if _, duplicate := pairs[key]; duplicate || !fromOK || !toOK || from.ID == to.ID || from.CategoryID == to.CategoryID || pair.CategoryOverride != "" || pair.Source != "expert" {
			t.Fatalf("invalid/duplicate expert pair: %+v", pair)
		}
		if bannedTargets[to.ID] || (withSauce[from.ID] && (to.CategoryID == categoryIDs["Соусы"] || withSauce[to.ID])) || (from.CategoryID == categoryIDs["Соусы"] && withSauce[to.ID]) {
			t.Fatalf("large/shared target or second included sauce: %d -> %d", from.ID, to.ID)
		}
		assertTextBounds(t, pair.ReasonText)
		pairs[key] = pair
	}
	targetCategories := make(map[int]bool)
	selected := make(map[[2]int64]bool, len(pairs))
	for _, dish := range menu.Items {
		t.Run(fmt.Sprint(dish.ID), func(t *testing.T) {
			recs, err := r.ForDish(context.Background(), dish.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(recs) < 3 {
				t.Fatalf("want at least 3 distinct target categories per source, got %d recommendations", len(recs))
			}
			seenIDs, seenCategories := map[int64]bool{}, map[int]bool{}
			for i, rec := range recs {
				target, exists := items[rec.ItemID]
				key := [2]int64{dish.ID, rec.ItemID}
				pair, paired := pairs[key]
				if !exists || !paired || rec.ItemID == dish.ID || target.CategoryID == dish.CategoryID || seenIDs[rec.ItemID] || seenCategories[target.CategoryID] || rec.Source != "expert" || rec.Score != pair.Score || rec.ReasonText != pair.ReasonText {
					t.Fatalf("invalid or non-expert selected candidate: %+v", rec)
				}
				if i > 0 {
					prev := recs[i-1]
					if prev.Score < rec.Score || (prev.Score == rec.Score && items[prev.ItemID].CategoryID > target.CategoryID) {
						t.Fatalf("wrong score/category order: %+v", recs)
					}
				}
				assertTextBounds(t, rec.ReasonText)
				seenIDs[rec.ItemID], seenCategories[target.CategoryID] = true, true
				targetCategories[target.CategoryID], selected[key] = true, true
			}
			filtered, err := r.ForDish(context.Background(), dish.ID, recIDs(recs))
			if err != nil || len(filtered) != 0 {
				t.Fatalf("FR-28/29: excluded top-1 candidates must not be replaced: %v, %v", recIDs(filtered), err)
			}
		})
	}
	if len(selected) != 414 {
		t.Errorf("want all 414 expert pairs selected, got %d", len(selected))
	}
	for _, category := range menu.Categories {
		if !targetCategories[category.ID] {
			t.Errorf("category %d %q never targeted", category.ID, category.Name)
		}
	}
	t.Logf("125 cards; %d selected expert candidates; %d target categories; no fallback replacements", len(selected), len(targetCategories))
}

// FR-10/19/20/21, О3: additions to a named complete dish are not a second main or sauce.
func TestFR19_RealDataGastronomyDirectedAdditions(t *testing.T) {
	r, menu := realData(t)
	items := make(map[int64]domain.MenuItem, len(menu.Items))
	for _, item := range menu.Items {
		items[item.ID] = item
	}
	for _, tc := range []struct {
		name string
		dish int64
		feed []int64
		cats []int
		cart int64
	}{
		{"khinkali_no_dolma_or_600g_salad", 655892482392, []int64{366609864022, 733465955822, 231821651422, 445870875313, 951817158352}, []int{1, 10, 11, 14, 12}, 445870875313},
		{"grill_satsebeli_no_second_sauce", 219414720772, []int64{573190167622, 693049741272, 844016957853}, []int{10, 1, 14}, 844016957853},
		{"grill_mint_sauce_and_salad", 577945503202, []int64{588160226782, 445870875313, 951817158352}, []int{2, 14, 12}, 445870875313},
		{"dorado_grill_pomegranate_sauce", 759806895422, []int64{693049741272, 301501361763, 939802252842}, []int{1, 14, 12}, 301501361763},
		{"dorado_steam_pomegranate_sauce", 159551626192, []int64{876369952933, 301501361763, 951817158352}, []int{1, 14, 12}, 301501361763},
		{"cod_sauce_and_vegetables_no_new_hot_dish", 183588997633, []int64{267806359892, 301501361763, 951817158352}, []int{1, 14, 12}, 301501361763},
		{"salmon_included_zucchini", 538899551122, []int64{366609864022, 301501361763, 939802252842}, []int{1, 14, 12}, 301501361763},
		{"ribs_for_group", 988436681252, []int64{267806359892, 301501361763, 939802252842}, []int{1, 14, 12}, 301501361763},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recs, err := r.ForDish(context.Background(), tc.dish, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := recIDs(recs); !reflect.DeepEqual(got, tc.feed) {
				t.Fatalf("ordered feed=%v, want %v", got, tc.feed)
			}
			for i, rec := range recs {
				if items[rec.ItemID].CategoryID != tc.cats[i] || rec.Source != "expert" {
					t.Errorf("rec %d: category=%d source=%s", rec.ItemID, items[rec.ItemID].CategoryID, rec.Source)
				}
				assertTextBounds(t, rec.ReasonText)
			}
			rec, err := r.ForCart(context.Background(), cartLines(tc.dish), nil)
			if err != nil {
				t.Fatal(err)
			}
			if rec == nil || rec.ItemID != tc.cart || rec.Source != "expert" || items[rec.ItemID].CategoryID != 14 || rec.ReasonText != findRec(t, recs, tc.cart).ReasonText {
				t.Fatalf("ForCart want %d/expert/drinks with same reason, got %+v", tc.cart, rec)
			}
		})
	}
}

// FR-10/28/29: exclusions affect one call, not the stored candidates or categories.
func TestFR28_RealDataGastronomyExclusionsDoNotPersist(t *testing.T) {
	r, _ := realData(t)
	ctx := context.Background()
	const dishID = 655892482392
	original, err := r.ForDish(ctx, dishID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(original) != 5 {
		t.Fatalf("want 5 original khinkali categories, got %d", len(original))
	}
	candidateIDs := recIDs(original)
	filtered, err := r.ForDish(ctx, dishID, candidateIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Fatalf("excluded candidates must leave no categories, got %v", recIDs(filtered))
	}
	restored, err := r.ForDish(ctx, dishID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, original) {
		t.Fatalf("nil exclusions must restore the complete original feed: got %+v, want %+v", restored, original)
	}
}
