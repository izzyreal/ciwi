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
      iconOnlySize: {web: 34, native: 34}, selectedTintOpacity: 0.24,
    },
    badge: {paddingX: 9, paddingY: 4, tintOpacity: 0.12, borderOpacity: 0.55},
    input: {minimumHeight: {web: 44, native: 44}, paddingX: {web: 12, native: 12}, paddingY: {web: 9, native: 8}, placeholderColor: '#757575'},
    select: {
      chevronPosition: 'trailing', chevronSize: 16, chevronGap: 6, minimumHeight: 44, menuPadding: 4,
      menuItemGap: 2, optionGap: 6, optionPaddingX: 8, optionPaddingY: 6, optionMinimumHeight: 28,
      selectionIndicatorWidth: 16, viewportInset: 8, menuGap: 4, menuMinimumWidth: 120,
    },
    logView: {minimumHeight: 48, maximumHeight: 420},
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
  screen: {dataSources: [{name: 'jobDetails', watchTopics: ['queue', 'history']}], root: {component: 'page', children: [
    {component: 'text', id: 'opened-job', text: {binding: 'jobDetails.id'}},
    {component: 'text', id: 'opened-status', text: {binding: 'jobDetails.status'}},
  ]}},
};

const outputScreen = {
  apiVersion: 'ciwi.ui/v1', kind: 'Screen', metadata: {name: 'output-test'},
  screen: {dataSources: [{name: 'jobDetails', watchTopics: ['history', 'job-output']}], root: {component: 'page', children: [
    {component: 'spacer', layout: {minHeight: '520'}},
    {
      component: 'row', layout: {direction: 'horizontal', gap: 'small'}, children: [
        {component: 'button', id: 'ordinary-output-action', text: {literal: 'Download'}, icon: 'download'},
        {
          component: 'button', id: 'job-output-tailing-toggle', text: {binding: 'jobDetails.tailing_label'},
          icon: 'arrow-bar-to-down', style: {role: 'tailing-toggle', toneBinding: 'jobDetails.tailing_tone'},
          actions: [{on: 'activate', command: 'toggle-output-tailing'}],
        },
      ],
    },
    {
      component: 'list', id: 'job-output-timeline-test', layout: {direction: 'horizontal', gap: 'small'},
      repeat: {source: 'jobDetails.timeline', as: 'item', key: 'item.id'}, children: [{
        component: 'card', style: {role: 'output-selector', selectedBinding: 'item.selected'},
        actions: [{on: 'activate', command: 'select-timeline-item', arguments: {id: '{{item.id}}'}}],
        children: [{component: 'text', text: {binding: 'item.title'}}],
      }],
    },
    {
      component: 'list', id: 'job-output-selectors', layout: {direction: 'vertical', gap: 'small'},
      repeat: {source: 'jobDetails.output_groups', as: 'outputGroup', key: 'outputGroup.id'},
      children: [{
        component: 'card', style: {role: 'output-selector', selectedBinding: 'outputGroup.selected'},
        actions: [{on: 'activate', command: 'select-timeline-item', arguments: {id: '{{outputGroup.id}}'}}],
        children: [
          {component: 'text', text: {binding: 'outputGroup.title'}},
          {component: 'text', text: {binding: 'outputGroup.status_label'}},
        ],
      }],
    },
    {
      component: 'section', id: 'job-output-viewer', style: {role: 'output-viewer'},
      layout: {direction: 'vertical', gap: '0', minHeight: '660', maxHeight: '660'}, children: [
        {component: 'text', id: 'selected-output-title', text: {binding: 'jobDetails.selected_output_group.title'}, style: {role: 'output-viewer-header'}},
        {
          component: 'scroller', id: 'job-output-document', layout: {direction: 'vertical', gap: 'small'},
          repeat: {source: 'jobDetails.selected_output_groups', as: 'outputGroup', key: 'outputGroup.id'},
          children: [
            {component: 'text', id: 'long-output', text: {binding: 'outputGroup.output'}, style: {role: 'output-code'}},
            {
              component: 'log-view', visible: {binding: 'outputGroup.interactive_log_available'},
              logView: {jobExecutionId: 'jobDetails.id', itemId: 'outputGroup.id'}, style: {role: 'output-code'},
            },
          ],
        },
      ],
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
    <style>:root { --ciwi-section-padding:14px; --ciwi-space-small:8px; --line:#334155; --surface:#10251c; --accent:#a3e635; --ok:#52e2a2; }</style></head>
    <body><div id="declarativeRoot"></div><script>
      window.ciwiUIResourceURL = value => value;
      window.alert = () => {};
      window.confirm = () => true;
      window.ciwiEventSources = [];
      window.EventSource = class EventSource {
        constructor(url) { this.url = String(url || ''); this.listeners = {}; window.ciwiEventSources.push(this); }
        addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener); }
        emit(type, data) { (this.listeners[type] || []).forEach(listener => listener({data: JSON.stringify(data)})); }
        close() {}
      };
    </script><script src="/ui/theme.js"></script><script src="/ui/actions.js"></script>
    <script src="/ui/view-state.js"></script><script src="/ui/heartbeat.js"></script>
    <script src="/ui/change-refresh.js"></script><script src="/ui/view-bindings.js"></script>
    <script src="/ui/select-control.js"></script><script src="/ui/graph-view.js"></script>
    <script src="/ui/tree-view.js"></script><script src="/ui/dom-reconciler.js"></script><script src="/ui/declarative.js"></script></body></html>`;
}

async function installJobRowFixture(page) {
  const cancelled = [];
  const jobDetailRequests = [];
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
      const id = path.basename(url.pathname);
      jobDetailRequests.push(id);
      const requestsForJob = jobDetailRequests.filter(value => value === id).length;
      await route.fulfill({json: {id, status: requestsForJob === 1 ? 'running' : 'succeeded', output_groups: [], timeline: []}});
    } else if (url.pathname === '/api/v1/jobs/queued-1/cancel') {
      cancelled.push('queued-1');
      await route.fulfill({json: {}});
    } else {
      await route.fulfill({status: 404, body: 'not found'});
    }
  });
  await page.goto('http://ciwi-rows.test/');
  await expect(page.locator('#queued-row')).toBeVisible();
  return {cancelled, jobDetailRequests};
}

async function installOutputFixture(page) {
  const output = Array.from({length: 180}, (_, index) => `line ${index}: long job output`).join('\n');
  const groups = [
    {
      id: 'step-1', title: 'Compile', status: 'succeeded', status_label: 'Succeeded', reached: true, selected: true,
      state_key: 'job-output:step-1', output, available: true, interactive_log_available: false,
      progress: {state: 'complete', fraction: 1},
    },
    {
      id: 'step-2', title: 'Package', status: 'succeeded', status_label: 'Succeeded', reached: true, selected: false,
      state_key: 'job-output:step-2', output: `package selected\n${output}`, available: true,
      interactive_log_available: false, progress: {state: 'complete', fraction: 1},
    },
  ];
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
      await route.fulfill({json: {
        id: 'job-output', ready: true, interactive_log_available: false, output_follow_latest: false,
        output_tailing: false, tailing_label: 'Tailing: Off', tailing_tone: 'accent', output_groups: groups,
        timeline: groups.map(group => ({id: group.id, title: group.title, status: group.status, selected: group.selected})),
        selected_timeline_item: groups[0], selected_output_group: groups[0], selected_output_groups: [groups[0]],
      }});
    } else {
      await route.fulfill({status: 404, body: 'not found'});
    }
  });
  await page.goto('http://ciwi-output.test/');
  await expect(page.locator('#job-output-viewer')).toBeVisible();
}

async function installInteractiveOutputFixture(page) {
  const initial = Array.from({length: 180}, (_, index) => `initial line ${index}\n`).join('');
  const logPageRequests = [];
  const phase = {
    id: 'phase-1', title: 'Check out source', kind: 'phase', status: 'succeeded', status_label: 'Succeeded', reached: true,
    selected: false, output: '', available: true, interactive_log_available: true,
    progress: {state: 'complete', fraction: 1},
  };
  const group = {
    id: 'step-1', title: 'Build image', kind: 'step', status: 'running', status_label: 'In progress', reached: true,
    selected: true, output: '', available: true, interactive_log_available: true,
    progress: {state: 'active', fraction: 0.5},
  };
  const next = {
    id: 'step-2', title: 'Publish image', kind: 'step', status: 'pending', status_label: 'Pending', reached: false,
    selected: false, output: '', available: true, interactive_log_available: true,
    progress: {state: 'none', fraction: 0},
  };
  await page.route('http://ciwi-live-output.test/**', async route => {
    const url = new URL(route.request().url());
    if (await serveAssets(route)) return;
    if (url.pathname === '/jobs/job-1') {
      await route.fulfill({contentType: 'text/html', body: documentHTML()});
    } else if (url.pathname === '/ui/contracts/routes.json') {
      await route.fulfill({json: {routes: [{name: 'job-details', pattern: '/jobs/{jobId}', screen: 'output-test', bindingRoot: 'jobDetails', platforms: ['web']}]}});
    } else if (url.pathname === '/ui/contracts/screens/output-test.json') {
      await route.fulfill({json: outputScreen});
    } else if (url.pathname === '/api/v1/views/jobs/job-1') {
      await route.fulfill({json: {
        id: 'job-1', status: 'running', interactive_log_available: true,
        output_groups: [phase, group, next], timeline: [phase, group, next],
      }});
    } else if (url.pathname === '/api/v1/views/jobs/job-1/log/page') {
      const mode = url.searchParams.get('mode') || 'head';
      const itemID = url.searchParams.get('item_id') || '';
      logPageRequests.push({mode, itemID});
      const chunks = mode === 'after'
        ? [{id: 2, text: 'live appended line\n', byte_count: 19}]
        : [{id: 1, text: initial, byte_count: initial.length}];
      await route.fulfill({json: {
        job_execution_id: 'job-1', item_id: url.searchParams.get('item_id') || '', chunks,
        has_before: false, has_after: false, terminal: false,
      }});
    } else {
      await route.fulfill({status: 404, body: 'not found'});
    }
  });
  await page.goto('http://ciwi-live-output.test/jobs/job-1');
  await expect(page.locator('.dsl-log-view')).toContainText('initial line 179');
  return {logPageRequests, group, next};
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

test('combined lifecycle and output changes refresh structured job progress', async ({page}) => {
  const fixture = await installJobRowFixture(page);
  await page.locator('#queued-status').click();
  await expect(page.locator('#opened-status')).toHaveText('running');
  await expect.poll(() => fixture.jobDetailRequests.filter(id => id === 'queued-1').length).toBe(1);

  const dispatchChange = topics => page.evaluate(value => {
    const source = window.ciwiEventSources.find(candidate => typeof candidate.onmessage === 'function');
    source.onmessage({data: JSON.stringify({topics: value, job_execution_ids: ['queued-1']})});
  }, topics);
  await dispatchChange(['job-output']);
  await page.waitForTimeout(250);
  expect(fixture.jobDetailRequests.filter(id => id === 'queued-1')).toHaveLength(1);

  await dispatchChange(['history', 'job-output']);
  await expect(page.locator('#opened-status')).toHaveText('succeeded');
  await expect.poll(() => fixture.jobDetailRequests.filter(id => id === 'queued-1').length).toBe(2);
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

test('output selectors stay collapsed in page flow and drive the fixed-height viewer', async ({page}) => {
  await page.setViewportSize({width: 390, height: 600});
  await installOutputFixture(page);
  const timeline = page.locator('#job-output-timeline-test .dsl-output-selector');
  const selectors = page.locator('#job-output-selectors .dsl-output-selector');
  await expect(timeline).toHaveCount(2);
  await expect(selectors).toHaveCount(2);
  await expect(page.locator('details')).toHaveCount(0);
  await expect(page.locator('#job-output-groups')).toHaveCount(0);
  await expect(timeline.nth(0)).toHaveAttribute('aria-pressed', 'true');
  await expect(selectors.nth(0)).toHaveAttribute('aria-pressed', 'true');
  await expect(selectors.nth(1)).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('#selected-output-title')).toHaveText('Compile');
  await expect.poll(() => page.locator('#job-output-viewer').evaluate(element => element.getBoundingClientRect().height)).toBe(420);

  await timeline.nth(1).click();
  await expect(timeline.nth(0)).toHaveAttribute('aria-pressed', 'false');
  await expect(timeline.nth(1)).toHaveAttribute('aria-pressed', 'true');
  await expect(selectors.nth(0)).toHaveAttribute('aria-pressed', 'false');
  await expect(selectors.nth(1)).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#selected-output-title')).toHaveText('Package');
  await expect(page.locator('#long-output')).toContainText('package selected');
  await expect(page.locator('#job-output-tailing-toggle')).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('#job-output-viewer')).toBeInViewport();

  await selectors.nth(0).click();
  await expect(timeline.nth(0)).toHaveAttribute('aria-pressed', 'true');
  await expect(selectors.nth(0)).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#selected-output-title')).toHaveText('Compile');
});

test('tailing toggle looks ordinary off and clearly selected on', async ({page}) => {
  await page.setViewportSize({width: 800, height: 700});
  await installOutputFixture(page);
  const toggle = page.locator('#job-output-tailing-toggle');
  const ordinary = page.locator('#ordinary-output-action');
  const appearance = locator => locator.evaluate(element => {
    const style = getComputedStyle(element);
    return {background: style.backgroundColor, border: style.borderColor, color: style.color};
  });

  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await expect(toggle).toHaveAttribute('aria-label', 'Tailing: Off');
  const off = await appearance(toggle);
  expect(off).toEqual(await appearance(ordinary));

  await toggle.click();
  await page.mouse.move(799, 699);
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(toggle).toHaveAttribute('aria-label', 'Tailing: On');
  await expect(page.locator('#selected-output-title')).toHaveText('Compile');
  await expect(page.locator('#job-output-selectors .dsl-output-selector').nth(0)).toHaveAttribute('aria-pressed', 'true');
  const on = await appearance(toggle);
  expect(on.background).not.toBe(off.background);
  expect(on.border).not.toBe(off.border);

  await toggle.click();
  await page.mouse.move(799, 699);
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  expect(await appearance(toggle)).toEqual(off);
});

test('indexed output tailing follows live pages in the dedicated viewer', async ({page}) => {
  await page.setViewportSize({width: 800, height: 500});
  const fixture = await installInteractiveOutputFixture(page);
  const documentScroller = page.locator('#job-output-document');
  const toggle = page.locator('#job-output-tailing-toggle');
  const distanceFromEnd = () => documentScroller.evaluate(element => element.scrollHeight - element.clientHeight - element.scrollTop);

  await page.locator('#job-output-selectors .dsl-output-selector').nth(1).click();
  await expect(page.locator('#selected-output-title')).toHaveText('Build image');
  await toggle.click();
  await expect.poll(() => documentScroller.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true);
  await expect.poll(distanceFromEnd).toBeLessThanOrEqual(3);
  await documentScroller.evaluate(element => {
    element.scrollTop = 0;
    element.dispatchEvent(new Event('scroll'));
  });
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');

  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect.poll(distanceFromEnd).toBeLessThanOrEqual(3);
  await page.evaluate(() => {
    const source = window.ciwiEventSources.find(candidate => candidate.url.endsWith('/log/stream'));
    source.emit('change', {terminal: false, streams: [{item_id: 'step-1', last_chunk_id: 2}]});
  });
  await expect(page.locator('.dsl-log-view')).toContainText('live appended line');
  await expect.poll(() => fixture.logPageRequests.filter(request => request.mode === 'after').length).toBe(1);
  await expect.poll(distanceFromEnd).toBeLessThanOrEqual(3);
});

test('tailing a selected item preserves it instead of jumping to the global tail', async ({page}) => {
  const fixture = await installInteractiveOutputFixture(page);
  const selectors = page.locator('#job-output-selectors .dsl-output-selector');
  await expect(page.locator('#selected-output-title')).toHaveText('Build image');

  await selectors.nth(0).click();
  await expect(page.locator('#selected-output-title')).toHaveText('Check out source');
  await page.locator('#job-output-tailing-toggle').click();

  await expect(page.locator('#selected-output-title')).toHaveText('Check out source');
  await expect(selectors.nth(0)).toHaveAttribute('aria-pressed', 'true');
  await expect.poll(() => fixture.logPageRequests.some(request => request.itemID === 'phase-1' && request.mode === 'tail')).toBe(true);

  fixture.group.status = 'succeeded';
  fixture.group.status_label = 'Succeeded';
  fixture.next.status = 'running';
  fixture.next.status_label = 'In progress';
  fixture.next.reached = true;
  await page.evaluate(() => {
    const source = window.ciwiEventSources.find(candidate => candidate.url.endsWith('/ui/changes'));
    source.onmessage({data: JSON.stringify({topics: ['job-output'], job_execution_ids: ['job-1']})});
  });

  await expect(page.locator('#selected-output-title')).toHaveText('Publish image');
  await expect(page.locator('#job-output-tailing-toggle')).toHaveAttribute('aria-pressed', 'true');
});

test('output scrolling chains to the page in both directions at its boundaries', async ({page}) => {
  await page.setViewportSize({width: 800, height: 500});
  await installOutputFixture(page);
  const container = page.locator('#job-output-document');

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
