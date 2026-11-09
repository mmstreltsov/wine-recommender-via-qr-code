package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	goose "github.com/pressly/goose/v3"

	"svoyshef/internal/api"
	"svoyshef/internal/app"
	"svoyshef/internal/recommender"
	"svoyshef/internal/store"
	"svoyshef/migrations"
	"svoyshef/plugins/aidabaran"
)

type config struct {
	Port            string
	DatabaseURL     string
	Restaurant      string
	DataDir         string
	FrontendDist    string
	CookieMaxAgeH   int
	AvailabilityTTL time.Duration
}

func loadConfig() (config, error) {
	cfg := config{
		Port:          envOr("PORT", "8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		Restaurant:    envOr("RESTAURANT", "aidabaran"),
		DataDir:       envOr("DATA_DIR", "../data"),
		FrontendDist:  envOr("FRONTEND_DIST", "../frontend/dist"),
		CookieMaxAgeH: 12,
	}
	if cfg.Restaurant != "aidabaran" {
		return cfg, fmt.Errorf("RESTAURANT=%q не поддерживается: в v0 доступен только плагин \"aidabaran\"", cfg.Restaurant)
	}
	if v := os.Getenv("COOKIE_MAX_AGE_H"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > int((1<<63-1)/int64(time.Hour)) {
			return cfg, fmt.Errorf("invalid COOKIE_MAX_AGE_H: %q", v)
		}
		cfg.CookieMaxAgeH = n
	}
	ttl := 5
	if v := os.Getenv("AVAILABILITY_TTL_S"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("invalid AVAILABILITY_TTL_S: %q", v)
		}
		ttl = n
	}
	cfg.AvailabilityTTL = time.Duration(ttl) * time.Second
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func runMigrations(ctx context.Context, databaseURL string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("operation=migrate_database code=internal")
	}
	defer func() { _ = sqlDB.Close() }()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		return errors.New("operation=migrate_database code=internal")
	}
	return nil
}

func newPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, errors.New("operation=connect_database code=internal")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("operation=connect_database code=internal")
	}
	return pool, nil
}

func buildHandler(cfg config, pool *pgxpool.Pool, loadResult *aidabaran.LoadResult, upsellItem map[string]int64, rec recommender.Recommender) (http.Handler, *app.AvailabilityCache) {
	availabilityCache := app.NewAvailabilityCache(store.NewAvailabilityStore(pool), cfg.AvailabilityTTL)
	menuService := app.NewMenuService(app.NewStaticMenu(loadResult.Categories, loadResult.Items), availabilityCache, upsellItem)
	eventsService := app.NewEventsService(store.NewEventStore(pool))
	cartService := app.NewCartService(menuService, store.NewCartStore(pool), eventsService, rec, availabilityCache)
	recsService := app.NewRecommendations(menuService, rec, store.NewCartStore(pool), availabilityCache)
	ui := aidabaran.NewUIConfig()
	handler := api.NewRouter(api.RouterDeps{
		Restaurant:    cfg.Restaurant,
		Config:        api.RestaurantConfig{Name: ui.Name, LogoURL: ui.LogoURL},
		RecBlockTitle: ui.RecBlockTitle,
		Menu:          menuService,
		Cart:          cartService,
		Recs:          recsService,
		Events:        eventsService,
		Sessions:      store.NewSessionStore(pool),
		CookieMaxAge:  time.Duration(cfg.CookieMaxAgeH) * time.Hour,
		DataDir:       cfg.DataDir,
		FrontendDist:  cfg.FrontendDist,
	})
	return handler, availabilityCache
}

func runServe() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := newPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	if err := runMigrations(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	loadResult, err := aidabaran.LoadMenu(cfg.DataDir, cfg.Restaurant)
	if err != nil {
		return fmt.Errorf("load menu: %w", err)
	}
	upsellItem, err := aidabaran.LoadUpsellMap(cfg.DataDir, cfg.Restaurant)
	if err != nil {
		return fmt.Errorf("load upsell map: %w", err)
	}
	rec, err := aidabaran.LoadRecommender(cfg.DataDir, cfg.Restaurant)
	if err != nil {
		return fmt.Errorf("load recommender: %w", err)
	}
	handler, _ := buildHandler(cfg, pool, loadResult, upsellItem, rec)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("svoyshef server listening on :%s (restaurant=%s, menu items=%d)", cfg.Port, cfg.Restaurant, len(loadResult.Items))
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}

