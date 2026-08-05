'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

const runnerSource = fs.readFileSync('assets/js/actions.js', 'utf8');

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolveValue, rejectValue) => {
    resolve = resolveValue;
    reject = rejectValue;
  });
  return { promise, resolve, reject };
}

function element() {
  const attributes = new Map();
  const classes = new Set();
  return {
    innerHTML: '<span>Run</span>',
    textContent: 'Run',
    disabled: false,
    classList: {
      add: value => classes.add(value),
      remove: value => classes.delete(value),
      contains: value => classes.has(value),
    },
    setAttribute: (key, value) => attributes.set(key, value),
    removeAttribute: key => attributes.delete(key),
    getAttribute: key => attributes.get(key),
  };
}

function harness(actions) {
  const events = [];
  const snackbars = [];
  const sandbox = {
    AbortController,
    CustomEvent: class CustomEvent {
      constructor(type, options) {
        this.type = type;
        this.detail = options.detail;
      }
    },
    Map,
    Math,
    Promise,
    Uint8Array,
    crypto: globalThis.crypto,
    fetch: async () => ({ ok: true, json: async () => ({ actions }) }),
    window: {
      dispatchEvent: event => events.push(event),
      showSnackbar: value => snackbars.push(value),
    },
  };
  sandbox.globalThis = sandbox;
  vm.runInNewContext(runnerSource, sandbox, { filename: 'actions.js' });
  return { window: sandbox.window, events, snackbars };
}

const mutation = { command: 'mutate', class: 'mutation', scope: 'resource:{{id}}', pending: 'Working…' };

test('mutation becomes busy immediately, coalesces duplicates, and restores its element', async () => {
  const runtime = harness([mutation]);
  const button = element();
  const result = deferred();
  let executions = 0;
  const execute = actionRuntime => {
    executions += 1;
    assert.match(actionRuntime.idempotencyKey, /^[0-9a-f-]{36}$/);
    return result.promise;
  };
  const first = runtime.window.ciwiRunAction('mutate', { id: 7 }, button, execute);
  const second = runtime.window.ciwiRunAction('mutate', { id: 7 }, button, execute);
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(executions, 1);
  assert.equal(button.disabled, true);
  assert.equal(button.getAttribute('aria-busy'), 'true');
  assert.equal(button.classList.contains('ciwi-action-pending'), true);
  assert.equal(button.textContent, 'Working…');
  result.resolve('done');
  assert.deepEqual(await Promise.all([first, second]), ['done', 'done']);
  assert.equal(button.disabled, false);
  assert.equal(button.getAttribute('aria-busy'), undefined);
  assert.equal(button.classList.contains('ciwi-action-pending'), false);
  assert.equal(button.innerHTML, '<span>Run</span>');
  assert.equal(runtime.window.ciwiActiveOperations().length, 0);
});

test('conflicting mutations are rejected and reported', async () => {
  const runtime = harness([
    mutation,
    { command: 'other-mutation', class: 'mutation', scope: 'resource:{{id}}', pending: 'Other work…' },
  ]);
  const active = deferred();
  const first = runtime.window.ciwiRunAction('mutate', { id: 7 }, element(), () => active.promise);
  await new Promise(resolve => setImmediate(resolve));
  await assert.rejects(
    runtime.window.ciwiRunAction('other-mutation', { id: 7 }, element(), async () => 'unexpected'),
    /Working/,
  );
  assert.equal(runtime.snackbars.length, 1);
  active.resolve('done');
  await first;
});

test('new query supersedes the previous query in the same scope', async () => {
  const runtime = harness([
    { command: 'query-a', class: 'query', scope: 'screen' },
    { command: 'query-b', class: 'query', scope: 'screen' },
  ]);
  let firstAborted = false;
  const first = runtime.window.ciwiRunAction('query-a', {}, element(), ({ signal }) => new Promise((_, reject) => {
    signal.addEventListener('abort', () => {
      firstAborted = true;
      reject(new Error('aborted'));
    });
  }));
  await new Promise(resolve => setImmediate(resolve));
  const second = runtime.window.ciwiRunAction('query-b', {}, element(), async () => 'fresh');
  assert.equal(await second, 'fresh');
  await assert.rejects(first, /aborted/);
  assert.equal(firstAborted, true);
});

test('failure restores state and actionHeaders forwards idempotency identity', async () => {
  const runtime = harness([mutation]);
  const button = element();
  await assert.rejects(
    runtime.window.ciwiRunAction('mutate', { id: 9 }, button, async () => { throw new Error('failed'); }),
    /failed/,
  );
  assert.equal(button.disabled, false);
  assert.equal(runtime.window.ciwiActiveOperations().length, 0);
  const headers = runtime.window.ciwiActionHeaders(
    { idempotencyKey: 'command-1' },
    { Accept: 'application/json' },
  );
  assert.equal(headers.Accept, 'application/json');
  assert.equal(headers['Idempotency-Key'], 'command-1');
});
