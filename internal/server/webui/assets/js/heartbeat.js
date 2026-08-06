(() => {
  'use strict';

  const fallbackDurationMilliseconds = 8000;
  const fallbackMinimumOpacity = .18;

  function cssValue(name) {
    if (typeof getComputedStyle !== 'function' || typeof document === 'undefined' || !document.documentElement) return '';
    return String(getComputedStyle(document.documentElement).getPropertyValue(name) || '').trim();
  }

  function durationMilliseconds() {
    const value = cssValue('--ciwi-heartbeat-fade-duration').toLowerCase();
    const match = value.match(/^([0-9]+(?:\.[0-9]+)?)(ms|s)$/);
    if (!match) return fallbackDurationMilliseconds;
    const duration = Number(match[1]) * (match[2] === 's' ? 1000 : 1);
    return Number.isFinite(duration) && duration > 0 ? duration : fallbackDurationMilliseconds;
  }

  function minimumOpacity() {
    const raw = cssValue('--ciwi-heartbeat-min-opacity');
    if (!raw) return fallbackMinimumOpacity;
    const value = Number(raw);
    return Number.isFinite(value) && value >= 0 && value <= 1 ? value : fallbackMinimumOpacity;
  }

  function timestampMilliseconds(value) {
    if (typeof value === 'number') return Number.isFinite(value) && value > 0 ? value : 0;
    const text = String(value || '').trim();
    if (!text) return 0;
    if (/^[0-9]+$/.test(text)) {
      const numeric = Number(text);
      return Number.isFinite(numeric) && numeric > 0 ? numeric : 0;
    }
    const parsed = new Date(text).getTime();
    return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
  }

  function opacity(timestamp, nowMilliseconds) {
    const beat = timestampMilliseconds(timestamp);
    if (!beat) return minimumOpacity();
    const now = Number.isFinite(Number(nowMilliseconds)) ? Number(nowMilliseconds) : Date.now();
    const elapsed = Math.max(0, now - beat);
    const remaining = Math.max(0, Math.min(1, 1 - elapsed / durationMilliseconds()));
    const minimum = minimumOpacity();
    return minimum + (1 - minimum) * remaining;
  }

  function ageLabel(timestamp, nowMilliseconds) {
    const beat = timestampMilliseconds(timestamp);
    if (!beat) return 'never';
    const now = Number.isFinite(Number(nowMilliseconds)) ? Number(nowMilliseconds) : Date.now();
    const elapsed = now - beat;
    if (elapsed < 0) return 'just now';
    const seconds = Math.floor(elapsed / 1000);
    if (seconds < 60) return seconds + 's ago';
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return minutes + 'm ago';
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return hours + 'h ago';
    return Math.floor(hours / 24) + 'd ago';
  }

  window.ciwiHeartbeat = Object.freeze({
    ageLabel,
    durationMilliseconds,
    minimumOpacity,
    opacity,
    timestampMilliseconds,
  });
})();