func runAvailability(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: availability set <item_id> on|off | availability list | availability find <подстрока>")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	switch args[0] {
	case "set":
		if len(args) != 3 || (args[2] != "on" && args[2] != "off") {
			return errors.New("usage: availability set <item_id> on|off")
		}
		itemID, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("некорректный item_id: %q", args[1])
		}
		return availabilitySet(cfg, itemID, args[2] == "on")
	case "list":
		if len(args) != 1 {
			return errors.New("usage: availability list")
		}
		return availabilityList(cfg)
	case "find":
		if len(args) != 2 {
			return errors.New("usage: availability find <подстрока>")
		}
		return availabilityFind(cfg, args[1])
	default:
		return fmt.Errorf("неизвестная подкоманда availability: %q", args[0])
	}
}

func loadMenuOrFail(cfg config) *aidabaran.LoadResult {
	loadResult, err := aidabaran.LoadMenu(cfg.DataDir, cfg.Restaurant)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка загрузки меню: %v\n", err)
		os.Exit(1)
	}
	return loadResult
}

func availabilitySet(cfg config, itemID int64, available bool) error {
	loadResult := loadMenuOrFail(cfg)
	found := false
	var name string
	for _, it := range loadResult.Items {
		if it.ID == itemID {
			found = true
			name = it.Name
			break
		}
	}
	if !found {
		return fmt.Errorf("позиция не найдена в меню: %d", itemID)
	}
	ctx := context.Background()
	pool, err := newPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	if err := runMigrations(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := store.NewAvailabilityStore(pool).Upsert(ctx, itemID, available); err != nil {
		return errors.New("operation=set_availability code=internal")
	}
	state := "off"
	if available {
		state = "on"
	}
	fmt.Printf("%d available=%s (%s)\n", itemID, state, name)
	return nil
}

func availabilityList(cfg config) error {
	ctx := context.Background()
	pool, err := newPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	if err := runMigrations(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	records, err := store.NewAvailabilityStore(pool).Records(ctx)
	if err != nil {
		return errors.New("operation=list_availability code=internal")
	}
	printed := false
	for _, r := range records {
		if !r.IsAvailable {
			fmt.Printf("%d available=off %s\n", r.ItemID, r.UpdatedAt.UTC().Format(time.RFC3339))
			printed = true
		}
	}
	if !printed {
		fmt.Println("нет отключений")
	}
	return nil
}

func availabilityFind(cfg config, substr string) error {
	loadResult := loadMenuOrFail(cfg)
	needle := strings.ToLower(substr)
	ctx := context.Background()
	pool, err := newPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	if err := runMigrations(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	records, err := store.NewAvailabilityStore(pool).Records(ctx)
	if err != nil {
		return errors.New("operation=find_availability code=internal")
	}
	disabled := make(map[int64]bool)
	for _, r := range records {
		disabled[r.ItemID] = !r.IsAvailable
	}
	catNames := make(map[int]string)
	for _, c := range loadResult.Categories {
		catNames[c.ID] = c.Name
	}
	found := false
	for _, it := range loadResult.Items {
		if !strings.Contains(strings.ToLower(it.Name), needle) {
			continue
		}
		found = true
		state := "on"
		if disabled[it.ID] {
			state = "off"
		}
		fmt.Printf("%d | %s | %s | %s\n", it.ID, it.Name, catNames[it.CategoryID], state)
	}
	if !found {
		fmt.Println("ничего не найдено")
	}
	return nil
}

// parseExportDate парсит значение переданного флага строго как YYYY-MM-DD в UTC.
func parseExportDate(name, value string) (*time.Time, error) {
	t, err := time.Parse("2006-01-02", value)
	if err != nil || t.Format("2006-01-02") != value {
		return nil, fmt.Errorf("флаг --%s: ожидалась дата в формате YYYY-MM-DD, получено %q", name, value)
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return &day, nil
}

// parseExportRange: --from → ts >= fromT00:00:00Z; --to → ts < (to+1)T00:00:00Z
// (день --to входит включительно, полуинтервал — events.md §4).
func parseExportRange(args []string) (from, toExcl *time.Time, err error) {
	fs := flag.NewFlagSet("export-events", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fromRaw := fs.String("from", "", "нижняя граница дат YYYY-MM-DD (включительно)")
	toRaw := fs.String("to", "", "верхняя граница дат YYYY-MM-DD (день входит включительно)")
	if err := fs.Parse(args); err != nil {
		return nil, nil, fmt.Errorf("некорректные флаги export-events: %w", err)
	}
	if fs.NArg() > 0 {
		return nil, nil, fmt.Errorf("неизвестный аргумент export-events: %q", fs.Arg(0))
	}
	provided := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if provided["from"] {
		from, err = parseExportDate("from", *fromRaw)
		if err != nil {
			return nil, nil, err
		}
	}
	var to *time.Time
	if provided["to"] {
		to, err = parseExportDate("to", *toRaw)
	}
	if err != nil {
		return nil, nil, err
	}
	if from != nil && to != nil && from.After(*to) {
		return nil, nil, errors.New("export-events: --from не может быть позже --to")
	}
	if to != nil {
		excl := to.AddDate(0, 0, 1)
		toExcl = &excl
	}
	return from, toExcl, nil
}

// writeEventsCSV: колонки ts,session_id,table_id,name,payload; заголовок обязателен;
// ts — RFC 3339 UTC с микросекундами; table_id NULL → пустое поле; payload — текст
// jsonb как в БД, всегда в кавычках по RFC 4180; LF, UTF-8, без BOM.
func writeEventsCSV(w io.Writer, rows []store.ExportedEvent) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"ts", "session_id", "table_id", "name", "payload"}); err != nil {
		return err
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	var prefix bytes.Buffer
	fields := csv.NewWriter(&prefix)
	for _, e := range rows {
		prefix.Reset()
		tableID := ""
		if e.TableID != nil {
			tableID = strconv.Itoa(*e.TableID)
		}
		rec := []string{
			e.TS.UTC().Format("2006-01-02T15:04:05.000000Z"),
			e.SessionID,
			tableID,
			e.Name,
		}
		if err := fields.Write(rec); err != nil {
			return err
		}
		fields.Flush()
		if err := fields.Error(); err != nil {
			return err
		}
		line := strings.TrimSuffix(prefix.String(), "\n") + ",\"" + strings.ReplaceAll(e.Payload, "\"", "\"\"") + "\"\n"
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	return nil
}

// runExportEvents (AN-04): CSV целиком собирается в буфер и попадает в stdout
// только при успехе — при ошибке флагов/БД stdout пуст (events.md §4).
func runExportEvents(args []string, stdout io.Writer) error {
	from, toExcl, err := parseExportRange(args)
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := newPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("export-events: не удалось подключиться к БД")
	}
	defer pool.Close()
	if err := runMigrations(ctx, cfg.DatabaseURL); err != nil {
		return errors.New("export-events: не удалось подготовить БД")
	}
	rows, err := store.NewEventStore(pool).ExportRows(ctx, from, toExcl)
	if err != nil {
		return errors.New("export-events: не удалось прочитать события из БД")
	}
	var buf bytes.Buffer
	if err := writeEventsCSV(&buf, rows); err != nil {
		return err
	}
	_, err = stdout.Write(buf.Bytes())
	return err
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "serve" {
		if err := runServe(); err != nil {
			log.Fatalf("serve: %v", err)
		}
		return
	}
	var err error
	switch args[0] {
	case "availability":
		err = runAvailability(args[1:])
	case "export-events":
		err = runExportEvents(args[1:], os.Stdout)
	default:
		err = fmt.Errorf("неизвестная подкоманда: %q", args[0])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %v\n", err)
		os.Exit(1)
	}
}
