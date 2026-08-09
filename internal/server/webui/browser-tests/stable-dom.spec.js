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
            visible: {binding: 'probe.select_visible'},
            select: {options: 'probe.options', value: 'probe.selected', as: 'option', optionValue: 'option.value', optionLabel: 'option.label'},
          },
          {
            component: 'disclosure', id: 'probe-disclosure', text: {literal: 'Disclosure'},
            disclosure: {defaultExpanded: true, stateKey: 'probe-disclosure'},
            children: [{component: 'text', text: {binding: 'probe.detail'}}],
          },
          {
            component: 'graph-view', id: 'probe-graph', text: {literal: 'Graph'},
            visible: {binding: 'probe.graph_visible'},
            graphView: {
              stateKey: 'probe-graph', defaultMode: 'graph', nodes: 'probe.graph_nodes', as: 'graphNode',
              nodeKey: 'graphNode.id', nodeLabel: {binding: 'graphNode.label'}, nodeMeta: {binding: 'graphNode.meta'},
              dependencies: 'graphNode.dependencies',
              details: [{component: 'text', text: {binding: 'graphNode.meta'}}],
            },
          },
          {
            component: 'tree-view', id: 'probe-tree', visible: {binding: 'probe.tree_visible'},
            treeView: {
              stateKey: 'probe-tree', nodes: 'probe.tree_nodes', as: 'treeNode', nodeKey: 'treeNode.key',
              nodeLabel: {binding: 'treeNode.label'}, children: 'treeNode.children', defaultExpanded: 'treeNode.default_expanded',
            },
          },
          {
            component: 'scroller', id: 'probe-output', visible: {binding: 'probe.output_visible'},
            children: [{component: 'text', id: 'probe-output-text', text: {binding: 'probe.output'}}],
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
    version: 'one', input: 'draft', detail: 'details', selected: 'a', select_visible: true, action_id: '1', action_visible: true,
    graph_visible: false, graph_nodes: [], tree_visible: false, tree_nodes: [], output_visible: false, output: '',
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
          #probe-graph .dsl-definition-graph-viewport { width: 180px; height: 110px; overflow: auto; }
          #probe-output { display: block; width: 220px; height: 40px; overflow: auto; white-space: pre; }
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

test('open custom select and its option identities survive a compatible refresh', async ({page}) => {
  await installFixture(page, [model(), model({
    version: 'two', detail: 'updated',
    options: [{value: 'a', label: 'Alpha updated'}, {value: 'b', label: 'Beta'}, {value: 'c', label: 'Gamma'}],
  })]);
  await page.locator('#probe-select').click();
  await expect(page.locator('.dsl-select-menu')).toBeVisible();
  await page.evaluate(() => {
    window.previousSelect = document.getElementById('probe-select');
    window.previousSelectMenu = document.querySelector('.dsl-select-menu');
    window.previousAlphaOption = document.querySelector('.dsl-select-option[data-value="a"]');
  });
  await page.locator('#probe-refresh').evaluate(element => element.click());
  await expect(page.locator('#probe-version')).toHaveText('two');
  await expect(page.locator('.dsl-select-menu')).toBeVisible();
  expect(await page.evaluate(() => window.previousSelect === document.getElementById('probe-select'))).toBe(true);
  expect(await page.evaluate(() => window.previousSelectMenu === document.querySelector('.dsl-select-menu'))).toBe(true);
  expect(await page.evaluate(() => window.previousAlphaOption === document.querySelector('.dsl-select-option[data-value="a"]'))).toBe(true);
  await expect(page.locator('#probe-select')).toHaveAttribute('aria-expanded', 'true');
  await expect(page.locator('#probe-select .dsl-select-label')).toHaveText('Alpha updated');
  await expect(page.locator('.dsl-select-option')).toHaveCount(3);
  await expect(page.locator('.dsl-select-option[data-value="c"]')).toHaveText('Gamma');
});

test('removing an open custom select disposes its portal', async ({page}) => {
  await installFixture(page, [model(), model({version: 'two', select_visible: false})]);
  await page.locator('#probe-select').click();
  await expect(page.locator('.dsl-select-menu')).toBeVisible();
  const oldTrigger = await page.locator('#probe-select').elementHandle();
  await page.locator('#probe-refresh').evaluate(element => element.click());
  await expect(page.locator('#probe-version')).toHaveText('two');
  await expect(page.locator('#probe-select')).toHaveCount(0);
  await expect(page.locator('.dsl-select-menu')).toHaveCount(0);
  expect(await page.evaluate(trigger => trigger.getAttribute('aria-expanded'), oldTrigger)).toBe('false');
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

test('tree branches retain identity and disclosure state across keyed updates', async ({page}) => {
  const folder = {key: 'folder', label: 'Folder', default_expanded: true, children: [{key: 'leaf', label: 'Leaf', children: []}]};
  await installFixture(page, [
    model({tree_visible: true, tree_nodes: [folder]}),
    model({version: 'two', tree_visible: true, tree_nodes: [
      {key: 'new', label: 'New root', children: []},
      {...folder, label: 'Folder updated'},
    ]}),
  ]);
  const branch = page.locator('#probe-tree [data-ciwi-component="tree-branch"]').first();
  await expect(branch).toHaveAttribute('open', '');
  await page.evaluate(() => { window.previousTreeBranch = document.querySelector('#probe-tree [data-ciwi-component="tree-branch"]'); });
  await refreshTo(page, 'two');
  expect(await page.evaluate(() => window.previousTreeBranch === document.querySelector('#probe-tree [data-ciwi-component="tree-branch"]'))).toBe(true);
  await expect(page.locator('#probe-tree [data-ciwi-component="tree-branch"]')).toHaveAttribute('open', '');
  await expect(page.locator('#probe-tree')).toContainText('Folder updated');
});

test('graph root keeps identity while zoom and viewport survive data changes', async ({page}) => {
  const graphNodes = [
    {id: 'a', label: 'A', meta: 'one', dependencies: []},
    {id: 'b', label: 'B', meta: 'two', dependencies: ['a']},
    {id: 'c', label: 'C', meta: 'three', dependencies: ['b']},
  ];
  await installFixture(page, [
    model({graph_visible: true, graph_nodes: graphNodes}),
    model({version: 'two', graph_visible: true, graph_nodes: [
      ...graphNodes.map(item => item.id === 'b' ? {...item, meta: 'updated'} : item),
      {id: 'd', label: 'D', meta: 'four', dependencies: ['c']},
    ]}),
  ]);
  await page.locator('.dsl-definition-graph-node', {hasText: 'B'}).click();
  await expect(page.locator('.dsl-definition-graph-node.selected')).toContainText('B');
  await page.getByRole('button', {name: 'Reset'}).click();
  await page.getByRole('button', {name: 'Zoom in'}).click();
  await expect(page.locator('.dsl-definition-graph-scale')).toHaveText('110%');
  await page.locator('.dsl-definition-graph-viewport').evaluate(viewport => {
    viewport.scrollLeft = 35;
    viewport.scrollTop = 12;
    viewport.dispatchEvent(new Event('scroll'));
    window.previousGraph = document.getElementById('probe-graph');
  });
  await refreshTo(page, 'two');
  expect(await page.evaluate(() => window.previousGraph === document.getElementById('probe-graph'))).toBe(true);
  await expect(page.locator('.dsl-definition-graph-scale')).toHaveText('110%');
  await expect.poll(() => page.locator('.dsl-definition-graph-viewport').evaluate(viewport => viewport.scrollLeft)).toBe(35);
  await expect(page.locator('.dsl-definition-graph-node.selected')).toContainText('B');
  await expect(page.locator('.dsl-definition-graph-details')).toContainText('updated');
  await expect(page.locator('#probe-graph')).toContainText('updated');
});

test('output scroller, text selection, and scroll position survive appended text', async ({page}) => {
  const initial = Array.from({length: 20}, (_, index) => 'line ' + String(index)).join('\n');
  await installFixture(page, [
    model({output_visible: true, output: initial}),
    model({version: 'two', output_visible: true, output: initial + '\nappended'}),
  ]);
  await page.locator('#probe-output').evaluate(scroller => {
    scroller.scrollTop = 30;
    const text = document.getElementById('probe-output-text').firstChild;
    const selection = window.getSelection();
    selection.setBaseAndExtent(text, 2, text, 5);
    window.previousOutputScroller = scroller;
    window.previousOutputText = text;
  });
  await refreshTo(page, 'two');
  expect(await page.evaluate(() => window.previousOutputScroller === document.getElementById('probe-output'))).toBe(true);
  expect(await page.evaluate(() => window.previousOutputText === document.getElementById('probe-output-text').firstChild)).toBe(true);
  expect(await page.locator('#probe-output').evaluate(scroller => scroller.scrollTop)).toBe(30);
  expect(await page.evaluate(() => {
    const selection = window.getSelection();
    return selection.anchorNode === window.previousOutputText && selection.focusNode === window.previousOutputText
      ? [selection.anchorOffset, selection.focusOffset]
      : null;
  })).toEqual([2, 5]);
});
