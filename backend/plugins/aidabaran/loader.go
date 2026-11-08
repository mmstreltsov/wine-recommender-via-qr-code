package aidabaran

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"svoyshef/internal/domain"
)

var parts = []string{
	"Салаты",
	"Холодные закуски",
	"Супы",
	"Хачапури",
	"Хинкали",
	"Горячие закуски",
	"Чебуреки",
	"Основные блюда",
	"Блюда на углях",
	"Овощи на углях и гарниры",
	"Соусы",
	"Десерты",
	"Тандырный хлеб",
	"Напитки",
}

const (
	drinksCategory = "Напитки"
	kbjuPrefix     = "КБЖУ на порцию"
)

var categoryIDs = func() map[string]int {
	m := make(map[string]int, len(parts))
	for i, name := range parts {
		m[name] = i + 1
	}
	return m
}()

var whitespaceRe = regexp.MustCompile(`[\s\p{Zs}]+`)

var upsellPriceRe = regexp.MustCompile(`\s*[\d\s.,]+₽?\s*$`)

type SkippedItem struct {
	UID    int64
	Title  string
	Reason string
}

type LoadResult struct {
	Categories []domain.Category
	Items      []domain.MenuItem
	Skipped    []SkippedItem
}

type rawMenu struct {
	Items []rawItem `json:"items"`
}

type rawItem struct {
	UID           int64        `json:"uid"`
	Title         string       `json:"title"`
	Category      string       `json:"category"`
	Price         *int         `json:"price"`
	Description   *string      `json:"description"`
	KBJU          *string      `json:"kbju"`
	Photos        []string     `json:"photos"`
	UpsellOptions []rawUpsell  `json:"upsell_options"`
	Editions      []rawEdition `json:"editions"`
	LocalPhoto    *string      `json:"local_photo"`
}

type rawUpsell struct {
	Name     string `json:"name"`
	AddPrice int    `json:"add_price"`
}

type rawEdition struct {
	Img string `json:"img"`
}

func LoadMenu(dataDir, restaurant string) (*LoadResult, error) {
	root := filepath.Join(dataDir, restaurant)
	data, err := os.ReadFile(filepath.Join(root, "menu.json"))
	if err != nil {
		return nil, fmt.Errorf("read menu: %w", err)
	}
	var raw rawMenu
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse menu: %w", err)
	}
	return mapMenu(raw.Items, restaurant, root)
}

func mapMenu(items []rawItem, restaurant, root string) (*LoadResult, error) {
	res := &LoadResult{}
	sortOrder := make(map[int]int)
	present := make(map[int]bool)
	for i := range items {
		it := items[i]
		title := strings.TrimSpace(it.Title)
		if title == "" {
			res.skip(it.UID, title, "title_empty")
			continue
		}
		if it.Price == nil {
			res.skip(it.UID, title, "price_null")
			continue
		}
		catID, ok := categoryIDs[it.Category]
		if !ok {
			res.skip(it.UID, title, "category_unknown")
			continue
		}
		present[catID] = true
		item := domain.MenuItem{
			ID:          it.UID,
			CategoryID:  catID,
			Name:        title,
			Price:       *it.Price,
			Description: mapDescription(it, catID),
			KBJU:        mapKBJU(it, catID),
			PhotoURL:    mapPhotoURL(it),
			SortOrder:   sortOrder[catID],
		}
		sortOrder[catID]++
		if it.LocalPhoto != nil && strings.HasPrefix(*it.LocalPhoto, "photos/") {
			rel := filepath.FromSlash(*it.LocalPhoto)
			if fileExists(filepath.Join(root, rel)) {
				item.PhotoFallback = "/static/" + restaurant + "/photos/" + strings.TrimPrefix(*it.LocalPhoto, "photos/")
			}
		}
		for _, u := range it.UpsellOptions {
			item.UpsellOptions = append(item.UpsellOptions, domain.Upsell{
				Name:     strings.TrimSpace(upsellPriceRe.ReplaceAllString(u.Name, "")),
				AddPrice: u.AddPrice,
			})
		}
		res.Items = append(res.Items, item)
	}
	for i, name := range parts {
		if present[i+1] {
			res.Categories = append(res.Categories, domain.Category{ID: i + 1, Name: name})
		}
	}
	return res, nil
}

func mapDescription(it rawItem, catID int) string {
	desc := cleanText(deref(it.Description))
	if it.KBJU == nil {
		return desc
	}
	kbju := cleanText(*it.KBJU)
	if hasCaloriesKeyword(kbju) || catID != categoryIDs[drinksCategory] {
		return desc
	}
	if desc == "" {
		return kbju
	}
	return desc + " " + kbju
}

func mapKBJU(it rawItem, catID int) string {
	if it.KBJU == nil {
		return ""
	}
	kbju := cleanText(*it.KBJU)
	if !hasCaloriesKeyword(kbju) {
		if catID != categoryIDs[drinksCategory] {
			log.Printf("aidabaran: uid=%d kbju без «Калорийности» в категории %q, KBJU отброшен", it.UID, it.Category)
		}
		return ""
	}
	kbju = strings.TrimPrefix(kbju, kbjuPrefix)
	return strings.TrimSpace(kbju)
}

func mapPhotoURL(it rawItem) string {
	if len(it.Photos) > 0 && it.Photos[0] != "" {
		return it.Photos[0]
	}
	if len(it.Editions) > 0 {
		return it.Editions[0].Img
	}
	return ""
}

func hasCaloriesKeyword(s string) bool {
	return strings.Contains(strings.ToLower(s), "калорийность")
}

func cleanText(s string) string {
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(html.UnescapeString(s), " "))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func (r *LoadResult) skip(uid int64, title, reason string) {
	r.Skipped = append(r.Skipped, SkippedItem{UID: uid, Title: title, Reason: reason})
	log.Printf("aidabaran: позиция пропущена uid=%d title=%q reason=%s", uid, title, reason)
}
