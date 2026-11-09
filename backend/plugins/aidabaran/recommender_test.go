package aidabaran

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"svoyshef/internal/domain"
	"svoyshef/internal/recommender"
)

func fixtureRecommender(t *testing.T) recommender.Recommender {
	t.Helper()
	r, err := LoadRecommender("testdata", "rec")
	if err != nil {
		t.Fatalf("LoadRecommender fixture: %v", err)
	}
	return r
}

func cartLines(ids ...int64) []domain.CartLine {
	lines := make([]domain.CartLine, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, domain.CartLine{ItemID: id, Qty: 1, Available: true})
	}
	return lines
}

func recIDs(recs []recommender.Rec) []int64 {
	ids := make([]int64, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.ItemID)
	}
	return ids
}

func findRec(t *testing.T, recs []recommender.Rec, id int64) recommender.Rec {
	t.Helper()
	for _, r := range recs {
		if r.ItemID == id {
			return r
		}
	}
	t.Fatalf("rec %d not found in %v", id, recIDs(recs))
	return recommender.Rec{}
}

func assertNoStopWords(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, w := range stopWords {
		if strings.Contains(lower, w) {
			t.Errorf("стоп-слово %q в тексте %q", w, text)
		}
	}
}

func assertTextBounds(t *testing.T, text string) {
	t.Helper()
	n := utf8.RuneCountInString(text)
	if n < reasonMinLen || n > reasonMaxLen {
		t.Errorf("длина текста %d вне [%d,%d]: %q", n, reasonMinLen, reasonMaxLen, text)
	}
	assertNoStopWords(t, text)
}

func TestFR10_ExpertMaxScoreAndSortOrderTieBreak(t *testing.T) {
	r := fixtureRecommender(t)
	recs, err := r.ForDish(context.Background(), 101, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	got := recIDs(recs)
	want := []int64{402, 701, 501, 601}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recs = %v, want %v", got, want)
	}
	wantScore := []float64{0.9, 0.85, 0.75, 0.7}
	for i, rec := range recs {
		if rec.Score != wantScore[i] {
			t.Errorf("rec %d score = %v, want %v", rec.ItemID, rec.Score, wantScore[i])
		}
		if rec.Source != "expert" {
			t.Errorf("rec %d source = %q, want expert", rec.ItemID, rec.Source)
		}
		if rec.ReasonText == "" {
			t.Errorf("rec %d: пустой текст обоснования", rec.ItemID)
		}
	}
	if findRec(t, recs, 402).ReasonText != "Ткемали с вишнёвой кислотой держит свежесть овощей и оставляет после салата чистое травяное послевкусие." {
		t.Errorf("текст expert-пары берётся из матрицы: %q", findRec(t, recs, 402).ReasonText)
	}
	for _, absent := range []int64{401, 502} {
		for _, id := range got {
			if id == absent {
				t.Errorf("позиция %d не должна быть в ленте", absent)
			}
		}
	}
}

func TestFR10_CategoryOverride(t *testing.T) {
	r := fixtureRecommender(t)
	eng := r.(*engine)
	dish := eng.items[101]
	cand, ok := eng.cascade(dish, 5)
	if !ok || cand.item.ID != 601 {
		t.Fatalf("cascade(101, cat Хинкали) = %+v ok=%v, want item 601 (override)", cand.item.ID, ok)
	}
	if _, ok := eng.cascade(dish, 13); ok {
		t.Errorf("cascade(101, cat Тандырный хлеб) должен быть пуст: пара 601 переназначена override-ом")
	}
	cand, ok = eng.cascade(dish, 14)
	if !ok || cand.item.ID != 701 {
		t.Fatalf("cascade(101, cat Напитки) = item %d ok=%v, want 701 (несуществующий override -> категория rec_id)", cand.item.ID, ok)
	}
}

