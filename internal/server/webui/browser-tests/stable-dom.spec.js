// Browser integration tests: deliberately excluded from `go test ./...`.
// See integration/browser-dom/README.md for the explicit Node/Playwright run.
const {test, expect} = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const repositoryRoot = path.resolve(__dirname, '../../../..');
const scriptDirectory = path.join(repositoryRoot, 'internal/server/webui/assets/js');

const controls = {
  controls: {
    button: {iconSize: 16, iconGap: 6, iconPosition: 'leading'},
    select: {
      chevronSize: 16, chevronGap: 6, minimumHeight: 32, menuPadding: 4,
      menuItemGap: 2, optionGap: 6, optionPaddingX: 8, optionPaddingY: 6,
      optionMinimumHeight: 28, selectionIndicatorWidth: 16,
      viewportInset: 8, menuGap: 4, menuMinimumWidth: 120,
    },
  },
};

const screen = {
  apiVersion: 'ciwi.ui/v1',
  kind: 'Screen',
  metadata: {name: 'stable-dom-test'},
  screen: {
    dataSources: [{name: 'probe'}],
    root: {
      component: 'page',
      children: [{
        component: 'column',
        visible: {binding: 'probe.ready'},
        children: [
          {component: 'text', id: 'probe-version', text: {binding: 'probe.version'}},
          {component: 'input', id: 'probe-input', input: {value: 'probe.input', placeholder: 'Edit'}},
          {
            component: 'select', id: 'probe-select',
            select: {options: 'probe.options', value: 'probe.selected', as: 'option', optionValue: 'option.value', optionLabel: 'option.label'},
          },
          {
            component: 'disclosure', id: 'probe-disclosure', text: {literal: 'Disclosure'},
            disclosure: {defaultExpanded: true, stateKey: 'probe-disclosure'},
            children: [{component: 'text', text: {binding: 'probe.detail'}}],
          },
          {
            component: 'list', id: 'probe-rows', repeat: {source: 'probe.rows', as: 'row', key: 'row.key'},
            children: [{
              component: 'disclosure', text: {binding: 'row.label'}, image: {asset: 'ciwi-logo'},
              disclosure: {defaultExpanded: true, stateKey: 'execution:{{row.key}}'},
              style: {role: 'execution-row', toneBinding: 'row.status'},
            }],
          },
          {
            component: 'button', id: 'probe-action', text: {literal: 'Run current'}, visible: {binding: 'probe.action_visible'},
            actions: [{on: 'activate', command: 'run-pipeline', arguments: {pipelineDbId: '{{probe.action_id}}'}}],
          },
          {
            component: 'button', id: 'probe-refresh', text: {literal: 'Refresh'},
            actions: [{on: 'activate', command: 'refresh'}],
          },
        ],
      }],
    },
  },
};

function model(overrides = {}) {
  return Object.assign({
    version: 'one', input: 'draft', detail: 'details', selected: 'a', action_id: '1', action_visible: true,
    options: [{value: 'a', label: 'Alpha'}, {value: 'b', label: 'Beta'}],
    rows: [{key: 'a', label: 'Execution A', status: 'running'}, {key: 'b', label: 'Execution B', status: 'waiting'}],
  }, overrides);
}

async function installFixture(page, models) {
  let viewIndex = 0;
  const actionRequests = [];
  await page.route('http://ciwi.test/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname === '/') {
      await route.fulfill({contentType: 'text/html', body: `<!doctype html>
        <html><head><style>
          .dsl-execution-row-status.dsl-status-accent { animation: ciwi-test-spin 10s linear infinite; }
          @keyframes ciwi-test-spin { to { transform: rotate(360deg); } }
        </style></head><body><div id="declarativeRoot"></div>
        <script>
          window.ciwiUIResourceURL = path => path;
          window.alerts = [];
          window.alert = message => window.alerts.push(String(message));
          window.EventSource = class EventSource {
            addEventListener() {}
            close() {}
          };
        </script>
        <script src="/ui/theme.js"></script>
        <script src="/ui/actions.js"></script>
        <script src="/ui/view-state.js"></script>
        <script src="/ui/heartbeat.js"></script>
        <script src="/ui/change-refresh.js"></script>
        <script src="/ui/declarative.js"></script>
      </body></html>`});
      return;
    }
    if (url.pathname.startsWith('/ui/') && url.pathname.endsWith('.js')) {
      const filename = path.basename(url.pathname);
      await route.fulfill({contentType: 'application/javascript', body: fs.readFileSync(path.join(scriptDirectory, filename), 'utf8')});
      return;
    }
    if (url.pathname === '/ui/contracts/routes.json') {
      await route.fulfill({json: {routes: [{name: 'stable-dom-test', pattern: '/', screen: 'stable-dom-test', bindingRoot: 'probe', platforms: ['web']}]}});
      return;
    }
    if (url.pathname === '/ui/contracts/screens/stable-dom-test.json') {
      await route.fulfill({json: screen});
      return;
    }
    if (url.pathname === '/ui/contracts/themes.json') {
      await route.fulfill({json: []});
      return;
    }
    if (url.pathname === '/ui/contracts/controls.json') {
      await route.fulfill({json: controls});
      return;
    }
    if (url.pathname === '/ui/contracts/actions.json') {
      await route.fulfill({json: {actions: []}});
      return;
    }
    if (url.pathname === '/api/v1/views/front-page') {
      const value = models[Math.min(viewIndex, models.length - 1)];
      viewIndex += 1;
      await route.fulfill({json: JSON.parse(JSON.stringify(value))});
      return;
    }
    if (/^\/api\/v1\/pipelines\/[^/]+\/run-selection$/.test(url.pathname)) {
      actionRequests.push(url.pathname);
      await route.fulfill({json: {}});
      return;
    }
    if (url.pathname === '/ciwi-logo.png' || url.pathname === '/ui/icons.svg') {
      await route.fulfill({status: 204, body: ''});
      return;
    }
    await route.fulfill({status: 404, body: 'not found'});
  });
  await page.goto('http://ciwi.test/');
  await expect(page.locator('#probe-version')).toHaveText(models[0].version);
  return {actionRequests};
}

