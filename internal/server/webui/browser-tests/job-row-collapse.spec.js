const {test, expect} = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const repositoryRoot = path.resolve(__dirname, '../../../..');
const scriptDirectory = path.join(repositoryRoot, 'internal/server/webui/assets/js');
const cssDirectory = path.join(repositoryRoot, 'internal/server/webui/assets/css');

const controls = {
  controls: {
    viewport: {compactMaximumWidth: 760, condensedDisclosureMaximumWidth: 560},
    button: {
      iconPosition: 'leading', minimumHeight: {web: 44, native: 44}, paddingX: {web: 12, native: 12},
      paddingY: {web: 8, native: 8}, iconSize: {web: 19, native: 19}, iconGap: {web: 8, native: 8},
      iconOnlySize: {web: 34, native: 34},
    },
    badge: {paddingX: 9, paddingY: 4, tintOpacity: 0.12, borderOpacity: 0.55},
    input: {minimumHeight: {web: 44, native: 44}, paddingX: {web: 12, native: 12}, paddingY: {web: 9, native: 8}, placeholderColor: '#757575'},
    select: {
      chevronPosition: 'trailing', chevronSize: 16, chevronGap: 6, minimumHeight: 44, menuPadding: 4,
      menuItemGap: 2, optionGap: 6, optionPaddingX: 8, optionPaddingY: 6, optionMinimumHeight: 28,
      selectionIndicatorWidth: 16, viewportInset: 8, menuGap: 4, menuMinimumWidth: 120,
    },
    disclosure: {chevronPosition: 'trailing', chevronSize: 20, chevronGap: 8},
    progress: {tintOpacity: 0.18},
  },
};

const frontScreen = {
  apiVersion: 'ciwi.ui/v1', kind: 'Screen', metadata: {name: 'front-page'},
  screen: {dataSources: [{name: 'frontPage'}], root: {component: 'page', children: [{
    component: 'column', children: [
      {
        component: 'list', repeat: {source: 'frontPage.queued', as: 'job', key: 'job.id'}, children: [{
          component: 'row', id: 'queued-row', style: {role: 'queued-execution-job-row'},
          actions: [{on: 'activate', command: 'navigate', arguments: {route: '/jobs/{{job.id}}'}}],
          children: [
            {component: 'text', text: {binding: 'job.label'}, style: {role: 'link'}},
            {component: 'text', id: 'queued-status', text: {binding: 'job.status'}},
            {component: 'text', text: {binding: 'job.pipeline'}},
            {component: 'text', text: {binding: 'job.build'}},
            {component: 'text', text: {binding: 'job.agent'}},
            {component: 'text', text: {binding: 'job.created'}},
            {component: 'text', text: {binding: 'job.reason'}},
            {component: 'button', id: 'queued-cancel', text: {literal: 'Cancel'}, actions: [{
              on: 'activate', command: 'cancel-execution', arguments: {jobExecutionId: '{{job.id}}'},
            }]},
          ],
        }],
      },
      {
        component: 'list', repeat: {source: 'frontPage.history', as: 'job', key: 'job.id'}, children: [{
          component: 'row', id: 'history-row', style: {role: 'history-execution-job-row'},
          actions: [{on: 'activate', command: 'navigate', arguments: {route: '/jobs/{{job.id}}'}}],
          children: [
            {component: 'text', text: {binding: 'job.label'}, style: {role: 'link'}},
            {component: 'text', text: {binding: 'job.status'}},
            {component: 'text', text: {binding: 'job.pipeline'}},
            {component: 'text', text: {binding: 'job.build'}},
            {component: 'text', text: {binding: 'job.agent'}},
            {component: 'text', text: {binding: 'job.created'}},
            {component: 'text', id: 'history-duration', text: {binding: 'job.duration'}},
          ],
        }],
      },
    ],
  }] }},
};

const detailsScreen = {
  apiVersion: 'ciwi.ui/v1', kind: 'Screen', metadata: {name: 'job-details'},
  screen: {dataSources: [{name: 'jobDetails'}], root: {component: 'page', children: [
    {component: 'text', id: 'opened-job', text: {binding: 'jobDetails.id'}},
  ]}},
};

