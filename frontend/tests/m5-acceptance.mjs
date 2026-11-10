// FR/UI/AN/NFR acceptance against the isolated backend, never Vite or the pilot DB.
/* global document, getComputedStyle */
import assert from 'node:assert/strict';
import { Buffer } from 'node:buffer';
import { createHash, randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { setTimeout as wait } from 'node:timers/promises';
import { URL } from 'node:url';

const origin = 'http://127.0.0.1:18080';
const project = 'vkr-m5-acceptance';
const db = `${project}-db-1`;
const backend = `${project}-backend-1`;
const output = process.env.M5_EVIDENCE_DIR;
assert.ok(output && path.isAbsolute(output), 'Set M5_EVIDENCE_DIR outside the repository');
assert.ok(process.env.PLAYWRIGHT_MODULE, 'Use the existing external Playwright installation');
const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE);
const results = [];
const header = 'ts,session_id,table_id,name,payload\n';
const names = ['page_open', 'menu_view', 'category_select', 'dish_card_open', 'rec_feed_visible',
  'rec_impression', 'menu_add_click', 'rec_add_click', 'cart_open', 'cart_qty_change',
  'cart_item_remove', 'pay_click', 'stub_view'].sort();
const dishID = 655892482392;
const descriptionID = 588160226782;
const version = 'm5-auth-pgx-r1';
const credentials = new Set();
const analyticIDs = new Set();
const sha = value => createHash('sha256').update(value).digest('hex');
let browser;

function docker(args, options = {}) {
  return spawnSync('docker', args, { encoding: 'utf8', timeout: 120000, maxBuffer: 16 * 1024 * 1024, ...options });
}
function successful(result) {
  assert.equal(result.status, 0, 'Docker command failed; inspect isolated stack privately');
  return result.stdout;
}
function sql(query, variables = {}) {
  const params = Object.entries(variables).flatMap(([key, value]) => {
    assert.match(key, /^[a-z_]+$/);
    assert.match(value, /^[0-9a-f-]+$/);
    return ['-v', `${key}=${value}`];
  });
  return successful(docker(['exec', '-i', db, 'psql', '-X', '-q', '-v', 'ON_ERROR_STOP=1', ...params,
    '-U', 'svoyshef', '-d', 'svoyshef', '-At'], { input: query })).trim();
}
function correlation(credential) {
  assert.match(credential, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
  credentials.add(credential.toLowerCase());
  const row = JSON.parse(sql(`SELECT row_to_json(s) FROM
    (SELECT id::text,table_id,created_at,expires_at,octet_length(credential_hash) AS hash_bytes,
     extract(epoch FROM expires_at-created_at)::int AS lifetime_seconds FROM sessions
     WHERE credential_hash=decode(:'hash','hex')) s`, { hash: sha(credential.toLowerCase()) }));
  assert.ok(row && row.id !== credential.toLowerCase() && row.hash_bytes === 32, 'Independent credential/hash correlation failed');
  analyticIDs.add(row.id);
  return row;
}
function noCredentialLeak(text) {
  const lower = text.toLowerCase();
  assert.ok([...credentials].every(value => !lower.includes(value) && !lower.includes(sha(value))), 'Credential or hash leaked');
  assert.ok(!/credential_hash|expires_at/.test(lower), 'Auth fields leaked');
}
const baseline = () => Number(sql('SELECT coalesce(max(id),0) FROM events'));
function events(after) {
  assert.ok(Number.isSafeInteger(after) && after >= 0);
  return JSON.parse(sql(`SELECT coalesce(json_agg(e ORDER BY id),'[]') FROM
    (SELECT id,session_id,table_id,name,payload FROM events WHERE id>${after}) e`));
}
function cli(...args) {
  return docker(['exec', backend, '/app/backend/server', ...args]);
}
async function test(name, fn) {
  try {
    const evidence = await fn();
    results.push({ name, pass: true, evidence: evidence ?? null });
    process.stdout.write(`PASS ${name}\n`);
  } catch (error) {
    results.push({ name, pass: false, error: error.message.slice(0, 300) });
    throw error;
  }
}
async function until(fn, message) {
  for (let i = 0; i < 100; i++) {
    if (await fn()) return;
    await wait(200);
  }
  assert.fail(message);
}
async function api(page, route) {
  const response = await page.request.get(`${origin}/api/v1${route}`);
  assert.equal(response.status(), 200);
  return response.json();
}
async function snapshot(page, label, width) {
  await page.evaluate(() => document.fonts.ready);
  const state = await page.evaluate(() => {
    let labels = document.body.innerText;
    for (const el of document.querySelectorAll('.dish-card__title,.dish__title,.rec-card__title,.cart-line__name')) {
      labels = labels.replaceAll(el.innerText, '');
    }
    return {
    width: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth),
    forms: document.querySelectorAll('form,input,textarea,select').length,
    latin: (labels.match(/[A-Za-z]{3,}/g) || []),
    small: [...document.querySelectorAll('button,a')].filter(el => {
      const r = el.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && (r.width < 44 || r.height < 44);
    }).map(el => el.className),
    };
  });
  assert.ok(state.width <= width, `${label}: horizontal overflow`);
  assert.equal(state.forms, 0);
  assert.deepEqual(state.latin, [], `${label}: English UI text`);
  assert.deepEqual(state.small, [], `${label}: targets smaller than 44px`);
  await page.screenshot({ path: path.join(output, `${width}-${label}.png`) });
  return state;
}
async function badge(page, expected) {
  await page.waitForFunction(n => Number(document.querySelector('.header__cart-badge')?.textContent || 0) === n, expected);
}
async function cartMatches(page) {
  const cart = await api(page, '/cart');
  for (const line of cart.lines) {
    const parent = page.locator(`.cart-line[data-item-id="${line.item.id}"]`);
    const img = parent.locator('img.cart-line__img');
    await parent.scrollIntoViewIfNeeded();
    await until(() => img.evaluate(el => el.complete && el.naturalWidth > 0), 'cart preview did not decode');
    assert.equal(await img.getAttribute('src'), line.item.photo_fallback);
  }
  const lines = await page.locator('.cart-line').evaluateAll(nodes => nodes.map(el => ({
    name: el.querySelector('.cart-line__name').textContent.trim(),
    qty: Number(el.querySelector('.qty-stepper__value').textContent),
    price: Number(el.querySelector('.cart-line__price').textContent.split('₽')[0].replace(/[^0-9]/g, '')),
    image: !!el.querySelector('img.cart-line__img'),
  })));
  assert.deepEqual(lines, cart.lines.map(l => ({ name: l.item.name, qty: l.qty, price: l.item.price, image: true })));
  const total = cart.lines.reduce((sum, l) => sum + l.item.price * l.qty, 0);
  assert.equal(cart.total, total);
  assert.equal(Number((await page.locator('.cart-total__sum').innerText()).replace(/[^0-9]/g, '')), total);
  await badge(page, cart.lines.reduce((sum, l) => sum + l.qty, 0));
  return cart;
}