func TestFR10_UpsellBranchFirstByOrder(t *testing.T) {
	r := fixtureRecommender(t)
	recs, err := r.ForDish(context.Background(), 201, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	got := recIDs(recs)
	want := []int64{701, 402, 501}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recs = %v, want %v", got, want)
	}
	up := findRec(t, recs, 402)
	if up.Source != "upsell" || up.Score != 0.6 {
		t.Errorf("upsell-кандидат = source %q score %v, want upsell 0.6", up.Source, up.Score)
	}
	for _, id := range got {
		if id == 401 {
			t.Errorf("при двух опциях одной категории берётся первая по порядку upsell_options (402), а не 401")
		}
	}
}

func TestFR10_HeuristicBranchAndRuleMiss(t *testing.T) {
	r := fixtureRecommender(t)
	recs, err := r.ForDish(context.Background(), 203, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	got := recIDs(recs)
	want := []int64{401, 501, 701}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recs = %v, want %v (равный score -> ID категории ASC)", got, want)
	}
	for _, rec := range recs {
		if rec.Source != "heuristic" || rec.Score != 0.4 {
			t.Errorf("rec %d = source %q score %v, want heuristic 0.4", rec.ItemID, rec.Source, rec.Score)
		}
	}
	for _, absent := range []int64{101, 102, 301, 601, 702} {
		for _, id := range got {
			if id == absent {
				t.Errorf("категория без правила/пар не должна давать кандидата: %d", absent)
			}
		}
	}
}

func TestFR10_Limit13(t *testing.T) {
	const n = 15
	cats := make([]domain.Category, 0, n)
	items := make([]domain.MenuItem, 0, n)
	tos := make([]string, 0, n-1)
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("Категория %02d", i)
		cats = append(cats, domain.Category{ID: i, Name: name})
		items = append(items, domain.MenuItem{ID: int64(1000 + i), CategoryID: i, Name: fmt.Sprintf("Блюдо %02d", i), Price: 100})
		if i > 1 {
			tos = append(tos, name)
		}
	}
	h := Heuristics{Rules: []heuristicRule{{FromCategory: "Категория 01", ToCategories: tos}}}
	r := NewRecommender(cats, items, Matrix{}, h, UpsellMap{}, CoursePriorities{})
	recs, err := r.ForDish(context.Background(), 1001, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	if len(recs) != forDishLimit {
		t.Fatalf("len = %d, want %d", len(recs), forDishLimit)
	}
	want := []int64{1002, 1003, 1004, 1005, 1006, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014}
	if got := recIDs(recs); !reflect.DeepEqual(got, want) {
		t.Fatalf("recs = %v, want первые 13 категорий по ID", got)
	}
	assertTextBounds(t, recs[0].ReasonText)
}

func TestFR28_ForDishExcludesInCart(t *testing.T) {
	r := fixtureRecommender(t)
	recs, err := r.ForDish(context.Background(), 101, []int64{402})
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	got := recIDs(recs)
	want := []int64{701, 501, 601}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recs = %v, want %v", got, want)
	}
	for _, id := range got {
		if id == 401 || id == 402 {
			t.Errorf("исключённый топ-1 убирает категорию целиком, замена не ищется: %d", id)
		}
	}
	recs, err = r.ForDish(context.Background(), 201, []int64{701})
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	if got := recIDs(recs); !reflect.DeepEqual(got, []int64{402, 501}) {
		t.Fatalf("recs = %v, want [402 501]", got)
	}
}

