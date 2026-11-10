package aidabaran

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"svoyshef/internal/domain"
)

func TestFR12_FallbacksDoNotInventCulinaryProperties(t *testing.T) {
	tests := []struct {
		from      string
		to        string
		dish      string
		candidate string
		anchors   []string
		forbidden []string
	}{
		{"Чебуреки", "Напитки", "Дэда - чебурек с сулугуни", "Сок яблочный", []string{"напиток", "смену текстуры"}, []string{"мяс", "сочн", "хруст", "фрукт"}},
		{"Чебуреки", "Салаты", "Айда чебурек с сулугуни", "Сезонные овощи с ароматной зеленью", []string{"сочность", "контраст", "тест", "начинк"}, []string{"мяс", "сок"}},
		{"Хачапури", "Салаты", "Лахмаджун с говядиной", "Большой овощной салат с сыром фета и джонджоли", []string{"контраст", "тест", "салат"}, []string{"сыр", "сливоч"}},
		{"Холодные закуски", "Основные блюда", "Ассорти из грузинских сыров", "Цыпленок тапака", []string{"закусоч", "основно", "курс"}, []string{"квашен", "солён", "солен", "кисл"}},
		{"Холодные закуски", "Блюда на углях", "Сезонные овощи с ароматной зеленью", "Шашлык из телятины с домашним сацебели", []string{"закус", "основн", "курс"}, []string{"квашен", "солён", "солен", "дым"}},
		{"Овощи на углях и гарниры", "Основные блюда", "Картофельное пюре", "Цыпленок тапака", []string{"гарнир", "основно", "текстур"}, []string{"угл", "дым", "томл", "слад", "овощ", "мяс"}},
		{"Овощи на углях и гарниры", "Блюда на углях", "Картофель фри", "Шашлык из телятины с домашним сацебели", []string{"гарнир", "основно"}, []string{"угл", "дым", "томл"}},
		{"Основные блюда", "Овощи на углях и гарниры", "Дорадо с гранатовым соусом на пару", "Картофель фри", []string{"гарнир", "основ", "текстур"}, []string{"мяс", "угл", "дым", "томл", "овощ"}},
		{"Хачапури", "Соусы", "Лахмаджун с говядиной", "Сацебели", []string{"тест", "начинк", "соус"}, []string{"сыр", "сливоч", "тянущ", "свеж", "нёб"}},
		{"Хинкали", "Соусы", "Хинкали сулугуни 3 шт", "Сацебели", []string{"начинк", "соус"}, []string{"бульон", "горяч", "прохлад", "восстанавл"}},
		{"Хинкали", "Холодные закуски", "Хинкали сулугуни 3 шт", "Пхали ассорти", []string{"закусоч", "курс", "тест"}, []string{"бульон", "квашен", "солён"}},
		{"Супы", "Тандырный хлеб", "Кукурузный суп с беконом и яйцом пашот", "Лаваш Армянский", []string{"хлеб", "жидк", "суп"}, []string{"бульон", "согрева", "жар", "мякиш"}},
		{"Супы", "Хачапури", "Грибная похлебка с трюфельной сметаной", "Лахмаджун с говядиной", []string{"тест", "начинк", "жидк", "суп"}, []string{"бульон", "сыр", "сливоч"}},
		{"Супы", "Чебуреки", "Уха сливочная", "Дэда - чебурек с сулугуни", []string{"тест", "начинк", "суп"}, []string{"мяс", "бульон"}},
		{"Тандырный хлеб", "Супы", "Лаваш Армянский", "Куриная лапша с чесночными крутонами", []string{"жидк", "хлеб", "суп"}, []string{"бульон", "мякиш"}},
		{"Салаты", "Основные блюда", "Жареные баклажаны с миндалем и сыром страчателла", "Цыпленок тапака", []string{"закусоч", "основно", "курс"}, []string{"кисл", "зелён", "тепл", "свеж", "мяс"}},
		{"Соусы", "Тандырный хлеб", "Дзадзики", "Шотис Пури", []string{"соус", "хлеб", "основ"}, []string{"ядрён", "прян", "специ"}},
		{"Соусы", "Хачапури", "Дзадзики", "Лахмаджун с говядиной", []string{"соус", "тест", "начинк"}, []string{"сыр", "сливоч", "специ"}},
		{"Тандырный хлеб", "Соусы", "Лаваш Армянский", "Сацебели", []string{"хлеб", "соус", "основ"}, []string{"дым", "тепл", "лепёшк"}},
		{"Напитки", "Десерты", "Вода Саирме негазированная", "Медовик с цветочным медом", []string{"напитк", "сладкий курс"}, []string{"фрукт", "жидк", "свеж", "нёб", "ярче", "обостр", "холод", "кисл"}},
		{"Напитки", "Холодные закуски", "Вода Саирме негазированная", "Ассорти из грузинских сыров", []string{"закуск", "напитк", "смену текстуры"}, []string{"фрукт", "квашен", "солён", "раскры"}},
		{"Основные блюда", "Десерты", "Дорадо с гранатовым соусом на пару", "Медовик с цветочным медом", []string{"самостоятельный сладкий курс", "после"}, []string{"кусоч", "гарнир", "мяс"}},
		{"Десерты", "Напитки", "Медовик с цветочным медом", "Вода Саирме негазированная", []string{"напиток", "смену текстуры"}, []string{"фрукт", "кисл", "раскры", "освеж", "жидкая часть"}},
	}
	for _, tt := range tests {
		t.Run(tt.dish, func(t *testing.T) {
			template := templateFor(tt.from, tt.to)
			for _, format := range []string{template.full, template.short} {
				text := fmt.Sprintf(format, tt.dish, tt.candidate)
				assertNoStopWords(t, text)
				// Check claims outside menu names (e.g. the candidate can itself be juice).
				claims := strings.ToLower(fmt.Sprintf(format, "", ""))
				for _, word := range tt.forbidden {
					if strings.Contains(claims, word) {
						t.Errorf("unsupported %q in %q", word, text)
					}
				}
				for _, anchor := range tt.anchors {
					if !strings.Contains(claims, anchor) {
						t.Errorf("missing culinary role %q in %q", anchor, text)
					}
				}
			}
			text := pairText(tt.dish, tt.candidate, tt.from, tt.to)
			assertTextBounds(t, text)
			for _, name := range []string{tt.dish, tt.candidate} {
				if !strings.Contains(text, "«"+name+"»") {
					t.Errorf("missing full name %q in %q", name, text)
				}
			}
		})
	}
}

