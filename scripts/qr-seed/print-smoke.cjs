// Optional QA-only browser test. Uses an existing Playwright installation, not runtime.
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');

(async () => {
  const [input, output, count = '20'] = process.argv.slice(2);
  assert(input && output, 'usage: node print-smoke.cjs INDEX_HTML OUTPUT_PDF [QR_COUNT]');
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({ offline: true });
    const page = await context.newPage();
    await page.goto(pathToFileURL(path.resolve(input)).href);
    await page.waitForFunction(() => [...document.images].every(img => img.complete && img.naturalWidth > 0));
    assert.equal(await page.locator('.cell').count(), Number(count));
    await page.emulateMedia({ media: 'print' });
    assert.equal(await page.locator('.header').evaluate(el => getComputedStyle(el).display), 'none');
    assert.equal(await page.locator('.cell').first().evaluate(el => getComputedStyle(el).breakInside), 'avoid');
    assert.equal(await page.evaluate(() => document.querySelectorAll('script, iframe, link').length), 0);
    const pdf = await page.pdf({ path: output, preferCSSPageSize: true, printBackground: true, displayHeaderFooter: false });
    assert.equal(pdf.subarray(0, 5).toString(), '%PDF-');
    assert(pdf.length > 10000, 'unexpectedly empty PDF');
    await page.screenshot({ path: output + '.png', fullPage: true });
    console.log(`PASS: offline A4 print, ${count} loaded QR images, no split cards; PDF ${pdf.length} bytes`);
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