async function refreshTo(page, version) {
  await page.locator('#probe-refresh').click();
  await expect(page.locator('#probe-version')).toHaveText(version);
}

test('renderer keys remain deterministic and status SVG identity survives compatible refreshes', async ({page}) => {
  await installFixture(page, [model(), model({version: 'two', detail: 'updated'})]);
  const before = await page.locator('#probe-input, #probe-select, #probe-disclosure, [data-disclosure-key="execution:a"]')
    .evaluateAll(elements => elements.map(element => element.dataset.ciwiNodeKey));
  await expect.poll(() => page.locator('.dsl-execution-row-status.dsl-status-accent').first().evaluate(element => (
    element.getAnimations()[0] && element.getAnimations()[0].currentTime
  ))).toBeGreaterThan(25);
  await page.evaluate(() => {
    window.previousSpinner = document.querySelector('.dsl-execution-row-status.dsl-status-accent');
    window.previousSpinnerAnimation = window.previousSpinner.getAnimations()[0];
    window.previousSpinnerAnimationTime = window.previousSpinnerAnimation.currentTime;
    window.previousSpinnerAnimationStartTime = window.previousSpinnerAnimation.startTime;
  });
  await page.locator('#probe-refresh').evaluate(element => element.click());
  await expect(page.locator('#probe-version')).toHaveText('two');
  const after = await page.locator('#probe-input, #probe-select, #probe-disclosure, [data-disclosure-key="execution:a"]')
    .evaluateAll(elements => elements.map(element => element.dataset.ciwiNodeKey));
  expect(after).toEqual(before);
  await expect.poll(() => page.evaluate(() => window.previousSpinner === document.querySelector('.dsl-execution-row-status.dsl-status-accent'))).toBe(true);
  await expect.poll(() => page.evaluate(() => window.previousSpinner.getAnimations()[0].startTime)).toBe(
    await page.evaluate(() => window.previousSpinnerAnimationStartTime),
  );
  expect(await page.evaluate(() => window.previousSpinner.getAnimations()[0].currentTime)).toBeGreaterThanOrEqual(
    await page.evaluate(() => window.previousSpinnerAnimationTime),
  );
});

test('an incompatible terminal icon replaces the retained status SVG', async ({page}) => {
  await installFixture(page, [model(), model({version: 'done', rows: [{key: 'a', label: 'Execution A', status: 'succeeded'}]})]);
  await page.evaluate(() => { window.previousSpinner = document.querySelector('.dsl-execution-row-status.dsl-status-accent'); });
  await refreshTo(page, 'done');
  expect(await page.evaluate(() => window.previousSpinner === document.querySelector('.dsl-execution-row-status'))).toBe(false);
  await expect(page.locator('.dsl-execution-row-status')).toHaveClass(/dsl-status-success/);
});

test('duplicate repeat keys abort before replacing the mounted screen', async ({page}) => {
  await installFixture(page, [model(), model({version: 'invalid', rows: [
    {key: 'same', label: 'First', status: 'waiting'},
    {key: 'same', label: 'Second', status: 'waiting'},
  ]})]);
  await page.locator('#probe-refresh').click();
  await expect(page.locator('#probe-version')).toHaveText('one');
  await expect.poll(() => page.evaluate(() => window.alerts.join('\n'))).toContain('Duplicate repeat key "same"');
});

