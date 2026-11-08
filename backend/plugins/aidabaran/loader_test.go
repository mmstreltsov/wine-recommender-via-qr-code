package aidabaran

import (
	"os"
	"testing"

	"svoyshef/internal/domain"
)

func loadFixture(t *testing.T) *LoadResult {
	t.Helper()
	res, err := LoadMenu("testdata", "aidabaran")
	if err != nil {
		t.Fatalf("LoadMenu: %v", err)
	}
	return res
}

func itemByID(t *testing.T, res *LoadResult, id int64) domain.MenuItem {
	t.Helper()
	for _, it := range res.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("item %d not found", id)
	return domain.MenuItem{}
}

func TestFR02_BasicMapping(t *testing.T) {
	res := loadFixture(t)
	wantCats := []domain.Category{
		{ID: 1, Name: "Салаты"},
		{ID: 11, Name: "Соусы"},
		{ID: 14, Name: "Напитки"},
	}
	if len(res.Categories) != len(wantCats) {
		t.Fatalf("categories count = %d, want %d", len(res.Categories), len(wantCats))
	}
	for i, c := range res.Categories {
		if c != wantCats[i] {
			t.Errorf("categories[%d] = %+v, want %+v", i, c, wantCats[i])
		}
	}
	it := itemByID(t, res, 101)
	if it.CategoryID != 1 || it.Name != "Салат с ореховой заправкой" || it.Price != 1390 {
		t.Errorf("item 101 basic fields = %+v", it)
	}
	if it.Description != "600 гр. подано с зеленью" {
		t.Errorf("description = %q, want unescaped and collapsed", it.Description)
	}
	if it.KBJU != "Калорийность: 569,1 ккал Белки: 11,2 г Жиры: 48,3 г" {
		t.Errorf("kbju = %q, want prefix stripped", it.KBJU)
	}
	if it.PhotoURL != "https://cdn.test/salat.jpg" {
		t.Errorf("photo url = %q", it.PhotoURL)
	}
	if it.PhotoFallback != "/static/aidabaran/photos/dishes/salat.jpg" {
		t.Errorf("photo fallback = %q", it.PhotoFallback)
	}
}

func TestFR02_SkipNoPriceEmptyTitleAndUnknownCategory(t *testing.T) {
	res := loadFixture(t)
	if len(res.Items) != 10 {
		t.Fatalf("items count = %d, want 10 (3 skipped)", len(res.Items))
	}
	byUID := map[string]int64{"price_null": 102, "title_empty": 103, "category_unknown": 105}
	got := map[int64]string{}
	for _, s := range res.Skipped {
		got[s.UID] = s.Reason
	}
	if len(res.Skipped) != 3 {
		t.Fatalf("skipped count = %d, want 3: %+v", len(res.Skipped), res.Skipped)
	}
	for reason, uid := range byUID {
		if got[uid] != reason {
			t.Errorf("skip reason for uid %d = %q, want %q", uid, got[uid], reason)
		}
	}
	for _, it := range res.Items {
		if it.ID == 102 || it.ID == 103 || it.ID == 105 {
			t.Errorf("skipped uid %d present in items", it.ID)
		}
	}
}

func TestFR02_KBJURules(t *testing.T) {
	res := loadFixture(t)
	drinkMerged := itemByID(t, res, 201)
	if drinkMerged.Description != "200 мл Освежающий напиток со льдом" {
		t.Errorf("drink description = %q, want kbju appended", drinkMerged.Description)
	}
	if drinkMerged.KBJU != "" {
		t.Errorf("drink kbju = %q, want empty", drinkMerged.KBJU)
	}
	drinkOnly := itemByID(t, res, 202)
	if drinkOnly.Description != "Сладкий чай" {
		t.Errorf("drink empty-desc = %q, want kbju only", drinkOnly.Description)
	}
	drinkCalories := itemByID(t, res, 203)
	if drinkCalories.KBJU != "Калорийность: 45 ккал на 100 мл" {
		t.Errorf("drink with calories kbju = %q", drinkCalories.KBJU)
	}
	if drinkCalories.Description != "Холодный чай" {
		t.Errorf("drink with calories description = %q", drinkCalories.Description)
	}
	foodNoKbju := itemByID(t, res, 104)
	if foodNoKbju.KBJU != "" || foodNoKbju.Description != "" {
		t.Errorf("non-drink unrecognized kbju: kbju=%q description=%q, want empty/empty", foodNoKbju.KBJU, foodNoKbju.Description)
	}
	noPrefixUpper := itemByID(t, res, 304)
	if noPrefixUpper.KBJU != "КАЛОРИЙНОСТЬ: 80 ккал" {
		t.Errorf("kbju case-insensitive = %q", noPrefixUpper.KBJU)
	}
}

