// Browser integration tests: deliberately excluded from `go test ./...`.
const {test, expect} = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const repositoryRoot = path.resolve(__dirname, '../../../..');
const scriptDirectory = path.join(repositoryRoot, 'internal/server/webui/assets/js');

const controls = {controls: {
  button: {iconPosition: 'leading', minimumHeight: {web: 44, native: 44}, paddingX: {web: 12, native: 12}, paddingY: {web: 8, native: 8}, iconSize: {web: 19, native: 19}, iconGap: {web: 8, native: 8}, iconOnlySize: {web: 34, native: 34}},
  badge: {paddingX: 9, paddingY: 4, tintOpacity: 0.12, borderOpacity: 0.55},
  input: {minimumHeight: {web: 44, native: 44}, paddingX: {web: 12, native: 12}, paddingY: {web: 9, native: 8}},
  select: {chevronPosition: 'trailing', chevronSize: 16, chevronGap: 6, minimumHeight: 44, menuPadding: 4, menuItemGap: 2, optionGap: 6, optionPaddingX: 8, optionPaddingY: 6, optionMinimumHeight: 28, selectionIndicatorWidth: 16, viewportInset: 8, menuGap: 4, menuMinimumWidth: 120},
  disclosure: {chevronPosition: 'trailing', chevronSize: 20, chevronGap: 8}, progress: {tintOpacity: 0.18},
}};

const frontScreen = {apiVersion: 'ciwi.ui/v1', kind: 'Screen', metadata: {name: 'front-page'}, screen: {
  root: {component: 'page', children: [{component: 'button', id: 'front-options', text: {literal: 'Options'}, actions: [{on: 'activate', command: 'navigate', arguments: {route: '/run-options/projects/9/chains/release'}}]}]},
}};

const projectScreen = {apiVersion: 'ciwi.ui/v1', kind: 'Screen', metadata: {name: 'project-details'}, screen: {
  root: {component: 'page', children: [
    {component: 'image', image: {binding: 'projectDetails.project.project_icon'}, visible: {binding: 'projectDetails.project.project_icon', empty: true, not: true}},
    {component: 'text', id: 'project-name', text: {binding: 'projectDetails.project.name'}},
    {component: 'button', id: 'project-options', text: {literal: 'Options'}, actions: [{on: 'activate', command: 'navigate', arguments: {route: '/run-options/projects/9/chains/release'}}]},
  ]},
}};

const runOptionsScreen = {apiVersion: 'ciwi.ui/v1', kind: 'Screen', metadata: {name: 'run-options'}, screen: {
  root: {component: 'page', children: [
    {component: 'button', id: 'run-options-back', text: {literal: 'Back'}, actions: [{on: 'activate', command: 'navigate-back', arguments: {fallbackRoute: '/projects/{{runOptions.project_id}}'}}]},
    {component: 'button', id: 'dry-run-chain', text: {literal: 'Dry Run Chain'}, actions: [{on: 'activate', command: 'run-chain', arguments: {
      projectId: '{{runOptions.project_id}}', chainId: '{{runOptions.chain_id}}', dryRun: 'true', backOnSuccess: 'true', fallbackRoute: '/projects/{{runOptions.project_id}}',
    }}]},
  ]},
}};