func TestFR29_EmptyFeedForItemWithoutPairs(t *testing.T) {
	r := fixtureRecommender(t)
	recs, err := r.ForDish(context.Background(), 301, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	if recs == nil || len(recs) != 0 {
		t.Fatalf("позиция без пар -> пустой список, got %v", recs)
	}
	recs, err = r.ForDish(context.Background(), 999999, nil)
	if err != nil {
		t.Fatalf("ForDish неизвестный item: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("неизвестный item -> пустой список, got %v", recIDs(recs))
	}
}

func TestFR10_ExcludesSelfItemID(t *testing.T) {
	cats := []domain.Category{
		{ID: 1, Name: "Салаты"},
		{ID: 2, Name: "Напитки"},
	}
	items := []domain.MenuItem{
		{ID: 10, CategoryID: 1, Name: "Салат по-грузински", Price: 100},
		{ID: 20, CategoryID: 2, Name: "Тархун", Price: 50},
	}
	m := Matrix{Pairs: []matrixPair{{
		DishID:           10,
		RecID:            10,
		CategoryOverride: "Напитки",
		Score:            0.9,
		Source:           "expert",
		ReasonText:       "Пара-самоссылка через override: движок обязан исключить сам itemID и скрыть категорию целиком, замена не ищется.",
	}}}
	h := Heuristics{Rules: []heuristicRule{{FromCategory: "Салаты", ToCategories: []string{"Напитки"}}}}
	r := NewRecommender(cats, items, m, h, UpsellMap{}, CoursePriorities{})
	recs, err := r.ForDish(context.Background(), 10, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	if got := recIDs(recs); len(got) != 0 {
		t.Fatalf("сам itemID исключён, категория умирает без замены, got %v", got)
	}
}

func TestFR19_EmptyCartNil(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec != nil {
		t.Fatalf("пустая корзина -> nil, got %+v", rec)
	}
}

func TestFR19_ExpertIntoUncoveredCourse(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), cartLines(201), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 701 || rec.Source != "expert" || rec.Score != 0.7 {
		t.Fatalf("want 701 expert 0.7, got %+v", rec)
	}
}

func TestFR19_UpsellFallbackIntoUncovered(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), cartLines(202), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 702 || rec.Source != "upsell" || rec.Score != 0.6 {
		t.Fatalf("want 702 upsell 0.6, got %+v", rec)
	}
}

func TestFR19_HeuristicTopFallback(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), cartLines(203), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 701 || rec.Source != "heuristic" || rec.Score != 0.4 {
		t.Fatalf("want 701 heuristic 0.4 (топ непокрытой категории), got %+v", rec)
	}
}

func TestFR20_RecChangesWithCart(t *testing.T) {
	r := fixtureRecommender(t)
	first, err := r.ForCart(context.Background(), cartLines(201), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if first == nil || first.ItemID != 701 {
		t.Fatalf("корзина [201] -> 701, got %+v", first)
	}
	second, err := r.ForCart(context.Background(), cartLines(201, 701), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if second == nil || second.ItemID != 501 || second.Source != "heuristic" {
		t.Fatalf("корзина [201,701] -> 501 heuristic (Десерты непокрыты), got %+v", second)
	}
}

func TestFR21_AllCoursesCoveredExpertAnyCategory(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), cartLines(401, 203, 701, 501, 101), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 402 || rec.Source != "expert" || rec.Score != 0.9 {
		t.Fatalf("want 402 expert 0.9 (категория уже в корзине, позиция другая), got %+v", rec)
	}
}

func TestFR21_HeuristicFallbackWhenNoPairs(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), cartLines(203, 401, 402, 701, 501, 502, 601, 101), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 201 || rec.Source != "heuristic" || rec.Score != 0.4 {
		t.Fatalf("все пары последней позиции в корзине -> heuristic 201, got %+v", rec)
	}
}

