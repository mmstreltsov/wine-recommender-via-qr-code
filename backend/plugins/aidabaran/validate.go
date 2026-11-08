package aidabaran

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"svoyshef/internal/domain"
)

const (
	SeverityError = "ERROR"
	SeverityWarn  = "WARN"

	minCoveragePercent = 80.0
	reasonMinLen       = 60
	reasonMaxLen       = 220
)

var stopWords = []string{"вкусно", "попробуйте", "популярный выбор", "отлично сочетается"}

type Finding struct {
	Severity string
	Message  string
}

type ValidationReport struct {
	Findings     []Finding
	PairCount    int
	MenuCount    int
	CoveredCount int
	Coverage     float64
}

func (r *ValidationReport) HasErrors() bool {
	return r.ErrorCount() > 0
}

func (r *ValidationReport) ErrorCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			n++
		}
	}
	return n
}

type menuIndex struct {
	catIDByName map[string]int
	catNameByID map[int]string
	catNames    []string
	itemByID    map[int64]domain.MenuItem
	dishIDs     []int64
}

func buildMenuIndex(cats []domain.Category, items []domain.MenuItem) *menuIndex {
	idx := &menuIndex{
		catIDByName: make(map[string]int),
		catNameByID: make(map[int]string),
		itemByID:    make(map[int64]domain.MenuItem),
	}
	for _, c := range cats {
		idx.catIDByName[c.Name] = c.ID
		idx.catNameByID[c.ID] = c.Name
		idx.catNames = append(idx.catNames, c.Name)
	}
	for _, it := range items {
		idx.itemByID[it.ID] = it
		idx.dishIDs = append(idx.dishIDs, it.ID)
	}
	return idx
}

type matrixFile struct {
	Pairs []matrixPair `json:"pairs"`
}

type matrixPair struct {
	DishID           int64   `json:"dish_id"`
	RecID            int64   `json:"rec_id"`
	CategoryOverride string  `json:"category_override"`
	Score            float64 `json:"score"`
	Source           string  `json:"source"`
	ReasonText       string  `json:"reason_text"`
}

type heuristicsFile struct {
	Rules []heuristicRule `json:"rules"`
}

type heuristicRule struct {
	FromCategory  string   `json:"from_category"`
	ToCategories  []string `json:"to_categories"`
}

type upsellMapFile struct {
	Map map[string]int64 `json:"map"`
}

type forcartFile struct {
	CoursePriority [][]string `json:"course_priority"`
}

func ValidateFiles(dataDir, restaurant string) (*ValidationReport, error) {
	menu, err := LoadMenu(dataDir, restaurant)
	if err != nil {
		return nil, fmt.Errorf("load menu: %w", err)
	}
	idx := buildMenuIndex(menu.Categories, menu.Items)
	recDir := filepath.Join(dataDir, restaurant, "recommender")
	report := &ValidationReport{MenuCount: len(menu.Items)}

	var mf matrixFile
	if err := loadJSONFile(filepath.Join(recDir, "matrix.json"), &mf); err != nil {
		report.Findings = append(report.Findings, fileFinding("matrix", recDir, "matrix.json", err))
	} else {
		report.PairCount = len(mf.Pairs)
		pairFindings, covered := validateMatrixPairs(mf.Pairs, idx)
		report.Findings = append(report.Findings, pairFindings...)
		report.Findings = append(report.Findings, validateCategoryTargets(mf.Pairs, idx)...)
		report.CoveredCount = len(covered)
		if report.MenuCount > 0 {
			report.Coverage = 100 * float64(report.CoveredCount) / float64(report.MenuCount)
		}
		report.Findings = append(report.Findings, validateCoverage(covered, idx)...)
	}

	var hf heuristicsFile
	if err := loadJSONFile(filepath.Join(recDir, "heuristics.json"), &hf); err != nil {
		report.Findings = append(report.Findings, fileFinding("heuristics", recDir, "heuristics.json", err))
	} else {
		report.Findings = append(report.Findings, validateHeuristics(hf.Rules, idx)...)
	}

	var uf upsellMapFile
	if err := loadJSONFile(filepath.Join(recDir, "upsell_map.json"), &uf); err != nil {
		report.Findings = append(report.Findings, fileFinding("upsell_map", recDir, "upsell_map.json", err))
	} else {
		report.Findings = append(report.Findings, validateUpsellMap(uf.Map, idx)...)
	}

	var ff forcartFile
	if err := loadJSONFile(filepath.Join(recDir, "forcart_priorities.json"), &ff); err != nil {
		report.Findings = append(report.Findings, fileFinding("forcart_priorities", recDir, "forcart_priorities.json", err))
	} else {
		report.Findings = append(report.Findings, validateForcartPriorities(ff.CoursePriority, idx)...)
	}
	return report, nil
}

