// FR-33/NFR-04/AN-04: one-time migration acceptance, isolated project only.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const project = 'vkr-m5-acceptance';
const db = `${project}-db-1`;
const backend = `${project}-backend-1`;
const origin = 'http://127.0.0.1:18080';
const output = process.env.M5_EVIDENCE_DIR;
assert.ok(output && path.isAbsolute(output));
assert.ok(process.env.COMPOSE_ENV_FILES?.endsWith('/m5-final-qa/acceptance.env'));
const sha = value => createHash('sha256').update(value).digest('hex');
function docker(args) {
  const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 300000, maxBuffer: 16 * 1024 * 1024 });
  assert.ok(result.status === 0, 'Isolated Docker operation failed; inspect privately');
  return result.stdout;
}
function sql(query) {
  return docker(['exec', db, 'psql', '-X', '-v', 'ON_ERROR_STOP=1', '-U', 'svoyshef', '-d', 'svoyshef', '-Atc', query]).trim();
}
function snapshot() {
  return sql(`SELECT jsonb_build_object(
    'sessions',(SELECT coalesce(jsonb_agg(to_jsonb(s)-'credential_hash'-'expires_at' ORDER BY id),'[]') FROM sessions s),
    'carts',(SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY session_id),'[]') FROM carts c),
    'cart_items',(SELECT coalesce(jsonb_agg(to_jsonb(c) ORDER BY session_id,item_id),'[]') FROM cart_items c),
    'events',(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),'[]') FROM events e),
    'availability',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY item_id),'[]') FROM item_availability a))::text`);
}
for (const container of [db, backend]) {
  assert.equal(docker(['inspect', '-f', '{{index .Config.Labels "com.docker.compose.project"}}', container]).trim(), project);
}
assert.equal(sql('SELECT max(version_id) FROM goose_db_version WHERE is_applied'), '2', 'This one-time test requires the preserved pre-0003 database');
const historicalCSV = docker(['exec', backend, '/app/backend/server', 'export-events']);
docker(['compose', '-p', project, 'stop', 'backend']);
assert.equal(docker(['inspect', '-f', '{{.State.Running}}', backend]).trim(), 'false');
const before = snapshot();
const counts = Object.fromEntries(Object.entries(JSON.parse(before)).map(([key, rows]) => [key, rows.length]));
const legacyID = sql('SELECT session_id::text FROM cart_items ORDER BY session_id LIMIT 1');
assert.match(legacyID, /^[0-9a-f-]{36}$/);
docker(['compose', '-p', project, 'build', 'backend']);
docker(['compose', '-p', project, 'up', '-d', '--wait', 'backend']);
assert.equal(sql('SELECT max(version_id) FROM goose_db_version WHERE is_applied'), '3');
assert.ok(snapshot() === before, 'Migration changed historical research rows or links');
assert.equal(Number(sql('SELECT count(*) FROM sessions WHERE credential_hash IS NULL AND expires_at IS NULL')), counts.sessions);
assert.ok(docker(['exec', backend, '/app/backend/server', 'export-events']) === historicalCSV, 'Historical CSV bytes changed during migration');
const replay = await globalThis.fetch(`${origin}/api/v1/cart?table=24`, { headers: { cookie: `session=${legacyID}` } });
assert.equal(replay.status, 200);
assert.equal((await replay.json()).lines.length, 0);
assert.ok(replay.headers.get('set-cookie')?.startsWith('session='));
assert.ok(!replay.headers.get('set-cookie').startsWith(`session=${legacyID};`), 'Legacy ID was reused as credential');
assert.equal(sql('SELECT count(*) FROM cart_items'), String(counts.cart_items));
assert.equal(sql('SELECT count(*) FROM events'), String(counts.events));
const record = {
  version: 'm5-auth-pgx-r1', generated: new Date().toISOString(), project, origin,
  image: docker(['inspect', '-f', '{{.Image}}', backend]).trim(),
  migration: '0002 -> 0003', counts, preservedResearchSHA256: sha(before),
  historicalCSVSHA256: sha(historicalCSV), historicalCSVUnchanged: true,
  legacyCredentialsNotBackfilled: true, legacyReplayNewEmptyCart: true, pass: true,
};
await mkdir(output, { recursive: true });
await writeFile(path.join(output, 'cutover-summary.json'), JSON.stringify(record, null, 2), { flag: 'wx', mode: 0o600 });
process.stdout.write(`PASS isolated cutover 0002->0003; ${counts.events} historical events preserved; legacy replay rejected\n`);
