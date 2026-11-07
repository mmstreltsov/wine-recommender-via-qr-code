#!/usr/bin/env python3
"""Парсер меню ресторана АйДаБаран (aidabaran.ru).

Кухня: каталог Tilda Catalog — товары отдаются публичным API
store.tildaapi.com/api/getproductslist/?storepartuid=<uid> (14 категорий).
Барная карта: 12 страниц-фото на главной, распознаётся vision-OCR (не автоматизируется этим скриптом) — см. barmenu.json.

Использование:
  python3 parser.py menu      # пересобрать menu.json
  python3 parser.py photos    # скачать все фото блюд в photos/dishes/
"""
import json
import os
import re
import ssl
import sys
import time
import urllib.request

BASE = os.path.dirname(os.path.abspath(__file__))
API = 'https://store.tildaapi.com/api/getproductslist/?storepartuid={uid}&c={ts}&flag_root=withroot'

# storepartuid -> категория (порядок как на aidabaran.ru/menu)
PARTS = [
    ('194623328422', 'Салаты'),
    ('240136778342', 'Холодные закуски'),
    ('517689024342', 'Супы'),
    ('732714363322', 'Хачапури'),
    ('264893697752', 'Хинкали'),
    ('234017945102', 'Горячие закуски'),
    ('560809522262', 'Чебуреки'),
    ('410613909802', 'Основные блюда'),
    ('723879739992', 'Блюда на углях'),
    ('237001137402', 'Овощи на углях и гарниры'),
    ('918834240312', 'Соусы'),
    ('154764542112', 'Десерты'),
    ('293021285612', 'Тандырный хлеб'),
    ('631420163442', 'Напитки'),
]

TR = {ord(a): b for a, b in zip('абвгдежзийклмнопрстуфхцчшщыьэюя', 'abvgdeezijklmnoprstufhcsshieia')}
TR[ord('ё')] = 'e'; TR[ord('ъ')] = ''; TR[ord('ь')] = ''


def slug(s: str) -> str:
    s = s.lower().translate(TR)
    s = re.sub(r'[^a-z0-9]+', '_', s).strip('_')
    return s[:60]


def price_to_int(v):
    if not v:
        return None
    s = re.sub(r'[^\d.]', '', str(v)).strip('.')
    try:
        return int(float(s)) if s else None
    except ValueError:
        return None


def fetch_menu():
    ctx = ssl.create_default_context()
    items = []
    for ts, (uid, name) in enumerate(PARTS):
        req = urllib.request.Request(API.format(uid=uid, ts=1000 + ts),
                                     headers={'User-Agent': 'Mozilla/5.0'})
        with urllib.request.urlopen(req, timeout=20, context=ctx) as r:
            data = json.load(r)
        time.sleep(0.3)
        for p in data['products']:
            gallery = []
            try:
                gallery = [g['img'] for g in json.loads(p.get('gallery') or '[]')]
            except json.JSONDecodeError:
                pass
            text = re.sub(r'<[^>]+>', ' ', p.get('text') or '')
            text = re.sub(r'\s+', ' ', text).strip()
            upsell = []
            for pr in p.get('properties') or []:
                for line in (pr.get('values') or '').split('\n'):
                    m = re.match(r'(.+?)\s*=\s*\+([\d\s.]+)', line.strip())
                    if m:
                        upsell.append({'name': m.group(1).strip(),
                                       'add_price': price_to_int(m.group(2))})
            items.append({
                'uid': p['uid'],
                'title': p['title'],
                'category': name,
                'price': price_to_int(p.get('price')),
                'description': (p.get('descr') or '').strip(),
                'kbju': text or None,
                'photos': gallery,
                'upsell_options': upsell,
                'editions': [{'price': price_to_int(e.get('price')), 'img': e.get('img', '')}
                             for e in p.get('editions') or []],
                'url': p.get('url', ''),
            })
    out = {
        'restaurant': 'АйДаБаран',
        'source': 'https://aidabaran.ru/menu (Tilda Catalog API: store.tildaapi.com/api/getproductslist/)',
        'parsed_at': time.strftime('%Y-%m-%d'),
        'items_count': len(items),
        'items': items,
    }
    with open(os.path.join(BASE, 'menu.json'), 'w', encoding='utf-8') as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
    print(f'menu.json: {len(items)} items')


def download_photos():
    ctx = ssl.create_default_context()
    menu = json.load(open(os.path.join(BASE, 'menu.json'), encoding='utf-8'))
    pdir = os.path.join(BASE, 'photos', 'dishes')
    os.makedirs(pdir, exist_ok=True)
    ok = fail = 0
    for item in menu['items']:
        img = (item.get('photos') or [None])[0] or (item.get('editions') or [{}])[0].get('img')
        fn = f"{slug(item['category'])}__{slug(item['title'])}.jpg"
        path = os.path.join(pdir, fn)
        item['local_photo'] = f'photos/dishes/{fn}'
        if os.path.exists(path):
            ok += 1
            continue
        if not img:
            print('NO IMG:', item['title'])
            fail += 1
            continue
        try:
            req = urllib.request.Request(img, headers={'User-Agent': 'Mozilla/5.0'})
            with urllib.request.urlopen(req, timeout=20, context=ctx) as r, open(path, 'wb') as f:
                f.write(r.read())
            ok += 1
        except Exception as e:
            print('FAIL:', item['title'], e)
            fail += 1
        time.sleep(0.1)
    with open(os.path.join(BASE, 'menu.json'), 'w', encoding='utf-8') as f:
        json.dump(menu, f, ensure_ascii=False, indent=1)
    print(f'photos: downloaded/present {ok}, failed {fail}')


if __name__ == '__main__':
    cmd = sys.argv[1] if len(sys.argv) > 1 else ''
    if cmd == 'menu':
        fetch_menu()
        download_photos()
    elif cmd == 'photos':
        download_photos()
    else:
        print(__doc__)