const outputScreen = {
  apiVersion: 'ciwi.ui/v1', kind: 'Screen', metadata: {name: 'output-test'},
  screen: {dataSources: [{name: 'jobDetails'}], root: {component: 'page', children: [
    {component: 'spacer', layout: {minHeight: '520'}},
    {
      component: 'scroller', id: 'job-output-groups', layout: {direction: 'vertical', gap: 'small', maxHeight: '660'},
      repeat: {source: 'jobDetails.output_groups', as: 'outputGroup', key: 'outputGroup.id'},
      children: [{
        component: 'disclosure', text: {binding: 'outputGroup.title'}, style: {role: 'output-group'}, progress: {binding: 'outputGroup.progress'},
        layout: {direction: 'vertical', gap: '0', padding: 'section-padding'},
        disclosure: {defaultExpandedBinding: 'outputGroup.default_expanded', stateKey: '{{outputGroup.state_key}}'},
        children: [
          {
            component: 'button', text: {literal: 'Collapse'}, icon: 'arrow-up', style: {role: 'floating-collapse'},
            actions: [{on: 'activate', command: 'set-disclosures', arguments: {prefix: '{{outputGroup.state_key}}', expanded: 'false'}}],
          },
          {component: 'text', id: 'long-output', text: {binding: 'outputGroup.output'}, style: {role: 'output-code'}},
        ],
      }],
    },
    {component: 'spacer', layout: {minHeight: '800'}},
  ] }},
};

async function serveAssets(route) {
  const url = new URL(route.request().url());
  if (url.pathname.startsWith('/ui/') && url.pathname.endsWith('.js')) {
    await route.fulfill({contentType: 'application/javascript', body: fs.readFileSync(path.join(scriptDirectory, path.basename(url.pathname)), 'utf8')});
    return true;
  }
  if (url.pathname === '/ui/declarative.css' || url.pathname === '/ui/chrome.css') {
    await route.fulfill({contentType: 'text/css', body: fs.readFileSync(path.join(cssDirectory, path.basename(url.pathname)), 'utf8')});
    return true;
  }
  if (url.pathname === '/ui/contracts/themes.json') {
    await route.fulfill({json: []});
    return true;
  }
  if (url.pathname === '/ui/contracts/controls.json') {
    await route.fulfill({json: controls});
    return true;
  }
  if (url.pathname === '/ui/contracts/actions.json') {
    await route.fulfill({json: {actions: [
      {command: 'remove-execution', class: 'mutation', scope: 'execution:{{jobExecutionId}}', pending: 'Removing execution…'},
      {command: 'cancel-execution', class: 'mutation', scope: 'execution:{{jobExecutionId}}', pending: 'Cancelling execution…'},
    ]}});
    return true;
  }
  if (url.pathname === '/ui/icons.svg') {
    await route.fulfill({status: 204, body: ''});
    return true;
  }
  return false;
}

function documentHTML() {
  return `<!doctype html><html><head><link rel="stylesheet" href="/ui/chrome.css"><link rel="stylesheet" href="/ui/declarative.css">
    <style>:root { --ciwi-section-padding: 14px; --ciwi-space-small: 8px; --line: #334155; }</style></head>
    <body><div id="declarativeRoot"></div><script>
      window.ciwiUIResourceURL = value => value;
      window.alert = () => {};
      window.confirm = () => true;
      window.EventSource = class EventSource { addEventListener() {} close() {} };
    </script><script src="/ui/theme.js"></script><script src="/ui/actions.js"></script>
    <script src="/ui/view-state.js"></script><script src="/ui/heartbeat.js"></script>
    <script src="/ui/change-refresh.js"></script><script src="/ui/view-bindings.js"></script>
    <script src="/ui/select-control.js"></script><script src="/ui/graph-view.js"></script>
    <script src="/ui/tree-view.js"></script><script src="/ui/dom-reconciler.js"></script><script src="/ui/declarative.js"></script></body></html>`;
}

