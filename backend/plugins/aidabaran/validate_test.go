package aidabaran

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"svoyshef/internal/domain"
)

func testIndex() *menuIndex {
	cats := []domain.Category{
		{ID: 1, Name: "Салаты"},
		{ID: 2, Name: "Супы"},
		{ID: 3, Name: "Соусы"},
		{ID: 4, Name: "Напитки"},
	}
	items := []domain.MenuItem{
		{ID: 100, CategoryID: 1},
		{ID: 200, CategoryID: 2},
		{ID: 300, CategoryID: 3},
		{ID: 400, CategoryID: 4},
	}
	return buildMenuIndex(cats, items)
}

func validPair() matrixPair {
	return matrixPair{DishID: 100, RecID: 300, Score: 0.9, Source: "expert", ReasonText: strings.Repeat("а", 80)}
}

func assertFindings(t *testing.T, got, want []Finding) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("findings = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("findings[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFR10_MatrixPairChecks(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*matrixPair)
		want   Finding
	}{
		{
			"dish_id not in menu",
			func(p *matrixPair) { p.DishID = 999 },
			Finding{SeverityError, "matrix dish_id=999 rec_id=300: dish_id не найден в меню"},
		},
		{
			"rec_id not in menu",
			func(p *matrixPair) { p.RecID = 999 },
			Finding{SeverityError, "matrix dish_id=100 rec_id=999: rec_id не найден в меню"},
		},
		{
			"self pair",
			func(p *matrixPair) { p.RecID = 100 },
			Finding{SeverityError, "matrix dish_id=100 rec_id=100: dish_id = rec_id"},
		},
		{
			"score above one",
			func(p *matrixPair) { p.Score = 1.5 },
			Finding{SeverityError, "matrix dish_id=100 rec_id=300: score 1.5 вне [0,1]"},
		},
		{
			"score below zero",
			func(p *matrixPair) { p.Score = -0.1 },
			Finding{SeverityError, "matrix dish_id=100 rec_id=300: score -0.1 вне [0,1]"},
		},
		{
			"reason too short",
			func(p *matrixPair) { p.ReasonText = "коротко" },
			Finding{SeverityError, "matrix dish_id=100 rec_id=300: reason_text 7 симв., нужно 60–220"},
		},
		{
			"reason too long",
			func(p *matrixPair) { p.ReasonText = strings.Repeat("б", 221) },
			Finding{SeverityError, "matrix dish_id=100 rec_id=300: reason_text 221 симв., нужно 60–220"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validPair()
			c.mutate(&p)
			fs, covered := validateMatrixPairs([]matrixPair{p}, testIndex())
			assertFindings(t, fs, []Finding{c.want})
			if (c.name == "dish_id not in menu" || c.name == "rec_id not in menu") && len(covered) != 0 {
				t.Errorf("covered = %v, want empty", covered)
			}
		})
	}
}

func TestFR10_ValidPairsPass(t *testing.T) {
	p := validPair()
	fs, covered := validateMatrixPairs([]matrixPair{p}, testIndex())
	assertFindings(t, fs, nil)
	if !covered[100] {
		t.Errorf("dish 100 must be covered")
	}
}

func TestFR12_ReasonStopWords(t *testing.T) {
	cases := []struct {
		reason string
		word   string
	}{
		{strings.Repeat("а ", 40) + "Вкусно и сытно", "вкусно"},
		{strings.Repeat("а ", 40) + "Попробуйте этот соус", "попробуйте"},
		{"Популярный выбор гостей и просто хороший аккомпанемент к этому блюду на каждый день", "популярный выбор"},
		{"Это блюдо отлично сочетается с выбранным, дополняет его текстуру и вкус полностью", "отлично сочетается"},
	}
	for _, c := range cases {
		t.Run(c.word, func(t *testing.T) {
			p := validPair()
			p.ReasonText = c.reason
			fs, _ := validateMatrixPairs([]matrixPair{p}, testIndex())
			assertFindings(t, fs, []Finding{
				{SeverityError, "matrix dish_id=100 rec_id=300: стоп-слово \"" + c.word + "\""},
			})
		})
	}
}