func TestFR21_GroupCategoriesAscendingID(t *testing.T) {
	cats := []domain.Category{
		{ID: 1, Name: "Хинкали"},
		{ID: 4, Name: "Салаты"},
		{ID: 7, Name: "Холодные закуски"},
		{ID: 9, Name: "Напитки"},
	}
	items := []domain.MenuItem{
		{ID: 100, CategoryID: 1, Name: "Хинкали с говядиной", Price: 480, SortOrder: 10},
		{ID: 401, CategoryID: 4, Name: "Салат по-грузински", Price: 590, SortOrder: 10},
		{ID: 400, CategoryID: 4, Name: "Салат из печных овощей", Price: 640, SortOrder: 20},
		{ID: 700, CategoryID: 7, Name: "Рулетики из баклажана", Price: 520, SortOrder: 10},
		{ID: 900, CategoryID: 9, Name: "Тархун", Price: 260, SortOrder: 10},
	}
	h := Heuristics{Rules: []heuristicRule{{
		FromCategory: "Хинкали",
		ToCategories: []string{"Холодные закуски", "Салаты", "Напитки"},
	}}}
	prio := CoursePriorities{CoursePriority: [][]string{
		{"Холодные закуски", "Салаты"},
		{"Напитки"},
	}}
	r := NewRecommender(cats, items, Matrix{}, h, UpsellMap{}, prio)
	ctx := context.Background()

	rec, err := r.ForCart(ctx, cartLines(100), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 401 || rec.Source != "heuristic" || rec.Score != scoreHeuristic {
		t.Fatalf("группа объявлена [Холодные закуски(7), Салаты(4)], to_categories тоже с 7 первым; при двух живых кандидатах выигрывает категория с меньшим ID: want 401 heuristic 0.4, got %+v", rec)
	}
	if rec.ReasonText == "" {
		t.Errorf("rec %d: пустой текст обоснования (FR-11)", rec.ItemID)
	}

	rec, err = r.ForCart(ctx, cartLines(100), []int64{401})
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 700 || rec.Source != "heuristic" {
		t.Fatalf("топ Салаты(4) исключён -> категория гаснет целиком, берётся топ следующей категории группы по ID ASC: want 700, got %+v", rec)
	}

	rec, err = r.ForCart(ctx, cartLines(100), []int64{401, 700})
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec != nil {
		t.Fatalf("обе категории группы исчерпаны -> nil без переключения на следующий курс (Напитки жив, 900), got %+v", rec)
	}
}

func TestFR28_ForCartSkipsCartPositions(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), cartLines(203, 701, 501, 402, 101), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil || rec.ItemID != 401 || rec.Score != 0.8 {
		t.Fatalf("402 в корзине -> следующая по каскаду пара 401 (0.8), got %+v", rec)
	}
}

