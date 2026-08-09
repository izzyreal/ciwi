const path = require('node:path');
const {Module} = require('node:module');

// Tests live beside the production browser adapter, while their dependencies
// remain isolated in this integration module.
process.env.NODE_PATH = path.join(__dirname, 'node_modules');
Module._initPaths();

const {defineConfig} = require('@playwright/test');

const junitReport = process.env.CIWI_JUNIT_REPORT;

module.exports = defineConfig({
  testDir: '../../internal/server/webui/browser-tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: junitReport
    ? [['junit', {outputFile: junitReport}]]
    : (process.env.CI ? 'github' : 'line'),
  use: {
    browserName: 'chromium',
    headless: true,
  },
});
