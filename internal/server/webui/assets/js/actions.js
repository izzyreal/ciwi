(function () {
  'use strict';

  const activeByFingerprint = new Map();
  const activeByScope = new Map();
  let catalogPromise;

  function uiResourceURL(path) {
    return typeof window.ciwiUIResourceURL === 'function' ? window.ciwiUIResourceURL(path) : path;
  }

  function newActionID() {
    if (globalThis.crypto && typeof globalThis.crypto.randomUUID === 'function') {
      return globalThis.crypto.randomUUID();
    }
    const bytes = new Uint8Array(16);
    if (globalThis.crypto && typeof globalThis.crypto.getRandomValues === 'function') {
      globalThis.crypto.getRandomValues(bytes);
    } else {
      for (let index = 0; index < bytes.length; index += 1) {
        bytes[index] = Math.floor(Math.random() * 256);
      }
    }
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
    return hex.slice(0, 8) + '-' + hex.slice(8, 12) + '-' + hex.slice(12, 16) + '-' +
      hex.slice(16, 20) + '-' + hex.slice(20);
  }

  function catalog() {
    if (!catalogPromise) {
      catalogPromise = fetch(uiResourceURL('/ui/contracts/actions.json'))
        .then(async response => {
          if (!response.ok) throw new Error(await response.text());
          const document = await response.json();
          return new Map((document.actions || []).map(spec => [spec.command, spec]));
        });
    }
    return catalogPromise;
  }

  function canonicalArguments(argumentsValue) {
    const source = argumentsValue || {};
    return Object.keys(source).sort().reduce((result, key) => {
      result[key] = String(source[key] == null ? '' : source[key]);
      return result;
    }, {});
  }

  function fingerprint(command, argumentsValue) {
    return command + ':' + JSON.stringify(canonicalArguments(argumentsValue));
  }

  function resolveScope(spec, argumentsValue) {
    const args = canonicalArguments(argumentsValue);
    const resolved = String(spec.scope || spec.command || '').replace(/\{\{\s*([a-zA-Z][a-zA-Z0-9]*)\s*\}\}/g, (_, key) => args[key] || '');
    return resolved.trim() || spec.command;
  }

  function updateElement(element, operation, active) {
    if (!element) return;
    if (active) {
      if (!operation.elementState) {
        operation.elementState = {
          disabled: !!element.disabled,
        };
      }
      element.__ciwiActionOperation = operation;
      syncActionElement(element);
    } else {
      if (element.__ciwiActionOperation === operation) element.__ciwiActionOperation = null;
      const renderedDisabled = Object.prototype.hasOwnProperty.call(element, '__ciwiRenderedDisabled')
        ? !!element.__ciwiRenderedDisabled
        : !!(operation.elementState && operation.elementState.disabled);
      element.disabled = renderedDisabled;
      element.removeAttribute('aria-busy');
      element.classList.remove('ciwi-action-pending');
	  element.removeAttribute('data-ciwi-pending-label');
    }
  }

  function syncActionElement(element) {
    if (!element) return;
    const operation = element.__ciwiActionOperation;
    if (!operation) return;
    element.disabled = true;
    element.setAttribute('aria-busy', 'true');
    element.classList.add('ciwi-action-pending');
    if (operation.spec.pending) element.setAttribute('data-ciwi-pending-label', operation.spec.pending);
  }

  function notify() {
    window.dispatchEvent(new CustomEvent('ciwi:operations-changed', {
      detail: Array.from(activeByFingerprint.values()).map(operation => ({
        command: operation.command, arguments: operation.arguments, scope: operation.scope,
        pending: operation.spec.pending || '', startedAt: operation.startedAt,
      })),
    }));
  }

  async function runAction(command, argumentsValue, element, execute) {
    const specs = await catalog();
    const spec = specs.get(command) || { command: command, class: 'local', scope: command };
    if (spec.class === 'local') return execute({ signal: undefined, idempotencyKey: '' });
    const args = canonicalArguments(argumentsValue);
    const key = fingerprint(command, args);
    const duplicate = activeByFingerprint.get(key);
    if (duplicate) return duplicate.promise;
    const scope = resolveScope(spec, args);
    const occupying = activeByScope.get(scope);
    if (occupying) {
      if (spec.class === 'query' && occupying.spec.class === 'query') {
        occupying.controller.abort();
      } else {
        const message = occupying.spec.pending || 'A conflicting action is already in progress';
        throw new Error(message);
      }
    }
    const controller = new AbortController();
    const operation = {
      command: command, arguments: args, scope: scope, spec: spec, controller: controller,
      idempotencyKey: newActionID(), startedAt: Date.now(), promise: null,
    };
    // Claim the fingerprint and scope synchronously once the catalog lookup
    // completes. Deferring this registration until the execution microtask
    // lets two same-tick clicks both pass the duplicate check.
    updateElement(element, operation, true);
    activeByFingerprint.set(key, operation);
    activeByScope.set(scope, operation);
    notify();
    operation.promise = Promise.resolve()
      .then(() => execute({
        signal: controller.signal,
        idempotencyKey: operation.idempotencyKey,
        refreshOnSuccess: !!operation.spec.refreshOnSuccess,
      }))
      .finally(() => {
        if (activeByFingerprint.get(key) === operation) activeByFingerprint.delete(key);
        if (activeByScope.get(scope) === operation) activeByScope.delete(scope);
        updateElement(element, operation, false);
        notify();
      });
    return operation.promise;
  }

  function actionHeaders(runtime, headers) {
    const result = { ...(headers || {}) };
    if (runtime && runtime.idempotencyKey) result['Idempotency-Key'] = runtime.idempotencyKey;
    return result;
  }

  function confirmAction(confirmation) {
    if (!confirmation) return true;
    return window.confirm(confirmation.message || confirmation.title || 'Continue?');
  }

  window.ciwiRunAction = runAction;
  window.ciwiActionHeaders = actionHeaders;
  window.ciwiActionID = newActionID;
  window.ciwiConfirmAction = confirmAction;
  window.ciwiSyncActionElement = syncActionElement;
  window.ciwiActiveOperations = () => Array.from(activeByFingerprint.values());
  // Warm the shared contract so the first user action receives immediate
  // feedback without waiting for a catalog round trip.
  void catalog();
})();