await mkdir(output, { recursive: true });
try {
  await test('NFR05_isolated_project_and_latest_backend_regressions', async () => {
    for (const container of [db, backend]) {
      assert.equal(successful(docker(['inspect', '-f', '{{index .Config.Labels "com.docker.compose.project"}}', container])).trim(), project);
    }
    const response = await globalThis.fetch(`${origin}/api/v1/does-not-exist`);
    assert.equal(response.status, 404);
    assert.match(response.headers.get('content-type'), /application\/json/);
    const invalid = cli('export-events', '--from=');
    assert.equal(invalid.status, 1);
    assert.equal(invalid.stdout, '');
    assert.ok(invalid.stderr.length > 0);
    assert.equal(sql('SELECT max(version_id) FROM goose_db_version WHERE is_applied'), '3');
  });
  await test('FR33_NFR04_independent_credential_reuse_CSV_UUID_replay_expiry', async () => {
    const sessions = sql('SELECT count(*) FROM sessions');
    const health = await globalThis.fetch(`${origin}/api/v1/health`);
    assert.equal(health.status, 200);
    assert.equal(health.headers.get('set-cookie'), null);
    assert.equal(sql('SELECT count(*) FROM sessions'), sessions);
    const request = async (credential, route = '/cart', options = {}) => {
      const response = await globalThis.fetch(`${origin}/api/v1${route}`, {
        ...options, headers: { ...(credential ? { cookie: `session=${credential}` } : {}),
          ...(options.body ? { 'content-type': 'application/json' } : {}) },
      });
      assert.ok(response.status === 200 || response.status === 201, 'Auth HTTP request failed');
      const text = await response.text();
      noCredentialLeak(text);
      const cookie = response.headers.get('set-cookie');
      const issued = cookie?.match(/^session=([^;]+);/)?.[1];
      if (issued) correlation(issued);
      return { body: JSON.parse(text), cookie, issued };
    };
    let owner = await request(null, '/cart?table=12');
    assert.equal(owner.body.lines.length, 0);
    assert.match(owner.cookie, /HttpOnly/);
    assert.match(owner.cookie, /SameSite=Lax/);
    assert.match(owner.cookie, /Path=\//);
    const credential = owner.issued;
    const state = correlation(credential);
    assert.equal(state.lifetime_seconds, Number(owner.cookie.match(/Max-Age=(\d+)/)[1]));
    await request(credential, '/cart/items', { method: 'POST', body: JSON.stringify({ item_id: dishID, source: 'menu' }) });
    for (const value of [credential, credential.toUpperCase()]) {
      const reused = await request(value, '/cart?table=99');
      assert.equal(reused.body.lines.length, 1);
      assert.equal(reused.cookie, null);
      const unchanged = correlation(credential);
      assert.ok(unchanged.id === state.id && unchanged.expires_at === state.expires_at && unchanged.table_id === 12, 'Reuse changed identity, expiry or table');
    }
    const exported = successful(cli('export-events'));
    assert.ok(exported.includes(state.id), 'Analytics ID missing from CSV');
    noCredentialLeak(exported);
    const replay = await request(state.id, '/cart?table=24');
    assert.equal(replay.body.lines.length, 0);
    assert.ok(replay.issued && replay.issued !== credential);
    assert.equal(correlation(replay.issued).table_id, 24);
    assert.equal((await request(credential)).body.lines.length, 1);
    const historicalID = sql(`SELECT DISTINCT e.session_id::text FROM events e
      JOIN sessions s ON s.id=e.session_id JOIN cart_items c ON c.session_id=s.id
      WHERE s.credential_hash IS NULL AND s.expires_at IS NULL ORDER BY 1 LIMIT 1`);
    assert.match(historicalID, /^[0-9a-f-]{36}$/);
    assert.ok(exported.includes(historicalID), 'Historical analytics ID missing from preserved CSV');
    const historicalLines = sql('SELECT count(*) FROM cart_items WHERE session_id=:\'sid\'::uuid', { sid: historicalID });
    assert.ok(Number(historicalLines) > 0);
    const historicalReplay = await request(historicalID, '/cart?table=48');
    assert.equal(historicalReplay.body.lines.length, 0);
    assert.ok(historicalReplay.issued);
    assert.equal(correlation(historicalReplay.issued).table_id, 48);
    assert.equal(sql('SELECT count(*) FROM cart_items WHERE session_id=:\'sid\'::uuid', { sid: historicalID }), historicalLines);
    for (const value of [randomUUID(), 'malformed']) {
      const replacement = await request(value);
      assert.equal(replacement.body.lines.length, 0);
      assert.ok(replacement.issued);
    }
    for (const boundary of ['past', 'equal']) {
      const key = owner.issued;
      if (boundary === 'past') {
        sql(`UPDATE sessions SET expires_at=now()-interval '1 second' WHERE credential_hash=decode(:'hash','hex')`, { hash: sha(key) });
      } else {
        const equality = JSON.parse(sql(`BEGIN;
          UPDATE sessions SET expires_at=now() WHERE credential_hash=decode(:'hash','hex');
          SELECT json_build_object('equal',expires_at=now(),'active',expires_at>now()) FROM sessions WHERE credential_hash=decode(:'hash','hex');
          COMMIT;`, { hash: sha(key) }));
        assert.deepEqual(equality, { equal: true, active: false });
      }
      const previous = correlation(key);
      const expired = await request(key, '/cart?table=36');
      assert.equal(expired.body.lines.length, 0);
      assert.ok(expired.issued && expired.issued !== key);
      assert.equal(correlation(expired.issued).table_id, 36);
      assert.equal(Number(sql('SELECT count(*) FROM cart_items WHERE session_id=:\'sid\'::uuid', { sid: state.id })), 1);
      assert.ok(correlation(key).id === previous.id, 'Expiry removed historical session');
      owner = expired;
    }
    return { independentCredential: true, hashBytes: 32, absoluteExpiry: true, reuse: true,
      exportedUUIDReplayRejected: true, historicalExportUUIDReplayRejected: true, unknownMalformedRejected: true, pastExpired: true,
      exactEqualityInactive: true, historicalCartPreserved: true };
  });
  browser = await chromium.launch();
  for (const width of [375, 1440]) {
    const start = baseline();
    const context = await browser.newContext({ viewport: { width, height: width === 375 ? 812 : 900 } });
    const page = await context.newPage();
    page.setDefaultTimeout(15000);
    const fontRequests = new Set();
    const cspErrors = [];
    const pageErrors = [];
    const blockedUrls = new Set();
    let blockedPhotos = 0;
    page.on('request', request => { if (request.resourceType() === 'font') fontRequests.add(request.url()); });
    page.on('console', message => { if (/Content Security Policy|Refused to/i.test(message.text())) cspErrors.push(message.text()); });
    page.on('pageerror', error => pageErrors.push(error.message));
    await context.route(/https:\/\/[^/]*tildacdn[^/]*\//, route => {
      blockedPhotos++;
      blockedUrls.add(route.request().url());
      return route.fulfill({ status: 404, body: '' });
    });

    await test(`FR01_FR02_FR04_FR06_FR25_FR26_FR27_menu_${width}`, async () => {
      const document = await page.goto(`${origin}/?table=12`);
      assert.equal(document.status(), 200);
      assert.ok(document.headers()['content-security-policy']);
      await page.waitForSelector('.dish-card');
      const menu = await api(page, '/menu');
      const flat = menu.categories.flatMap(c => c.items);
      assert.equal(menu.categories.length, 14);
      assert.equal(flat.length, 125);
      assert.ok(await page.locator('.dish-card img').evaluateAll(nodes => nodes.length === 125 && nodes.every(img => img.loading === 'lazy')));
      assert.deepEqual(await page.locator('.chip').allTextContents(), ['Все', ...menu.categories.map(c => c.name)]);
      const cards = await page.locator('.dish-card').evaluateAll(nodes => nodes.map(el => ({
        href: el.getAttribute('href'), name: el.querySelector('.dish-card__title').textContent,
        price: Number(el.querySelector('.dish-card__price').textContent.replace(/[^0-9]/g, '')),
      })));
      assert.deepEqual(cards, flat.map(i => ({ href: `/dish/${i.id}`, name: i.name, price: i.price })));
      assert.deepEqual(await page.locator('a').evaluateAll(nodes => nodes.map(a => a.getAttribute('href')).filter(h => !(/^\/(dish\/\d+|cart|pay-stub)?$/.test(h)))), []);
      const config = await api(page, '/config');
      assert.equal(await page.locator('.header__title').innerText(), config.restaurant.name);
      assert.equal(await page.locator('.header__logo').getAttribute('src'), config.restaurant.logo_url);
      assert.equal(await page.locator('.chip.is-active').innerText(), 'Все');
      assert.equal(await page.locator('.chip.is-active').evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(253, 195, 0)');
      assert.equal(await page.locator('.dish-card').first().evaluate(el => getComputedStyle(el).cursor), 'pointer');
      if (width === 1440) {
        const card = page.locator('.dish-card').first();
        const before = await card.evaluate(el => [getComputedStyle(el).backgroundColor, getComputedStyle(el).borderColor]);
        await card.hover();
        await wait(300);
        assert.notDeepEqual(await card.evaluate(el => [getComputedStyle(el).backgroundColor, getComputedStyle(el).borderColor]), before);
      }
      await snapshot(page, 'menu', width);
      for (let i = 0; i < flat.length; i++) {
        await page.locator('.dish-card').nth(i).scrollIntoViewIfNeeded();
        await page.waitForFunction(({ index, src }) => {
          const img = document.querySelectorAll('.dish-card img')[index];
          return img?.getAttribute('src') === src && img.complete && img.naturalWidth > 0;
        }, { index: i, src: flat[i].photo_fallback });
      }
      assert.ok(blockedPhotos >= new Set(flat.map(i => i.photo_url).filter(Boolean)).size,
        `Blocked ${blockedPhotos} requests; shared photos are fetched once`);
      assert.ok([...blockedUrls].every(url => url.includes('/-/resize/400x400/')));
      const cat = menu.categories.find(c => c.name === 'Хачапури');
      await page.getByRole('button', { name: 'Хачапури', exact: true }).click();
      assert.deepEqual(await page.locator('.dish-card__title').allTextContents(), cat.items.map(i => i.name));
      await page.waitForFunction(() => getComputedStyle(document.querySelector('.chip.is-active')).backgroundColor === 'rgb(253, 195, 0)');
      assert.equal(await page.locator('.chip.is-active').evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(253, 195, 0)');
      await until(() => events(start).some(e => e.name === 'category_select' && e.payload.category_id === cat.id), 'category_select missing in DB');
      await page.getByRole('button', { name: 'Все', exact: true }).click();
      assert.equal(await page.locator('.dish-card').count(), 125);
      return { cards: 125, categories: 14, decodedLocalFallbacks: 125, blockedPhotos };
    });

    await test(`FR03_FR05_FR06_FR07_FR09_FR13_FR14_FR15_FR30_FR32_dish_${width}`, async () => {
      await page.locator(`a[href="/dish/${dishID}"]`).click();
      await page.waitForSelector('.dish__title');
      assert.equal(new URL(page.url()).pathname, `/dish/${dishID}`);
      const dish = await api(page, `/menu/items/${dishID}`);
      assert.equal(await page.locator('.dish__title').innerText(), dish.name);
      assert.equal(Number((await page.locator('.dish__price').innerText()).replace(/[^0-9]/g, '')), dish.price);
      assert.match(await page.locator('.dish__price').innerText(), /₽/);
      assert.equal(await page.locator('.dish__kbju').innerText(), dish.kbju);
      assert.equal(await page.locator('.dish__descr').innerText(), dish.description);
      assert.equal(await page.locator('.rec-feed__title').innerText(), 'Хорошо сочетается с');
      const order = await page.evaluate(() => {
        const selectors = ['.dish__img', '.dish__head', '.dish__composition', '.rec-feed'];
        const boxes = selectors.map(s => document.querySelector(s).getBoundingClientRect());
        const cards = [...document.querySelectorAll('.rec-card')].map(el => el.getBoundingClientRect());
        return boxes.every((r, i) => i === 0 || r.top >= boxes[i - 1].bottom) &&
          cards.every((r, i) => i === 0 || (r.top >= cards[i - 1].bottom && Math.abs(r.x - cards[0].x) < 1));
      });
      assert.ok(order, 'Photo/info/composition/feed and vertical card order');
      const recs = await api(page, `/menu/items/${dishID}/recommendations`);
      assert.deepEqual(await page.locator('.rec-card').evaluateAll(nodes => nodes.map(el => Number(el.dataset.itemId))), recs.recs.map(r => r.item.id));
      assert.deepEqual(await page.locator('.rec-card__reason').allInnerTexts(), recs.recs.map(r => r.reason_text));
      assert.ok(recs.recs.length >= 5);
      await page.evaluate(() => globalThis.scrollTo(0, 0));
      await page.waitForFunction(src => {
        const img = document.querySelector('img.dish__img');
        return img?.getAttribute('src') === src && img.complete && img.naturalWidth > 0;
      }, dish.photo_fallback);
      await snapshot(page, 'dish', width);
      await page.locator('.rec-feed__title').scrollIntoViewIfNeeded();
      await wait(300);
      await snapshot(page, 'recommendations', width);
      const rec = page.locator('.rec-card').first();
      await rec.scrollIntoViewIfNeeded();
      await wait(300);
      await page.evaluate(() => { globalThis.__acceptanceMark = 'same-document'; });
      const linesBeforeOpen = (await api(page, '/cart')).lines;
      const recID = recs.recs[0].item.id;
      const geometry = await rec.evaluate(el => {
        const photo = el.querySelector('.rec-card__visual').getBoundingClientRect();
        const text = el.querySelector('.rec-card__body').getBoundingClientRect();
        return { ratio: photo.width / photo.height, above: photo.bottom <= text.top + 1, width: photo.width, cardWidth: el.getBoundingClientRect().width };
      });
      assert.ok(Math.abs(geometry.ratio - 4 / 3) < 0.01 && geometry.above);
      assert.ok(geometry.width <= 600 && geometry.cardWidth <= width);
      await rec.locator('.rec-card__title').click();
      await page.waitForFunction(id => globalThis.location.pathname === `/dish/${id}` && document.querySelector('.dish__title'), recID);
      assert.equal(new URL(page.url()).pathname, `/dish/${recID}`);
      assert.deepEqual((await api(page, '/cart')).lines, linesBeforeOpen);
      await until(() => events(start).some(e => e.name === 'dish_card_open' && e.payload.item_id === recID && e.payload.source === 'rec_feed'), 'rec detail open source missing');
      await page.locator('.back-link').click();
      await page.waitForFunction(id => globalThis.location.pathname === `/dish/${id}` && document.querySelector('.dish__title'), dishID);
      await rec.getByRole('button').click();
      await badge(page, 1);
      assert.equal(await page.evaluate(() => globalThis.__acceptanceMark), 'same-document');
      assert.equal(new URL(page.url()).pathname, `/dish/${dishID}`);
      await until(() => events(start).some(e => e.name === 'rec_add_click' && e.payload.context === 'dish' && e.payload.item_id === recs.recs[0].item.id), 'dish add event missing');
      for (const count of [2, 3]) {
        const button = page.locator('.dish__buy button');
        await until(() => button.isEnabled(), 'dish add not enabled');
        await button.click();
        await badge(page, count);
      }
      const cart = await api(page, '/cart');
      assert.equal(cart.lines.find(l => l.item.id === dishID).qty, 2);
      assert.ok(events(start).some(e => e.name === 'menu_add_click' && e.payload.item_id === dishID));
      await page.goto(`${origin}/dish/${descriptionID}`);
      await page.waitForSelector('.dish__title');
      const onlyDescription = await api(page, `/menu/items/${descriptionID}`);
      assert.equal(onlyDescription.kbju, '');
      assert.ok(onlyDescription.description.trim());
      assert.equal(await page.locator('.dish__descr').innerText(), onlyDescription.description);
      assert.equal(await page.locator('.dish__kbju').count(), 0);
      await until(() => ['rec_feed_visible', 'rec_impression'].every(n => events(start).some(e => e.name === n)), 'real viewport events missing');
      return { orderedRecommendations: recs.recs.length, repeatedDishQty: 2, bothEmptyComposition: 'component test' };
    });

    await test(`FR17_FR18_FR19_FR20_FR23_FR27_FR32_cart_${width}`, async () => {
      await page.locator('.header__cart').click();
      await page.waitForSelector('.cart-line');
      assert.equal(new URL(page.url()).pathname, '/cart');
      let cart = await cartMatches(page);
      assert.equal(await page.locator('.rec-card').count(), 1);
      assert.equal(Number(await page.locator('.rec-card').getAttribute('data-item-id')), cart.recommendation.item.id);
      await snapshot(page, 'cart', width);
      const pay = await page.locator('.btn--pay').boundingBox();
      assert.ok(pay.y >= 0 && pay.y + pay.height <= page.viewportSize().height + 1);
      const previousRec = cart.recommendation.item.id;
      await page.locator('.rec-card').scrollIntoViewIfNeeded();
      await wait(300);
      await page.locator('.rec-card__link').click();
      await page.waitForFunction(id => globalThis.location.pathname === `/dish/${id}` && document.querySelector('.dish__title'), previousRec);
      assert.deepEqual((await api(page, '/cart')).lines, cart.lines);
      await until(() => events(start).some(e => e.name === 'dish_card_open' && e.payload.item_id === previousRec && e.payload.source === 'rec_cart'), 'cart detail open source missing');
      await page.locator('.back-link').click();
      await page.waitForSelector('.page--cart .rec-card');
      await page.locator('.rec-card button').click();
      await until(async () => (await page.locator('.cart-line').count()) === 3, 'cart recommendation not added');
      cart = await cartMatches(page);
      assert.ok(cart.lines.some(l => l.item.id === previousRec && l.added_from === 'rec_cart'));
      assert.notEqual(cart.recommendation?.item.id, previousRec);
      assert.equal(new URL(page.url()).pathname, '/cart');
      const line = page.locator('.cart-line', { has: page.locator('.cart-line__name', { hasText: cart.lines.find(l => l.item.id === dishID).item.name }) });
      for (const [label, qty] of [['Уменьшить количество', 1], ['Увеличить количество', 2]]) {
        await line.getByRole('button', { name: label }).click();
        await until(async () => Number(await line.locator('.qty-stepper__value').innerText()) === qty, 'qty not updated');
        await cartMatches(page);
        await until(() => events(start).some(e => e.name === 'cart_qty_change' && e.payload.item_id === dishID && e.payload.from === (qty === 1 ? 2 : 1) && e.payload.to === qty), 'qty event payload mismatch');
      }
      const removed = cart.lines.find(l => l.item.id !== dishID);
      await page.locator('.cart-line', { hasText: removed.item.name }).locator('.cart-line__remove').click();
      await until(async () => (await page.locator('.cart-line').count()) === 2, 'line not removed');
      cart = await cartMatches(page);
      assert.ok(!cart.lines.some(l => l.item.id === removed.item.id));
      await until(() => events(start).some(e => e.name === 'cart_item_remove' && e.payload.item_id === removed.item.id), 'remove event payload mismatch');
      for (let qty = 3; qty <= 20; qty++) {
        await line.getByRole('button', { name: 'Увеличить количество' }).click();
        await until(async () => Number(await line.locator('.qty-stepper__value').innerText()) === qty, 'qty cap test failed');
        await cartMatches(page);
      }
      assert.ok(await line.getByRole('button', { name: 'Увеличить количество' }).isDisabled());
      await cartMatches(page);
      return { maxQty: 20, recCartAdded: true, totalsComparedAfterEveryMutation: true };
    });

    await test(`UI04_FR36_FR37_availability_finally_${width}`, async () => {
      const original = sql(`SELECT coalesce((SELECT is_available::text FROM item_availability WHERE item_id=${dishID}),'absent')`);
      const before = baseline();
      try {
        successful(cli('availability', 'set', String(dishID), 'off'));
        await until(async () => !(await api(page, '/menu')).categories.flatMap(c => c.items).some(i => i.id === dishID), 'availability TTL did not expire');
        await page.reload();
        await page.waitForSelector('.cart-line--unavailable');
        assert.equal(await page.locator('.cart-line--unavailable').getByText('Нет в наличии').count(), 1);
        assert.ok(Number(await page.locator('.cart-line--unavailable .cart-line__img').evaluate(el => getComputedStyle(el).opacity)) < 1);
        assert.ok(await page.locator('.cart-line--unavailable .cart-line__remove').isEnabled());
        assert.ok(await page.locator('.cart-line--unavailable').getByRole('button', { name: 'Уменьшить количество' }).isEnabled());
        await page.locator('.btn--pay').click();
        assert.equal(new URL(page.url()).pathname, '/cart');
        assert.match(await page.locator('.cart-blocked-msg').innerText(), /Часть позиций больше нет в наличии/);
        await wait(400);
        assert.ok(!events(before).some(e => ['pay_click', 'stub_view'].includes(e.name)));
        await snapshot(page, 'unavailable', width);
      } finally {
        successful(cli('availability', 'set', String(dishID), original === 'false' ? 'off' : 'on'));
        if (original === 'absent') sql(`DELETE FROM item_availability WHERE item_id=${dishID}`);
      }
      await until(async () => (await api(page, '/menu')).categories.flatMap(c => c.items).some(i => i.id === dishID), 'availability not restored');
      await page.reload();
      await page.waitForSelector('.cart-line');
      assert.equal(await page.locator('.cart-line--unavailable').count(), 0);
      assert.equal(sql(`SELECT coalesce((SELECT is_available::text FROM item_availability WHERE item_id=${dishID}),'absent')`), original);
    });

    await test(`FR24_FR31_FR35_NFR06_payment_empty_retry_${width}`, async () => {
      await page.locator('.btn--pay').click();
      await page.waitForURL('**/pay-stub');
      assert.equal(await page.locator('.stub__text').innerText(), 'Ваш выбор сформирован. Позовите официанта.');
      await snapshot(page, 'stub', width);
      await page.locator('.page--stub .btn').click();
      await page.waitForSelector('.dish-card');
      const abort = route => route.abort('connectionfailed');
      await context.route('**/api/v1/cart', abort);
      try {
        await page.locator('.header__cart').click();
        await page.waitForSelector('.error-retry');
        assert.match(await page.locator('.error-retry').innerText(), /Что-то пошло не так, попробуйте ещё раз/);
        assert.doesNotMatch(await page.locator('body').innerText(), /Error:|SQL|stack|trace|ECONN|\/api\/|\b500\b/);
        assert.ok(await page.getByRole('button', { name: 'Повторить' }).isVisible());
        await snapshot(page, 'network-error', width);
      } finally {
        await context.unroute('**/api/v1/cart', abort);
      }
      await page.getByRole('button', { name: 'Повторить' }).click();
      await page.waitForSelector('.cart-line');
      while (await page.locator('.cart-line').count()) {
        const count = await page.locator('.cart-line').count();
        await page.locator('.cart-line__remove').first().click();
        await until(async () => (await page.locator('.cart-line').count()) === count - 1, 'empty cart removal failed');
      }
      assert.equal(await page.locator('.cart-empty__text').innerText(), 'Корзина пуста');
      assert.equal(await page.locator('.btn--pay,.rec-feed').count(), 0);
      await snapshot(page, 'empty-cart', width);
    });

    await test(`AN02_AN03_FR34_all_13_DB_events_table12_${width}`, async () => {
      await until(() => names.every(name => events(start).some(e => e.name === name)), 'not all 13 event names recorded');
      const rows = events(start);
      assert.deepEqual([...new Set(rows.map(e => e.name))].sort(), names);
      assert.ok(rows.every(e => e.table_id === 12));
      const cookie = (await context.cookies()).find(c => c.name === 'session');
      const identity = correlation(cookie.value);
      assert.ok(rows.every(e => e.session_id === identity.id), 'Browser events not correlated with hashed credential');
      assert.equal(await page.evaluate(() => document.cookie.includes('session=')), false);
      assert.ok(rows.some(e => e.name === 'rec_add_click' && e.payload.context === 'dish'));
      assert.ok(rows.some(e => e.name === 'rec_add_click' && e.payload.context === 'cart'));
      const pay = rows.findIndex(e => e.name === 'pay_click');
      const stub = rows.findIndex(e => e.name === 'stub_view');
      assert.ok(pay >= 0 && stub > pay);
      return { events: rows.length, names, table: 12, payBeforeStub: true };
    });
    await test(`UI01_NFR01_NFR03_NFR06_fonts_CSP_${width}`, async () => {
      await page.evaluate(() => document.fonts.ready);
      assert.ok(fontRequests.size >= 2);
      assert.ok([...fontRequests].every(url => url.startsWith(`${origin}/static/aidabaran/assets/fonts/`)));
      assert.ok([...fontRequests].some(url => url.includes('unbounded-cyrillic')));
      assert.ok([...fontRequests].some(url => url.includes('montserrat-cyrillic')));
      const fonts = await page.evaluate(() => ({ title: getComputedStyle(document.querySelector('h1')).fontFamily, body: getComputedStyle(document.body).fontFamily }));
      assert.match(fonts.title, /Unbounded/);
      assert.match(fonts.body, /Montserrat/);
      const cdp = await context.newCDPSession(page);
      await cdp.send('DOM.enable');
      await cdp.send('CSS.enable');
      const { root } = await cdp.send('DOM.getDocument');
      const rendered = [];
      for (const [selector, family] of [['h1', 'Unbounded'], ['.back-link', 'Montserrat']]) {
        const { nodeId } = await cdp.send('DOM.querySelector', { nodeId: root.nodeId, selector });
        const { fonts: actual } = await cdp.send('CSS.getPlatformFontsForNode', { nodeId });
        assert.ok(actual.some(font => font.isCustomFont && font.familyName.includes(family) && font.glyphCount > 0));
        rendered.push(family);
      }
      await cdp.detach();
      assert.deepEqual(cspErrors, []);
      assert.deepEqual(pageErrors, []);
      return { selfHostedFontRequests: fontRequests.size, fonts, renderedCustomFonts: rendered, cspErrors: 0, pageErrors: 0 };
    });
    await context.close();
  }

  await test('FR34_AN03_invalid_table_NULL_and_missing_table_NULL', async () => {
    for (const query of ['?table=abc', '']) {
      const start = baseline();
      const context = await browser.newContext();
      try {
        const page = await context.newPage();
        await page.goto(`${origin}/${query}`);
        await page.waitForSelector('.dish-card');
        await until(() => events(start).some(e => e.name === 'menu_view'), 'NULL table menu_view missing');
        assert.ok(events(start).every(e => e.table_id === null));
      } finally {
        await context.close();
      }
    }
  });
  await browser.close();
  browser = null;
  await wait(500);

  await test('AN05_all_13_payload_schemas_only_IDs_numbers_closed_enums', async () => {
    const integer = value => Number.isSafeInteger(value) && value >= 0;
    const id = value => integer(value) && value > 0;
    const context = value => ['dish', 'cart'].includes(value);
    const schemas = {
      page_open: {}, menu_view: {}, stub_view: {}, category_select: { category_id: integer },
      dish_card_open: { item_id: id, source: value => ['menu', 'rec_feed', 'rec_cart'].includes(value) },
      rec_feed_visible: { dish_id: id, recs_count: value => integer(value) && value <= 13 },
      rec_impression: { context, rec_item_id: id, rec_source: value => ['expert', 'upsell', 'heuristic'].includes(value) },
      menu_add_click: { item_id: id }, rec_add_click: { item_id: id, context },
      cart_open: { lines_count: integer, total: integer }, cart_qty_change: { item_id: id, from: id, to: id },
      cart_item_remove: { item_id: id }, pay_click: { total: integer, lines_count: integer },
    };
    const rows = events(0);
    assert.deepEqual(Object.keys(schemas).sort(), names);
    for (const row of rows) {
      assert.ok(Object.hasOwn(schemas, row.name));
      assert.deepEqual(Object.keys(row.payload).sort(), Object.keys(schemas[row.name]).sort());
      assert.ok(Buffer.byteLength(JSON.stringify(row.payload)) <= 1024);
      for (const [key, validate] of Object.entries(schemas[row.name])) assert.ok(validate(row.payload[key]), `Invalid ${row.name}.${key}`);
    }
    return { schemas: names.length, reviewedRows: rows.length, freeTextFields: 0 };
  });

  await test('AN04_CSV_exact_DB_bytes_dates_order_NULL_payload_header_LF_noBOM', async () => {
    const rows = JSON.parse(sql(`SELECT coalesce(json_agg(e ORDER BY ts,id),'[]') FROM
      (SELECT id,ts,to_char(ts AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"') AS stamp,
       session_id,table_id,name,payload::text AS payload FROM events) e`));
    assert.ok(rows.length > 0);
    const encode = selected => header + selected.map(r => `${r.stamp},${r.session_id},${r.table_id ?? ''},${r.name},"${r.payload.replaceAll('"', '""')}"\n`).join('');
    const day = rows[0].stamp.slice(0, 10);
    const variants = [
      { args: [], selected: rows },
      { args: ['--from', day], selected: rows.filter(r => r.stamp.slice(0, 10) >= day) },
      { args: ['--to', day], selected: rows.filter(r => r.stamp.slice(0, 10) <= day) },
      { args: ['--from', day, '--to', day], selected: rows.filter(r => r.stamp.slice(0, 10) === day) },
      { args: ['--from', '2000-01-01', '--to', '2000-01-02'], selected: [] },
    ];
    for (const { args, selected } of variants) {
      const csv = successful(cli('export-events', ...args));
      assert.equal(csv, encode(selected), 'CSV bytes differ from ordered canonical DB rows');
      assert.ok(!csv.includes('\r'));
      assert.ok(!csv.startsWith('\uFEFF'));
      assert.ok(Buffer.from(csv).equals(Buffer.from(encode(selected), 'utf8')));
    }
    const all = successful(cli('export-events'));
    noCredentialLeak(all);
    assert.ok(all.includes(',page_open,"{}"\n'));
    assert.ok(rows.some(r => r.table_id === null));
    assert.deepEqual([...new Set(rows.map(r => r.name))].sort(), names);
    assert.ok(rows.every(r => /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}Z$/.test(r.stamp)));
    return { rows: rows.length, exactByteComparisons: variants.length, names: names.length, nullTable: true };
  });
  await test('AN04_invalid_CLI_exit1_stderr_stdout_empty', async () => {
    for (const args of [['--from='], ['--to', ''], ['--from', '2026-1-01'], ['--bogus'], ['--to'],
      ['--from', '2026-11-21', '--to', '2026-11-20']]) {
      const result = cli('export-events', ...args);
      assert.equal(result.status, 1);
      assert.equal(result.stdout, '');
      assert.ok(result.stderr.trim());
    }
    const unavailable = docker(['exec', '-e', 'DATABASE_URL=postgres://127.0.0.1:1/unavailable?sslmode=disable&connect_timeout=1',
      backend, '/app/backend/server', 'export-events']);
    assert.equal(unavailable.status, 1);
    assert.equal(unavailable.stdout, '');
    assert.ok(unavailable.stderr.trim());
    return { cases: 7 };
  });
  await test('NFR04_AN05_no_credential_hash_or_analytics_ID_in_runtime_logs', async () => {
    const result = docker(['logs', backend]);
    const logs = result.stdout + result.stderr;
    noCredentialLeak(logs);
    assert.ok([...analyticIDs].every(id => !logs.includes(id)), 'Analytics ID leaked into logs');
    return { checkedCredentials: credentials.size, checkedAnalyticsIDs: analyticIDs.size, leaks: 0 };
  });
} catch (error) {
  process.stderr.write(`Acceptance stopped: ${error.message.slice(0, 500)}\n`);
  process.exitCode = 1;
} finally {
  await browser?.close();
  await writeFile(path.join(output, 'acceptance-summary.json'), JSON.stringify({
    version, origin, project, generated: new Date().toISOString(),
    image: successful(docker(['inspect', '-f', '{{.Image}}', backend])).trim(),
    testSourceSHA256: sha(await readFile(new URL(import.meta.url))), results,
    passed: results.filter(r => r.pass).length, failed: results.filter(r => !r.pass).length,
  }, null, 2), { flag: 'wx', mode: 0o600 });
  process.stdout.write(`TOTAL ${results.length}; FAILED ${results.filter(r => !r.pass).length}\n`);
}