func TestFR29_ForCartNoCandidateNil(t *testing.T) {
	r := fixtureRecommender(t)
	rec, err := r.ForCart(context.Background(), cartLines(301), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec != nil {
		t.Fatalf("нет пар/опций/правил -> nil, got %+v", rec)
	}
}

func TestFR10_DeterministicOutput(t *testing.T) {
	r := fixtureRecommender(t)
	ctx := context.Background()
	a, err := r.ForDish(ctx, 101, []int64{402})
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	b, err := r.ForDish(ctx, 101, []int64{402})
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("ForDish недетерминизм: %v vs %v", a, b)
	}
	ca, err := r.ForCart(ctx, cartLines(201, 701), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	cb, err := r.ForCart(ctx, cartLines(201, 701), nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if !reflect.DeepEqual(ca, cb) {
		t.Fatalf("ForCart недетерминизм: %+v vs %+v", ca, cb)
	}
}

func TestFR11_TextsForEverySource(t *testing.T) {
	r := fixtureRecommender(t)
	recs, err := r.ForDish(context.Background(), 201, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	for _, rec := range recs {
		if rec.ReasonText == "" {
			t.Errorf("rec %d: пустой текст (FR-11)", rec.ItemID)
		}
		if rec.Source == "upsell" || rec.Source == "heuristic" {
			assertTextBounds(t, rec.ReasonText)
		}
	}
}

func TestFR11_TemplateBoundsAllCategories(t *testing.T) {
	cats := make([]string, 0, len(reasonTemplates)+1)
	for name := range reasonTemplates {
		cats = append(cats, name)
	}
	cats = append(cats, "Такой категории нет")
	for _, name := range cats {
		assertTextBounds(t, pairText("Баже", "Ткемали", "Соусы", name))
	}
}

func nameOfLen(n int) string {
	r := []rune("хинкали с говядиной и свининой рулетики из баклажана и сладкого перца с орехом")
	for len(r) < n {
		r = append(r, 'а')
	}
	return string(r[:n])
}

func TestFR12_LongNamesFit220(t *testing.T) {
	long71 := nameOfLen(71)
	long67 := nameOfLen(67)
	cats := make([]string, 0, len(reasonTemplates)+1)
	for name := range reasonTemplates {
		cats = append(cats, name)
	}
	cats = append(cats, "Такой категории нет")
	for _, name := range cats {
		assertTextBounds(t, pairText(long71, long67, "Супы", name))
	}
	ultra1 := nameOfLen(80)
	ultra2 := nameOfLen(80)
	for _, name := range []string{"Хинкали", "Напитки", "Такой категории нет"} {
		assertTextBounds(t, pairText(ultra1, ultra2, "Супы", name))
	}
	huge1 := nameOfLen(95)
	huge2 := nameOfLen(95)
	for _, name := range []string{"Хинкали", "Напитки", "Такой категории нет"} {
		assertTextBounds(t, pairText(huge1, huge2, "Супы", name))
	}
}

func realData(t *testing.T) (recommender.Recommender, *LoadResult) {
	t.Helper()
	const dataDir = "../../../data"
	if _, err := os.Stat(filepath.Join(dataDir, "aidabaran", "menu.json")); err != nil {
		t.Skip("реальные данные недоступны")
	}
	r, err := LoadRecommender(dataDir, "aidabaran")
	if err != nil {
		t.Fatalf("LoadRecommender real: %v", err)
	}
	menu, err := LoadMenu(dataDir, "aidabaran")
	if err != nil {
		t.Fatalf("LoadMenu real: %v", err)
	}
	return r, menu
}

func TestFR10_RealDataSmoke(t *testing.T) {
	r, menu := realData(t)
	items := make(map[int64]domain.MenuItem, len(menu.Items))
	cats := make(map[int]string, len(menu.Categories))
	for _, it := range menu.Items {
		items[it.ID] = it
	}
	for _, c := range menu.Categories {
		cats[c.ID] = c.Name
	}
	const dishID = 655892482392
	dish := items[dishID]
	if dish.CategoryID != categoryIDs["Хинкали"] {
		t.Fatalf("smoke-блюдо %d не в категории Хинкали", dishID)
	}
	recs, err := r.ForDish(context.Background(), dishID, nil)
	if err != nil {
		t.Fatalf("ForDish: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("пустая лента для реального хинкали")
	}
	for _, rec := range recs {
		it := items[rec.ItemID]
		if it.CategoryID == dish.CategoryID {
			t.Errorf("rec %d из категории самого блюда", rec.ItemID)
		}
		t.Logf("SMOKE ForDish %q -> %q [%s] <%s> %s", dish.Name, it.Name, cats[it.CategoryID], rec.Source, rec.ReasonText)
		assertTextBounds(t, rec.ReasonText)
	}
}

func TestFR19_RealDataSmoke(t *testing.T) {
	r, menu := realData(t)
	items := make(map[int64]domain.MenuItem, len(menu.Items))
	cats := make(map[int]string, len(menu.Categories))
	for _, it := range menu.Items {
		items[it.ID] = it
	}
	for _, c := range menu.Categories {
		cats[c.ID] = c.Name
	}
	lines := cartLines(329216982342, 977604609092)
	rec, err := r.ForCart(context.Background(), lines, nil)
	if err != nil {
		t.Fatalf("ForCart: %v", err)
	}
	if rec == nil {
		t.Fatal("ForCart nil для корзины без напитков")
	}
	it := items[rec.ItemID]
	if cats[it.CategoryID] != drinksCategory {
		t.Errorf("категория кандидата = %q, want %q", cats[it.CategoryID], drinksCategory)
	}
	t.Logf("SMOKE ForCart [Сацебели, Хинкали дегустационный сет] -> %q [%s] <%s %.2f> %s", it.Name, cats[it.CategoryID], rec.Source, rec.Score, rec.ReasonText)
}