async function installJobRowFixture(page) {
  const cancelled = [];
  const frontView = {
    queued: [{id: 'queued-1', label: 'Queued job', status: 'running', pipeline: 'release', build: 'v1', agent: 'ios', created: 'now', reason: 'manual'}],
    history: [{id: 'history-1', label: 'History job', status: 'succeeded', pipeline: 'test', build: 'v0', agent: 'mac', created: 'earlier', duration: '1m'}],
  };
  await page.route('http://ciwi-rows.test/**', async route => {
    const url = new URL(route.request().url());
    if (await serveAssets(route)) return;
    if (url.pathname === '/' || url.pathname.startsWith('/jobs/')) {
      await route.fulfill({contentType: 'text/html', body: documentHTML()});
    } else if (url.pathname === '/ui/contracts/routes.json') {
      await route.fulfill({json: {routes: [
        {name: 'front-page', pattern: '/', screen: 'front-page', bindingRoot: 'frontPage', platforms: ['web']},
        {name: 'job-details', pattern: '/jobs/{jobId}', screen: 'job-details', bindingRoot: 'jobDetails', platforms: ['web']},
      ]}});
    } else if (url.pathname === '/ui/contracts/screens/front-page.json') {
      await route.fulfill({json: frontScreen});
    } else if (url.pathname === '/ui/contracts/screens/job-details.json') {
      await route.fulfill({json: detailsScreen});
    } else if (url.pathname === '/api/v1/views/front-page') {
      await route.fulfill({json: frontView});
    } else if (url.pathname.startsWith('/api/v1/views/jobs/')) {
      await route.fulfill({json: {id: path.basename(url.pathname), status: 'succeeded', output_groups: [], timeline: []}});
    } else if (url.pathname === '/api/v1/jobs/queued-1/cancel') {
      cancelled.push('queued-1');
      await route.fulfill({json: {}});
    } else {
      await route.fulfill({status: 404, body: 'not found'});
    }
  });
  await page.goto('http://ciwi-rows.test/');
  await expect(page.locator('#queued-row')).toBeVisible();
  return {cancelled};
}

async function installOutputFixture(page) {
  const output = Array.from({length: 180}, (_, index) => `line ${index}: long job output`).join('\n');
  await page.route('http://ciwi-output.test/**', async route => {
    const url = new URL(route.request().url());
    if (await serveAssets(route)) return;
    if (url.pathname === '/') {
      await route.fulfill({contentType: 'text/html', body: documentHTML()});
    } else if (url.pathname === '/ui/contracts/routes.json') {
      await route.fulfill({json: {routes: [{name: 'output-test', pattern: '/', screen: 'output-test', bindingRoot: 'jobDetails', platforms: ['web']}]}});
    } else if (url.pathname === '/ui/contracts/screens/output-test.json') {
      await route.fulfill({json: outputScreen});
    } else if (url.pathname === '/api/v1/views/front-page') {
      await route.fulfill({json: {output_groups: [{
        id: 'step-1', title: 'Long build step', state_key: 'job-output:step-1', default_expanded: true, output,
        progress: {state: 'complete', fraction: 1},
      }]}});
    } else {
      await route.fulfill({status: 404, body: 'not found'});
    }
  });
  await page.goto('http://ciwi-output.test/');
  await expect(page.locator('.dsl-floating-collapse')).toBeVisible();
}

test('queued and history job rows navigate from passive cells while nested actions retain ownership', async ({page}) => {
  const fixture = await installJobRowFixture(page);
  await page.locator('#queued-cancel').click();
  await expect.poll(() => fixture.cancelled).toEqual(['queued-1']);
  await expect(page).toHaveURL('http://ciwi-rows.test/');

  await page.locator('#queued-status').click();
  await expect(page).toHaveURL('http://ciwi-rows.test/jobs/queued-1');
  await expect(page.locator('#opened-job')).toHaveText('queued-1');

  await page.goto('http://ciwi-rows.test/');
  await page.locator('#history-row').focus();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL('http://ciwi-rows.test/jobs/history-1');
  await expect(page.locator('#opened-job')).toHaveText('history-1');
});