func validateMatrixPairs(pairs []matrixPair, idx *menuIndex) ([]Finding, map[int64]bool) {
	var fs []Finding
	covered := make(map[int64]bool)
	for _, p := range pairs {
		prefix := fmt.Sprintf("matrix dish_id=%d rec_id=%d", p.DishID, p.RecID)
		_, dishOK := idx.itemByID[p.DishID]
		rec, recOK := idx.itemByID[p.RecID]
		if !dishOK {
			fs = append(fs, errorf("%s: dish_id не найден в меню", prefix))
		}
		if !recOK {
			fs = append(fs, errorf("%s: rec_id не найден в меню", prefix))
		}
		if p.DishID == p.RecID {
			fs = append(fs, errorf("%s: dish_id = rec_id", prefix))
		}
		if p.Score < 0 || p.Score > 1 {
			fs = append(fs, errorf("%s: score %g вне [0,1]", prefix, p.Score))
		}
		if n := utf8.RuneCountInString(p.ReasonText); n < reasonMinLen || n > reasonMaxLen {
			fs = append(fs, errorf("%s: reason_text %d симв., нужно %d–%d", prefix, n, reasonMinLen, reasonMaxLen))
		}
		lower := strings.ToLower(p.ReasonText)
		for _, w := range stopWords {
			if strings.Contains(lower, w) {
				fs = append(fs, errorf("%s: стоп-слово %q", prefix, w))
				break
			}
		}
		if p.CategoryOverride != "" {
			if overrideID, ok := idx.catIDByName[p.CategoryOverride]; ok {
				if recOK && overrideID != rec.CategoryID {
					fs = append(fs, warnf(`%s: category_override %q ≠ категории rec_id (%q)`, prefix, p.CategoryOverride, idx.catNameByID[rec.CategoryID]))
				}
			} else {
				fs = append(fs, warnf(`%s: category_override %q не найдена в меню`, prefix, p.CategoryOverride))
			}
		}
		if dishOK && recOK {
			covered[p.DishID] = true
		}
	}
	return fs, covered
}

func validateCategoryTargets(pairs []matrixPair, idx *menuIndex) []Finding {
	targets := make(map[int]bool)
	for _, p := range pairs {
		if p.CategoryOverride != "" {
			if id, ok := idx.catIDByName[p.CategoryOverride]; ok {
				targets[id] = true
				continue
			}
		}
		if rec, ok := idx.itemByID[p.RecID]; ok {
			targets[rec.CategoryID] = true
		}
	}
	var fs []Finding
	for _, name := range idx.catNames {
		if !targets[idx.catIDByName[name]] {
			fs = append(fs, errorf("matrix category=%q: ни одна пара не ведёт в категорию", name))
		}
	}
	return fs
}

func validateCoverage(covered map[int64]bool, idx *menuIndex) []Finding {
	total := len(idx.dishIDs)
	if total == 0 {
		return []Finding{errorf("matrix: меню пустое, покрытие не вычисляется")}
	}
	var fs []Finding
	pct := 100 * float64(len(covered)) / float64(total)
	if pct < minCoveragePercent {
		fs = append(fs, errorf("matrix: покрытие %.1f%% (%d/%d) < %.0f%%", pct, len(covered), total, minCoveragePercent))
	}
	for _, id := range idx.dishIDs {
		if !covered[id] {
			fs = append(fs, warnf("matrix dish_id=%d: блюдо не покрыто (нет пар)", id))
		}
	}
	return fs
}

func validateHeuristics(rules []heuristicRule, idx *menuIndex) []Finding {
	var fs []Finding
	hasFrom := make(map[string]bool)
	for _, r := range rules {
		if _, ok := idx.catIDByName[r.FromCategory]; !ok {
			fs = append(fs, errorf("heuristics from=%q: категория не найдена в меню", r.FromCategory))
		}
		hasFrom[r.FromCategory] = true
		for _, to := range r.ToCategories {
			if _, ok := idx.catIDByName[to]; !ok {
				fs = append(fs, errorf("heuristics from=%q: to-категория %q не найдена в меню", r.FromCategory, to))
			}
		}
	}
	for _, name := range idx.catNames {
		if !hasFrom[name] {
			fs = append(fs, errorf("heuristics from=%q: нет from-правила для категории меню", name))
		}
	}
	return fs
}

func validateUpsellMap(m map[string]int64, idx *menuIndex) []Finding {
	var fs []Finding
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, text := range keys {
		if _, ok := idx.itemByID[m[text]]; !ok {
			fs = append(fs, errorf("upsell_map text=%q: item_id %d не найден в меню", text, m[text]))
		}
	}
	return fs
}

func validateForcartPriorities(groups [][]string, idx *menuIndex) []Finding {
	var fs []Finding
	seen := make(map[string]bool)
	for _, group := range groups {
		for _, name := range group {
			if _, ok := idx.catIDByName[name]; !ok {
				fs = append(fs, errorf("forcart_priorities category=%q: нет в меню", name))
				continue
			}
			if seen[name] {
				fs = append(fs, errorf("forcart_priorities category=%q: более одной группы", name))
			}
			seen[name] = true
		}
	}
	return fs
}

func loadJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func fileFinding(rule, dir, name string, err error) Finding {
	return errorf("%s: файл %s: %v", rule, filepath.Join(dir, name), err)
}

func errorf(format string, args ...any) Finding {
	return Finding{Severity: SeverityError, Message: fmt.Sprintf(format, args...)}
}

func warnf(format string, args ...any) Finding {
	return Finding{Severity: SeverityWarn, Message: fmt.Sprintf(format, args...)}
}