async function installRunOptionsFixture(page, {failRun = false} = {}) {
  const runBodies = [];
  const screens = {'front-page': frontScreen, 'project-details': projectScreen, 'run-options': runOptionsScreen};
  await page.route('http://ciwi-navigation.test/**', async route => {
    const url = new URL(route.request().url());
    if (['/', '/projects/9', '/run-options/projects/9/chains/release'].includes(url.pathname)) {
      await route.fulfill({contentType: 'text/html', body: `<!doctype html><html><body><div id="declarativeRoot"></div>
        <script>window.ciwiUIResourceURL = path => path; window.alerts = []; window.alert = value => window.alerts.push(String(value));
        window.EventSource = class EventSource { addEventListener() {} close() {} };</script>
        <script src="/ui/theme.js"></script><script src="/ui/actions.js"></script><script src="/ui/notices.js"></script>
        <script src="/ui/view-state.js"></script><script src="/ui/heartbeat.js"></script><script src="/ui/change-refresh.js"></script><script src="/ui/declarative.js"></script>
      </body></html>`});
      return;
    }
    if (url.pathname.startsWith('/ui/') && url.pathname.endsWith('.js')) {
      await route.fulfill({contentType: 'application/javascript', body: fs.readFileSync(path.join(scriptDirectory, path.basename(url.pathname)), 'utf8')});
      return;
    }
    if (url.pathname === '/ui/contracts/routes.json') {
      await route.fulfill({json: {routes: [
        {name: 'front-page', pattern: '/', screen: 'front-page', bindingRoot: 'frontPage', platforms: ['web']},
        {name: 'project-details', pattern: '/projects/{projectId}', screen: 'project-details', bindingRoot: 'projectDetails', platforms: ['web']},
        {name: 'chain-run-options', pattern: '/run-options/projects/{projectId}/chains/{chainId}', screen: 'run-options', bindingRoot: 'runOptions', platforms: ['web']},
      ]}});
      return;
    }
    if (url.pathname.startsWith('/ui/contracts/screens/')) {
      const name = path.basename(url.pathname, '.json');
      await route.fulfill({json: screens[name]});
      return;
    }
    if (url.pathname === '/ui/contracts/themes.json') return route.fulfill({json: []});
    if (url.pathname === '/ui/contracts/controls.json') return route.fulfill({json: controls});
    if (url.pathname === '/ui/contracts/actions.json') return route.fulfill({json: {actions: [
      {command: 'navigate', class: 'local'}, {command: 'navigate-back', class: 'local'},
      {command: 'run-chain', class: 'mutation', scope: 'chain:{{projectId}}:{{chainId}}', refreshOnSuccess: true},
    ]}});
    if (url.pathname === '/api/v1/views/front-page') return route.fulfill({json: {}});
    if (url.pathname === '/api/v1/views/projects/9') return route.fulfill({json: {project: {id: 9, name: 'Project Nine'}, pipelines: [], structure_filters: []}});
    if (url.pathname === '/api/v1/views/run-options/projects/9/chains/release') return route.fulfill({json: {
      project_id: 9, chain_id: 'release', target_kind: 'chain', target_label: 'Release', supports_dry_run: true,
      source_repo: '', source_refs: [], eligible_agents: [], selected_source_ref: '', selected_agent_id: '', pending_jobs: 1,
    }});
    if (url.pathname === '/api/v1/projects/9/pipeline-chains/release/run') {
      runBodies.push(JSON.parse(route.request().postData() || '{}'));
      if (failRun) return route.fulfill({status: 409, body: 'queue conflict'});
      return route.fulfill({json: {notice: {message: 'Queued dry run', action_label: 'Show queued jobs', route: '/', section: 'queued-executions'}}});
    }
    if (url.pathname === '/ui/icons.svg') return route.fulfill({status: 204, body: ''});
    await route.fulfill({status: 404, body: 'not found'});
  });
  return {runBodies};
}

test('Run Options Back and successful dry run return to the front-page origin', async ({page}) => {
  const fixture = await installRunOptionsFixture(page);
  await page.goto('http://ciwi-navigation.test/');
  await page.locator('#front-options').click();
  await expect(page).toHaveURL(/\/run-options\/projects\/9\/chains\/release$/);
  await page.locator('#run-options-back').click();
  await expect(page).toHaveURL('http://ciwi-navigation.test/');

  await page.locator('#front-options').click();
  await page.locator('#dry-run-chain').click();
  await expect(page).toHaveURL('http://ciwi-navigation.test/');
  expect(fixture.runBodies).toEqual([{pipeline_job_id: '', dry_run: true, source_ref: '', agent_id: '', execution_mode: ''}]);
  await expect(page.locator('.ciwi-snackbar-message')).toHaveText('Queued dry run');
  expect(await page.evaluate(() => window.alerts)).toEqual([]);
});

test('Project Details is the origin and remains binding-complete without an icon field', async ({page}) => {
  await installRunOptionsFixture(page);
  await page.goto('http://ciwi-navigation.test/projects/9');
  await expect(page.locator('#project-name')).toHaveText('Project Nine');
  await page.locator('#project-options').click();
  await page.locator('#run-options-back').click();
  await expect(page).toHaveURL('http://ciwi-navigation.test/projects/9');
  await expect(page.locator('#project-name')).toHaveText('Project Nine');
  expect(await page.evaluate(() => window.alerts)).toEqual([]);
});

test('A direct Run Options link falls back to Project Details and failures stay put', async ({page}) => {
  await installRunOptionsFixture(page, {failRun: true});
  await page.goto('http://ciwi-navigation.test/run-options/projects/9/chains/release');
  await page.locator('#dry-run-chain').click();
  await expect(page).toHaveURL(/\/run-options\/projects\/9\/chains\/release$/);
  await expect.poll(() => page.evaluate(() => window.alerts.join('\n'))).toContain('queue conflict');
  await page.locator('#run-options-back').click();
  await expect(page).toHaveURL('http://ciwi-navigation.test/projects/9');
  await expect(page.locator('#project-name')).toHaveText('Project Nine');
});