func TestFR10_CategoryOverrideWarnings(t *testing.T) {
	p := validPair()
	p.CategoryOverride = "Напитки"
	fs, _ := validateMatrixPairs([]matrixPair{p}, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityWarn, `matrix dish_id=100 rec_id=300: category_override "Напитки" ≠ категории rec_id ("Соусы")`},
	})

	p = validPair()
	p.CategoryOverride = "Нет такой"
	fs, _ = validateMatrixPairs([]matrixPair{p}, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityWarn, `matrix dish_id=100 rec_id=300: category_override "Нет такой" не найдена в меню`},
	})

	p = validPair()
	p.CategoryOverride = "Соусы"
	fs, _ = validateMatrixPairs([]matrixPair{p}, testIndex())
	assertFindings(t, fs, nil)
}

func TestFR10_CategoryTargets(t *testing.T) {
	fs := validateCategoryTargets([]matrixPair{validPair()}, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityError, `matrix category="Салаты": ни одна пара не ведёт в категорию`},
		{SeverityError, `matrix category="Супы": ни одна пара не ведёт в категорию`},
		{SeverityError, `matrix category="Напитки": ни одна пара не ведёт в категорию`},
	})

	p1 := validPair()
	p2 := validPair()
	p2.DishID, p2.RecID = 300, 400
	p3 := validPair()
	p3.DishID, p3.RecID = 400, 100
	p4 := validPair()
	p4.DishID, p4.RecID = 400, 200
	fs = validateCategoryTargets([]matrixPair{p1, p2, p3, p4}, testIndex())
	assertFindings(t, fs, nil)
}

func TestFR10_Coverage(t *testing.T) {
	idx := testIndex()
	fs := validateCoverage(map[int64]bool{100: true, 200: true, 300: true}, idx)
	assertFindings(t, fs, []Finding{
		{SeverityError, "matrix: покрытие 75.0% (3/4) < 80%"},
		{SeverityWarn, "matrix dish_id=400: блюдо не покрыто (нет пар)"},
	})

	fs = validateCoverage(map[int64]bool{100: true, 200: true, 300: true, 400: true}, idx)
	assertFindings(t, fs, nil)

	cats := []domain.Category{
		{ID: 1, Name: "Салаты"},
		{ID: 2, Name: "Супы"},
	}
	items := []domain.MenuItem{
		{ID: 100, CategoryID: 1},
		{ID: 200, CategoryID: 1},
		{ID: 300, CategoryID: 2},
		{ID: 400, CategoryID: 2},
		{ID: 500, CategoryID: 2},
	}
	fs = validateCoverage(map[int64]bool{100: true, 200: true, 300: true, 400: true}, buildMenuIndex(cats, items))
	assertFindings(t, fs, []Finding{
		{SeverityWarn, "matrix dish_id=500: блюдо не покрыто (нет пар)"},
	})
}

func TestFR10_HeuristicsChecks(t *testing.T) {
	valid := []heuristicRule{
		{FromCategory: "Салаты", ToCategories: []string{"Соусы", "Напитки"}},
		{FromCategory: "Супы", ToCategories: []string{"Напитки"}},
		{FromCategory: "Соусы", ToCategories: []string{"Салаты"}},
		{FromCategory: "Напитки", ToCategories: []string{"Салаты"}},
	}
	fs := validateHeuristics(valid, testIndex())
	assertFindings(t, fs, nil)

	badTo := append([]heuristicRule{}, valid...)
	badTo[0] = heuristicRule{FromCategory: "Салаты", ToCategories: []string{"Нет"}}
	fs = validateHeuristics(badTo, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityError, `heuristics from="Салаты": to-категория "Нет" не найдена в меню`},
	})

	badFrom := append([]heuristicRule{}, valid...)
	badFrom[0] = heuristicRule{FromCategory: "Нет", ToCategories: []string{"Соусы"}}
	fs = validateHeuristics(badFrom, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityError, `heuristics from="Нет": категория не найдена в меню`},
		{SeverityError, `heuristics from="Салаты": нет from-правила для категории меню`},
	})
}