func TestFR11_FallbackNamesAndReasonSurviveLengthLimits(t *testing.T) {
	categories := []string{"Неизвестная категория"}
	for category := range reasonTemplates {
		categories = append(categories, category)
	}
	for _, from := range categories {
		for _, to := range categories {
			template := templateFor(from, to)
			t.Run(from+"/"+to, func(t *testing.T) {
				for _, lengths := range [][2]int{{1, 1}, {71, 67}, {95, 95}, {500, 500}, {500, 1}, {1, 500}} {
					t.Run(fmt.Sprint(lengths), func(t *testing.T) {
						dish, candidate := strings.Repeat("Я", lengths[0]), strings.Repeat("Ю", lengths[1])
						text := pairText(dish, candidate, from, to)
						assertTextBounds(t, text)
						if !utf8.ValidString(text) || strings.Count(text, "«") != 2 || strings.Count(text, "»") != 2 {
							t.Fatalf("invalid Unicode or truncated pair: %q", text)
						}
						if !strings.Contains(text, "Я") || !strings.Contains(text, "Ю") {
							t.Errorf("one name lost: %q", text)
						}
						full, short := fmt.Sprintf(template.full, dish, candidate), fmt.Sprintf(template.short, dish, candidate)
						switch {
						case utf8.RuneCountInString(full) <= reasonMaxLen:
							if text != full {
								t.Errorf("fitting full template changed: %q", text)
							}
						case utf8.RuneCountInString(short) <= reasonMaxLen:
							if text != short {
								t.Errorf("fitting short template changed: %q", text)
							}
						default:
							suffix := template.short[strings.LastIndex(template.short, "»")+len("»"):]
							if !strings.HasSuffix(text, suffix) || !strings.Contains(text, "…") {
								t.Errorf("culinary reason or shortening marker lost: %q", text)
							}
						}
						if text != pairText(dish, candidate, from, to) {
							t.Error("non-deterministic fallback text")
						}
					})
				}
			})
		}
	}
}

func TestFR11_UpsellAndHeuristicUseSafeFallback(t *testing.T) {
	for _, upsell := range []bool{false, true} {
		t.Run(fmt.Sprintf("upsell=%v", upsell), func(t *testing.T) {
			categories := []domain.Category{{ID: 1, Name: "Чебуреки"}, {ID: 2, Name: "Напитки"}}
			dish := domain.MenuItem{ID: 1, CategoryID: 1, Name: "Айда чебурек с сулугуни"}
			candidate := domain.MenuItem{ID: 2, CategoryID: 2, Name: "Сок яблочный"}
			mapping := UpsellMap{}
			wantSource, wantScore := "heuristic", scoreHeuristic
			if upsell {
				dish.UpsellOptions = []domain.Upsell{{Name: candidate.Name}}
				mapping.Map = map[string]int64{candidate.Name: candidate.ID}
				wantSource, wantScore = "upsell", scoreUpsell
			}
			r := NewRecommender(categories, []domain.MenuItem{dish, candidate}, Matrix{},
				Heuristics{Rules: []heuristicRule{{FromCategory: "Чебуреки", ToCategories: []string{"Напитки"}}}},
				mapping, CoursePriorities{CoursePriority: [][]string{{"Напитки"}}}, RecommendationPolicy{})
			recs, err := r.ForDish(context.Background(), dish.ID, nil)
			if err != nil || len(recs) != 1 {
				t.Fatalf("ForDish = %v, %v", recs, err)
			}
			rec := recs[0]
			if rec.ItemID != candidate.ID || rec.Source != wantSource || rec.Score != wantScore {
				t.Fatalf("cascade changed: %+v", rec)
			}
			if rec.ReasonText != pairText(dish.Name, candidate.Name, "Чебуреки", "Напитки") || strings.Contains(rec.ReasonText, "мяс") || !strings.Contains(rec.ReasonText, "напиток") {
				t.Errorf("unsafe or unexpected fallback: %q", rec.ReasonText)
			}
			assertTextBounds(t, rec.ReasonText)
			cartRec, err := r.ForCart(context.Background(), cartLines(dish.ID), nil)
			if err != nil || cartRec == nil || *cartRec != rec {
				t.Fatalf("ForCart differs: %+v, %v; want %+v", cartRec, err, rec)
			}
		})
	}
}