test('constrained job action labels stay centered inside the web button', async ({page}) => {
  await page.setViewportSize({width: 1280, height: 720});
  await installJobRowFixture(page);
  const button = page.locator('#queued-cancel');

  for (const action of [
    {command: 'cancel-execution', label: 'Cancel', pending: 'Cancelling execution…'},
    {command: 'remove-execution', label: 'Remove', pending: 'Removing execution…'},
  ]) {
    await button.evaluate((element, value) => {
      element.querySelector('.dsl-button-label-current').textContent = value.label;
      window.ciwiReservePendingLabel(element, value.command);
    }, action);
    await expect(button.locator('.dsl-button-label')).toHaveAttribute('data-ciwi-reserved-label', action.pending);
    const geometry = await button.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      const label = element.querySelector('.dsl-button-label').getBoundingClientRect();
      const current = element.querySelector('.dsl-button-label-current').getBoundingClientRect();
      return {
        bounds: bounds.toJSON(), label: label.toJSON(), current: current.toJSON(),
        centerDelta: (current.left + current.right - label.left - label.right) / 2,
      };
    });
    expect(geometry.label.left).toBeGreaterThanOrEqual(geometry.bounds.left - 0.5);
    expect(geometry.label.right).toBeLessThanOrEqual(geometry.bounds.right + 0.5);
    expect(geometry.current.left).toBeGreaterThanOrEqual(geometry.bounds.left - 0.5);
    expect(geometry.current.right).toBeLessThanOrEqual(geometry.bounds.right + 0.5);
    expect(Math.abs(geometry.centerDelta)).toBeLessThanOrEqual(0.5);
  }

  await button.evaluate(element => {
    const icon = document.createElement('span');
    icon.className = 'dsl-icon';
    element.prepend(icon);
  });
  const iconGeometry = await button.evaluate(element => {
    const bounds = element.getBoundingClientRect();
    const label = element.querySelector('.dsl-button-label').getBoundingClientRect();
    const current = element.querySelector('.dsl-button-label-current').getBoundingClientRect();
    return {
      bounds: bounds.toJSON(), label: label.toJSON(),
      centerDelta: (current.left + current.right - label.left - label.right) / 2,
    };
  });
  expect(iconGeometry.label.left).toBeGreaterThanOrEqual(iconGeometry.bounds.left - 0.5);
  expect(iconGeometry.label.right).toBeLessThanOrEqual(iconGeometry.bounds.right + 0.5);
  expect(Math.abs(iconGeometry.centerDelta)).toBeLessThanOrEqual(0.5);
});

test('Collapse remains reachable at the end of a long output group', async ({page}) => {
  await page.setViewportSize({width: 390, height: 600});
  await installOutputFixture(page);
  const container = page.locator('#job-output-groups');
  await container.evaluate(element => { element.scrollTop = element.scrollHeight; });
  const geometry = await page.locator('.dsl-floating-collapse').evaluate(button => {
    const control = button.getBoundingClientRect();
    const viewport = document.getElementById('job-output-groups').getBoundingClientRect();
    return {control: control.toJSON(), viewport: viewport.toJSON()};
  });
  expect(geometry.control.top).toBeGreaterThanOrEqual(geometry.viewport.top + 7);
  expect(geometry.control.bottom).toBeLessThanOrEqual(geometry.viewport.bottom + 1);
  expect(geometry.control.right).toBeLessThanOrEqual(geometry.viewport.right - 7);

  await page.locator('.dsl-floating-collapse').click();
  await expect(page.locator('details.dsl-output-group')).not.toHaveAttribute('open', '');
  await expect(page.locator('.dsl-floating-collapse')).toBeHidden();
  await expect.poll(() => page.locator('details.dsl-output-group').evaluate(element => element.getBoundingClientRect().height)).toBe(50);
});

test('output scrolling chains to the page in both directions at its boundaries', async ({page}) => {
  await page.setViewportSize({width: 800, height: 500});
  await installOutputFixture(page);
  const container = page.locator('#job-output-groups');

  await container.evaluate(element => {
    element.scrollTop = 0;
    element.scrollIntoView({block: 'center'});
  });
  const initialPageScroll = await page.evaluate(() => window.scrollY);
  await container.hover();
  await page.mouse.wheel(0, 240);
  await expect.poll(() => container.evaluate(element => element.scrollTop)).toBeGreaterThan(0);
  expect(await page.evaluate(() => window.scrollY)).toBe(initialPageScroll);

  await container.evaluate(element => {
    element.scrollTop = element.scrollHeight;
    element.scrollIntoView({block: 'center'});
  });
  const beforeDownwardChain = await page.evaluate(() => window.scrollY);
  await container.hover();
  await page.mouse.wheel(0, 240);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(beforeDownwardChain);

  await container.evaluate(element => {
    element.scrollTop = 0;
    element.scrollIntoView({block: 'center'});
  });
  const beforeUpwardChain = await page.evaluate(() => window.scrollY);
  expect(beforeUpwardChain).toBeGreaterThan(0);
  await container.hover();
  await page.mouse.wheel(0, -240);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeLessThan(beforeUpwardChain);
});