func TestFR10_UpsellMapChecks(t *testing.T) {
	fs := validateUpsellMap(map[string]int64{"Тигровые креветки (50г) 420₽": 300}, testIndex())
	assertFindings(t, fs, nil)

	fs = validateUpsellMap(map[string]int64{"текст": 999}, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityError, `upsell_map text="текст": item_id 999 не найден в меню`},
	})
}

func TestFR10_ForcartPrioritiesChecks(t *testing.T) {
	fs := validateForcartPriorities([][]string{{"Напитки"}, {"Салаты", "Супы"}}, testIndex())
	assertFindings(t, fs, nil)

	fs = validateForcartPriorities([][]string{{"Напитки"}, {"Десерты"}}, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityError, `forcart_priorities category="Десерты": нет в меню`},
	})

	fs = validateForcartPriorities([][]string{{"Напитки", "Салаты"}, {"Салаты", "Соусы"}}, testIndex())
	assertFindings(t, fs, []Finding{
		{SeverityError, `forcart_priorities category="Салаты": более одной группы`},
	})
}

func TestFR10_ValidateFilesEndToEnd(t *testing.T) {
	report, err := ValidateFiles("testdata", "aidabaran")
	if err != nil {
		t.Fatalf("ValidateFiles: %v", err)
	}
	if report.HasErrors() {
		t.Fatalf("unexpected errors: %+v", report.Findings)
	}
	if report.PairCount != 8 || report.MenuCount != 10 || report.CoveredCount != 8 {
		t.Errorf("report counts = pairs %d, menu %d, covered %d; want 8/10/8", report.PairCount, report.MenuCount, report.CoveredCount)
	}
	if want := 100 * 8.0 / 10.0; report.Coverage < want-0.01 || report.Coverage > want+0.01 {
		t.Errorf("coverage = %f, want %f", report.Coverage, want)
	}
	assertFindings(t, report.Findings, []Finding{
		{SeverityWarn, "matrix dish_id=304: блюдо не покрыто (нет пар)"},
		{SeverityWarn, "matrix dish_id=305: блюдо не покрыто (нет пар)"},
	})
}

func TestFR10_ValidateFilesMatrixFileMissing(t *testing.T) {
	dir := t.TempDir()
	restDir := filepath.Join(dir, "aidabaran")
	recDir := filepath.Join(restDir, "recommender")
	if err := os.MkdirAll(recDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	copyFixture := func(dst, src string) {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o640); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}
	copyFixture(filepath.Join(restDir, "menu.json"), filepath.Join("testdata", "aidabaran", "menu.json"))
	copyFixture(filepath.Join(recDir, "heuristics.json"), filepath.Join("testdata", "aidabaran", "recommender", "heuristics.json"))
	copyFixture(filepath.Join(recDir, "upsell_map.json"), filepath.Join("testdata", "aidabaran", "recommender", "upsell_map.json"))
	copyFixture(filepath.Join(recDir, "forcart_priorities.json"), filepath.Join("testdata", "aidabaran", "recommender", "forcart_priorities.json"))

	report, err := ValidateFiles(dir, "aidabaran")
	if err != nil {
		t.Fatalf("ValidateFiles: %v", err)
	}
	if !report.HasErrors() {
		t.Fatalf("want errors, got findings %+v", report.Findings)
	}
	if report.ErrorCount() != 1 || len(report.Findings) != 1 {
		t.Fatalf("findings = %+v, want single matrix file error", report.Findings)
	}
	f := report.Findings[0]
	if f.Severity != SeverityError || !strings.HasPrefix(f.Message, "matrix: файл ") || !strings.Contains(f.Message, "matrix.json") {
		t.Errorf("finding = %+v, want missing matrix.json file error", f)
	}
}
