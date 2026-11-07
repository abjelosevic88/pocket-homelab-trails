// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds eight months of hikes and rides around Sarajevo):
//
//   DATA_DIR=/tmp/trails-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Amar PORT=18254 ./pocket-trails
//   CDP=http://127.0.0.1:9344 BASE=http://127.0.0.1:18254 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18254'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 2500)
  assert.ok((await page.content()).includes('Coming up') || (await page.content()).includes('Lately'), 'Today shows outings')
  await shot(page, 'desktop-today')
  await go(page, '/routes', 1200)
  await shot(page, 'desktop-routes')
  const routes = await api(page, '/routes')
  const bj = routes.find(r => r.name.includes('Bjelašnica')) || routes[0]
  await go(page, `/routes/${bj.id}`, 3500)
  assert.ok((await page.content()).includes('Along the way'), 'a route shows its spots')
  await shot(page, 'desktop-route')
  const next = (await api(page, '/today')).current.outing
  await go(page, `/outings/${next.id}`, 3500)
  assert.ok((await page.content()).includes('What to pack'), 'an outing shows its packing list')
  await shot(page, 'desktop-outing')
  const done = (await api(page, '/outings?status=done&order=desc'))[1]
  await go(page, `/outings/${done.id}`, 3500)
  assert.ok((await page.content()).includes('How did it go'), 'a finished outing compares the track')
  await shot(page, 'desktop-outing-done')
  await go(page, '/map', 3500)
  await shot(page, 'desktop-map')
  await go(page, '/routes/draw', 2500)
  await shot(page, 'desktop-planner')
  await go(page, '/gear', 1200)
  await shot(page, 'desktop-gear')
  await page.locator('.segmented button', { hasText: 'Packing lists' }).click()
  await page.waitForTimeout(600)
  await shot(page, 'desktop-packs')
  await go(page, '/year', 1500)
  await shot(page, 'desktop-year')
  await go(page, '/settings/safety', 1200)
  await shot(page, 'desktop-settings-safety')
  await go(page, '/settings/apps', 1500)
  await shot(page, 'desktop-settings-apps')
  await go(page, '/', 1200)
  await page.keyboard.press('n')
  await page.waitForTimeout(900)
  await shot(page, 'desktop-plan-outing')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 2500)
  await shot(page, 'phone-today')
  const routes = await api(page, '/routes')
  await go(page, `/routes/${routes[0].id}`, 3500)
  await shot(page, 'phone-route')
  const next = (await api(page, '/today')).current.outing
  await go(page, `/outings/${next.id}`, 3000)
  await shot(page, 'phone-outing')
  await go(page, '/routes', 1200)
  await shot(page, 'phone-routes')
  await go(page, '/routes/draw', 2500)
  await shot(page, 'phone-planner')
  await go(page, '/', 800)
  await page.locator('.topbar .btn').first().click()
  await page.waitForTimeout(500)
  await shot(page, 'phone-menu')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/', 2500)
  await shot(page, 'desktop-today-dark')
  const routes = await api(page, '/routes')
  await go(page, `/routes/${routes[1].id}`, 3500)
  await shot(page, 'desktop-route-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