func TestFR02_PhotoFallbackChain(t *testing.T) {
	res := loadFixture(t)
	fromEditions := itemByID(t, res, 301)
	if fromEditions.PhotoURL != "https://cdn.test/sause-fb.jpg" {
		t.Errorf("editions fallback url = %q", fromEditions.PhotoURL)
	}
	noPhoto := itemByID(t, res, 302)
	if noPhoto.PhotoURL != "" {
		t.Errorf("no photo url = %q, want empty", noPhoto.PhotoURL)
	}
	missingLocal := itemByID(t, res, 303)
	if missingLocal.PhotoFallback != "" {
		t.Errorf("missing local photo fallback = %q, want empty", missingLocal.PhotoFallback)
	}
	blankPhotoWithEditions := itemByID(t, res, 305)
	if blankPhotoWithEditions.PhotoURL != "https://cdn.test/sause-ed.jpg" {
		t.Errorf("editions url for blank photo = %q", blankPhotoWithEditions.PhotoURL)
	}
}

func TestFR02_UpsellTrailingPriceStripped(t *testing.T) {
	res := loadFixture(t)
	it := itemByID(t, res, 101)
	if len(it.UpsellOptions) != 2 {
		t.Fatalf("upsell count = %d, want 2", len(it.UpsellOptions))
	}
	if got, want := it.UpsellOptions[0].Name, "Тигровые креветки (50г)"; got != want {
		t.Errorf("upsell[0].name = %q, want %q", got, want)
	}
	if it.UpsellOptions[0].AddPrice != 420 {
		t.Errorf("upsell[0].add_price = %d, want 420", it.UpsellOptions[0].AddPrice)
	}
	if got := it.UpsellOptions[1].Name; got != "Просто опция без цены" {
		t.Errorf("upsell[1].name = %q, want unchanged", got)
	}
}

func TestFR02_SortOrderWithinCategory(t *testing.T) {
	res := loadFixture(t)
	cases := []struct {
		id   int64
		want int
	}{
		{101, 0}, {104, 1},
		{201, 0}, {202, 1}, {203, 2},
		{301, 0}, {302, 1}, {303, 2}, {304, 3}, {305, 4},
	}
	for _, c := range cases {
		if got := itemByID(t, res, c.id).SortOrder; got != c.want {
			t.Errorf("item %d sort order = %d, want %d", c.id, got, c.want)
		}
	}
}

func TestFR02_RealMenu(t *testing.T) {
	const path = "../../../data/aidabaran/menu.json"
	if _, err := os.Stat(path); err != nil {
		t.Skip("real menu.json not available")
	}
	res, err := LoadMenu("../../../data", "aidabaran")
	if err != nil {
		t.Fatalf("LoadMenu real: %v", err)
	}
	if len(res.Items) != 125 {
		t.Errorf("items = %d, want 125", len(res.Items))
	}
	if len(res.Categories) != 14 {
		t.Errorf("categories = %d, want 14", len(res.Categories))
	}
	if len(res.Skipped) != 0 {
		t.Errorf("skipped = %d, want 0: %+v", len(res.Skipped), res.Skipped)
	}
	noPhoto := 0
	for _, it := range res.Items {
		if it.PhotoURL == "" {
			noPhoto++
		}
	}
	if noPhoto != 0 {
		t.Errorf("items without photo url = %d, want 0", noPhoto)
	}
}
