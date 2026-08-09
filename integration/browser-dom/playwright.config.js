const path = require('node:path');
const {Module} = require('node:module');

// Tests live beside the production browser adapter, while their dependencies
// remain isolated in this integration module.
process.env.NODE_PATH = path.join(__dirname, 'node_modules');
Module._initPaths();

const {defineConfig} = require('@playwright/test');

module.exports = defineConfig({
  testDir: '../../internal/server/webui/browser-tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? 'github' : 'line',
  use: {
    browserName: 'chromium',
    headless: true,
  },
});
