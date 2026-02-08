#!/usr/bin/env node
import { chromium } from 'playwright';

const DEFAULT_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36';

function readStdin() {
  return new Promise((resolve) => {
    let data = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => {
      data += chunk;
    });
    process.stdin.on('end', () => resolve(data));
    process.stdin.on('error', () => resolve(data));
  });
}

function writeResult(payload) {
  process.stdout.write(JSON.stringify(payload));
}

async function main() {
  const raw = (await readStdin()).trim();
  if (!raw) {
    writeResult({ ok: false, error: 'missing request body' });
    return;
  }

  let req;
  try {
    req = JSON.parse(raw);
  } catch (err) {
    writeResult({ ok: false, error: 'invalid JSON request' });
    return;
  }

  const url = req.url;
  if (!url || typeof url !== 'string') {
    writeResult({ ok: false, error: 'url is required' });
    return;
  }

  const timeoutMs = Number.isFinite(req.timeoutMs) ? req.timeoutMs : 30000;
  const userAgent = typeof req.userAgent === 'string' && req.userAgent.trim() ? req.userAgent : DEFAULT_UA;
  const waitUntil = typeof req.waitUntil === 'string' && req.waitUntil.trim() ? req.waitUntil : 'domcontentloaded';

  let browser;
  try {
    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
      userAgent,
      locale: 'en-US',
      viewport: { width: 1280, height: 720 },
      extraHTTPHeaders: {
        'Accept-Language': 'en-US,en;q=0.9',
      },
    });
    const page = await context.newPage();
    await page.goto(url, { waitUntil, timeout: timeoutMs });
    const title = await page.title();
    const text = await page.evaluate(() => (document.body ? document.body.innerText : ''));
    writeResult({ ok: true, url, title, text });
  } catch (err) {
    const message = err && typeof err.message === 'string' ? err.message : String(err);
    writeResult({ ok: false, error: message });
  } finally {
    if (browser) {
      await browser.close();
    }
  }
}

process.on('unhandledRejection', (err) => {
  const message = err && typeof err.message === 'string' ? err.message : String(err);
  writeResult({ ok: false, error: message });
});

process.on('uncaughtException', (err) => {
  const message = err && typeof err.message === 'string' ? err.message : String(err);
  writeResult({ ok: false, error: message });
});

main();