func TestFR12_TextForUsesSourceAndCandidateCategories(t *testing.T) {
	e := &engine{catName: map[int]string{
		1: "Хачапури", 2: "Салаты", 3: "Напитки", 4: "Основные блюда", 5: "Десерты",
	}}
	dish := domain.MenuItem{Name: "Лахмаджун с говядиной", CategoryID: 1}
	candidate := domain.MenuItem{Name: "Сезонные овощи с ароматной зеленью", CategoryID: 2}
	saladText := e.textFor(dish, candidate)
	if !strings.Contains(saladText, "тест") || !strings.Contains(saladText, "салат") {
		t.Fatalf("source/target roles missing: %q", saladText)
	}
	candidate.Name, candidate.CategoryID = "Вода Саирме негазированная", 3
	drinkText := e.textFor(dish, candidate)
	if !strings.Contains(drinkText, "напиток") || strings.Contains(drinkText, "салат") {
		t.Fatalf("candidate category ignored: %q", drinkText)
	}
	candidate.Name, candidate.CategoryID = "Медовик с цветочным медом", 5
	dish.Name, dish.CategoryID = "Дорадо с гранатовым соусом на пару", 4
	dessertAfterFood := e.textFor(dish, candidate)
	if !strings.Contains(dessertAfterFood, "После") || !strings.Contains(dessertAfterFood, "сладкий курс") {
		t.Fatalf("dessert not a following course: %q", dessertAfterFood)
	}
	dish.Name, dish.CategoryID = "Вода Саирме негазированная", 3
	dessertWithDrink := e.textFor(dish, candidate)
	if strings.Contains(dessertWithDrink, "После") || !strings.Contains(dessertWithDrink, "напитк") {
		t.Fatalf("drink treated as a previous food course: %q", dessertWithDrink)
	}
	for _, text := range []string{saladText, drinkText, dessertAfterFood, dessertWithDrink} {
		assertTextBounds(t, text)
	}
}

func TestFR12_FallbacksPreserveRealMenuNames(t *testing.T) {
	menu, err := LoadMenu("../../../data", "aidabaran")
	if err != nil {
		t.Fatalf("LoadMenu: %v", err)
	}
	e := &engine{catName: make(map[int]string)}
	for _, category := range menu.Categories {
		e.catName[category.ID] = category.Name
	}
	for _, dish := range menu.Items {
		for _, candidate := range menu.Items {
			if dish.CategoryID == candidate.CategoryID {
				continue
			}
			text := e.textFor(dish, candidate)
			assertTextBounds(t, text)
			if !strings.Contains(text, "«"+dish.Name+"»") || !strings.Contains(text, "«"+candidate.Name+"»") {
				t.Errorf("menu names truncated for %d -> %d: %q", dish.ID, candidate.ID, text)
			}
		}
	}
}

func TestFR12_KnownFallbackDirectionsHaveRoles(t *testing.T) {
	var heuristics Heuristics
	if err := loadJSONFile("../../../data/aidabaran/recommender/heuristics.json", &heuristics); err != nil {
		t.Fatalf("load heuristics: %v", err)
	}
	roles := map[string]string{
		"Салаты": "салат", "Холодные закуски": "закус", "Супы": "суп",
		"Хачапури": "тест", "Чебуреки": "тест", "Горячие закуски": "закус",
		"Основные блюда": "основно", "Блюда на углях": "основно",
		"Овощи на углях и гарниры": "гарнир", "Соусы": "соус",
		"Десерты": "сладкий курс", "Тандырный хлеб": "хлеб", "Напитки": "напиток",
	}
	for _, rule := range heuristics.Rules {
		for _, to := range rule.ToCategories {
			t.Run(rule.FromCategory+"/"+to, func(t *testing.T) {
				role, ok := roles[to]
				if !ok {
					t.Fatalf("new target %q requires culinary review", to)
				}
				template := templateFor(rule.FromCategory, to)
				for _, format := range []string{template.full, template.short} {
					claims := fmt.Sprintf(format, "", "")
					if !strings.Contains(claims, role) {
						t.Errorf("missing target role %q in %q", role, claims)
					}
					for _, vague := range []string{"сопостав", "сравни", "раскры", "жидкая часть", "категория не распознана"} {
						if strings.Contains(claims, vague) {
							t.Errorf("unsupported or empty explanation %q: %q", vague, claims)
						}
					}
				}
				assertTextBounds(t, pairText("Блюдо", "Дополнение", rule.FromCategory, to))
			})
		}
	}
}
