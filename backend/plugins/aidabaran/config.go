package aidabaran

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	RestaurantName = "АйДаБаран"
	LogoURL        = "/static/aidabaran/logo.png"
	RecBlockTitle  = "Хорошо сочетается с"
)

type UIConfig struct {
	Name          string
	LogoURL       string
	RecBlockTitle string
}

func NewUIConfig() UIConfig {
	return UIConfig{Name: RestaurantName, LogoURL: LogoURL, RecBlockTitle: RecBlockTitle}
}

type rawUpsellMap struct {
	Map map[string]int64 `json:"map"`
}

func LoadUpsellMap(dataDir, restaurant string) (map[string]int64, error) {
	path := filepath.Join(dataDir, restaurant, "recommender", "upsell_map.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read upsell map: %w", err)
	}
	var raw rawUpsellMap
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse upsell map: %w", err)
	}
	keys := make([]string, 0, len(raw.Map))
	for k := range raw.Map {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make(map[string]int64, len(raw.Map))
	for _, text := range keys {
		key := strings.TrimSpace(upsellPriceRe.ReplaceAllString(text, ""))
		if key == "" {
			continue
		}
		if _, dup := result[key]; !dup {
			result[key] = raw.Map[text]
		}
	}
	return result, nil
}
