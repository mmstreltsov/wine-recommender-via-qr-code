# 30.1. Контракт: модель данных

## 1. Доменные типы ядра (`internal/domain`)

Ядро оперирует только этими типами. Плагин обязан привести сырые данные ресторана к ним.

```go
type Category struct {
    ID    int    // 1..N, порядок = порядок на сайте ресторана
    Name  string // «Салаты», …
}
type MenuItem struct {
    ID           int64     // стабильный ID позиции (у АйДаБарана = uid из Tilda)
    CategoryID   int
    Name         string
    Price        int       // рубли, целое
    Description  string    // «600 гр.» и т.п.
    KBJU         string    // «Калорийность: 569,1 ккал …» или ""
    PhotoURL     string    // CDN-URL оригинала (ресайз добавляет фронт)
    PhotoFallback string   // локальный путь/URL копии, "" если нет
    UpsellOptions []Upsell // апсейл-опции ресторана
    SortOrder    int       // позиция внутри категории (порядок сайта)
}
type Upsell struct { Name string; AddPrice int }

type CartLine struct { ItemID int64; Qty int; AddedFrom string }
type Cart  struct { SessionID string; Lines []CartLine; UpdatedAt time.Time }

type Event struct {
    SessionID string; TableID *int
    Name      string           // таксономия из 30.4
    Payload   map[string]any
    Ts        time.Time
}
```

## 2. JSON Schema ответов API (то, что видит фронт)

`MenuItemSummary` — краткая схема позиции (используется в `GET /menu`, `recs[].item`,
`cart.lines[].item`, `cart.recommendation.item`):
```json
{
  "id": 876201943922,
  "category_id": 1,
  "name": "Большой овощной салат с сыром фета и джонджоли",
  "price": 1390,
  "photo_url": "https://static.tildacdn.com/stor…/file.jpg",
  "photo_fallback": "/static/aidabaran/photos/dishes/salati__….jpg"
}
```

Полный `MenuItem` (только в `GET /menu/items/{id}`) = `MenuItemSummary` плюс:
```json
{
  "description": "600 гр.",
  "kbju": "Калорийность: 569,1 ккал Белки: 11,2 г Жиры: 48,3 г Углеводы: 22,4 г",
  "upsell_options": [{"name": "Тигровые креветки (50г)", "add_price": 420}]
}
```
Правила:
- `kbju` = `""`, если КБЖУ нет или не распознан при загрузке (правила распознавания — §3;
  фронт скрывает блок «Состав» при пустых `kbju` и `description`, FR-07).
- `photo_url` — оригинальный CDN-URL (без `/-/resize/`); ресайз делает фронт по гайду 40-frontend.
- `photo_fallback` — путь, который бэкенд раздаёт статикой из `data/<r>/photos/dishes/…`
  (`""`, если локальной копии нет — фронт тогда просто оставляет CDN-URL).
- Целостность: `price >= 0`; пустые `name` не допускаются (loader отвергает позицию и логирует).

`Recommendation` в ответах API (`item` — `MenuItemSummary`):
```json
{
  "item": { /* MenuItemSummary */ },
  "reason_text": "К копчёной скумбрии просится свежая зелень и танин красного — …",
  "source": "expert"   // expert | upsell | heuristic
}
```

## 3. Маппинг: `data/aidabaran/menu.json` → домен (знает ТОЛЬКО плагин)

| Сырое поле (Tilda) | Домен | Правило |
|---|---|---|
| `items[].uid` | `ID` | как есть (int64, стабильно) |
| `items[].category` | `CategoryID` | по словарю названий → порядок из `parser.py PARTS` (1..14) |
| `items[].title` | `Name` | как есть |
| `items[].price` | `Price` | уже int (parser приводит); NULL → позиция пропускается + лог |
| `items[].description` | `Description` | html-unescape (`&nbsp;` и пр.) + схлопывание пробелов; вес порции — как есть |
| `items[].kbju` | `KBJU` | html-unescape + схлопывание пробелов; КБЖУ — только если содержит подстроку «Калорийность» (регистронезависимо), тогда отрезается префикс «КБЖУ на порцию » (фронт не парсит). Иначе: категория «Напитки» — `Description = {исходное description} + " " + {kbju-текст}` (при пустом description — только kbju-текст), KBJU=`""`; прочие категории — warning в лог, KBJU=`""`. `null` → `""` |
| `items[].photos[0]` else `items[].editions[0].img` | `PhotoURL` | первый непустой; нет обоих → `""` (позиция без фото допустима, фронт показывает заглушку-плашку) |
| `items[].local_photo` | `PhotoFallback` | заменить префикс `photos/` на `/static/aidabaran/photos/`; existence-check файла относительно `DATA_DIR/<restaurant>/`; файла нет → `""`. Слаг в Go не вычисляется (транслит parser.py в Go не воспроизводится) |
| `items[].upsell_options[]` | `UpsellOptions` | имя опции: отрезать хвостовую цену (regexp `\s*[\d\s.,]+₽?\s*$`); display-формат `"{name} +{add_price} ₽"`; `add_price` int |
| порядок в массиве `items` | `SortOrder` | индекс внутри категории |

`barmenu.json` в v0 плагином НЕ загружается (резерв под винный модуль).

## 4. Матрица пар: `data/<restaurant>/recommender/matrix.json`

Схема и семантика — в `30.3 recommender.md` §2. Модель ядра матрицы не знает: это данные плагина.

## 5. События и корзина

Схемы таблиц — `20-architecture.md` §4; таксономия и payload'ы — `30.4 events.md`.
Ограничения валидации (граница API): `item_id` ∈ меню; `qty` ∈ [1,20]; `table` ∈ [1,999];
`name` события ∈ таксономии; payload — строго плоский JSON (только скалярные значения верхнего
уровня, глубина 1), ≤ 1 КБ.