test('empty repeat keys abort before replacing the mounted screen', async ({page}) => {
  await installFixture(page, [model(), model({version: 'invalid', rows: [{key: ' ', label: 'Empty', status: 'waiting'}]})]);
  await page.locator('#probe-refresh').click();
  await expect(page.locator('#probe-version')).toHaveText('one');
  await expect.poll(() => page.evaluate(() => window.alerts.join('\n'))).toContain('Empty repeat key');
});

test('a retained action element uses the newest scope exactly once', async ({page}) => {
  const fixture = await installFixture(page, [
    model(),
    model({version: 'two', action_id: '2'}),
    model({version: 'three', action_id: '3'}),
    model({version: 'four', action_id: '42'}),
  ]);
  const oldButton = await page.locator('#probe-action').elementHandle();
  await refreshTo(page, 'two');
  await refreshTo(page, 'three');
  await refreshTo(page, 'four');
  expect(await page.evaluate(button => document.getElementById('probe-action') === button, oldButton)).toBe(true);
  await page.locator('#probe-action').click();
  await expect.poll(() => fixture.actionRequests).toEqual(['/api/v1/pipelines/42/run-selection']);
});

test('a removed action identity becomes inert', async ({page}) => {
  const fixture = await installFixture(page, [model(), model({version: 'two', action_visible: false})]);
  const oldButton = await page.locator('#probe-action').elementHandle();
  await refreshTo(page, 'two');
  await page.evaluate(button => document.getElementById('declarativeRoot').appendChild(button), oldButton);
  await page.locator('#probe-action').click();
  await page.waitForTimeout(50);
  expect(fixture.actionRequests).toEqual([]);
});

test('focused input mounted identity survives a same-route refresh', async ({page}) => {
  await installFixture(page, [model(), model({version: 'two', detail: 'updated'})]);
  await page.locator('#probe-input').focus();
  await page.locator('#probe-input').evaluate(element => {
    element.setSelectionRange(1, 3);
    window.previousInput = element;
  });
  await page.locator('#probe-refresh').evaluate(element => element.click());
  await expect(page.locator('#probe-version')).toHaveText('two');
  expect(await page.evaluate(() => window.previousInput === document.getElementById('probe-input'))).toBe(true);
  expect(await page.evaluate(() => document.activeElement === window.previousInput)).toBe(true);
  expect(await page.locator('#probe-input').evaluate(element => [element.selectionStart, element.selectionEnd])).toEqual([1, 3]);
});

test('input edits survive unchanged backing data and authoritative changes still apply', async ({page}) => {
  await installFixture(page, [
    model(),
    model({version: 'two'}),
    model({version: 'three', input: 'server replacement'}),
  ]);
  await page.locator('#probe-input').fill('local edit');
  await page.locator('#probe-refresh').evaluate(element => element.click());
  await expect(page.locator('#probe-version')).toHaveText('two');
  await expect(page.locator('#probe-input')).toHaveValue('local edit');
  await page.locator('#probe-refresh').evaluate(element => element.click());
  await expect(page.locator('#probe-version')).toHaveText('three');
  await expect(page.locator('#probe-input')).toHaveValue('server replacement');
});

test('[expected failure: slice 4] open custom select survives an unrelated refresh', async ({page}) => {
  test.fail(true, 'Stateful component reconciliation is delivery slice 4');
  await installFixture(page, [model(), model({version: 'two', detail: 'updated'})]);
  await page.locator('#probe-select').click();
  await expect(page.locator('.ciwi-select-menu')).toBeVisible();
  await page.locator('#probe-refresh').evaluate(element => element.click());
  await expect(page.locator('#probe-version')).toHaveText('two');
  await expect(page.locator('.ciwi-select-menu')).toBeVisible();
});

test('expanded disclosure mounted identity survives a same-route refresh', async ({page}) => {
  await installFixture(page, [model(), model({version: 'two', detail: 'updated'})]);
  await page.evaluate(() => { window.previousDisclosure = document.getElementById('probe-disclosure'); });
  await refreshTo(page, 'two');
  expect(await page.evaluate(() => window.previousDisclosure === document.getElementById('probe-disclosure'))).toBe(true);
  expect(await page.locator('#probe-disclosure').evaluate(element => element.open)).toBe(true);
});

test('keyed row mounted identity survives sibling insertion and reorder', async ({page}) => {
  await installFixture(page, [model(), model({version: 'two', rows: [
    {key: 'new', label: 'New', status: 'waiting'},
    {key: 'b', label: 'Execution B', status: 'waiting'},
    {key: 'a', label: 'Execution A', status: 'running'},
  ]})]);
  await page.evaluate(() => { window.previousRow = document.querySelector('[data-disclosure-key="execution:b"]'); });
  await refreshTo(page, 'two');
  expect(await page.evaluate(() => window.previousRow === document.querySelector('[data-disclosure-key="execution:b"]'))).toBe(true);
});
