# Browser DOM integration tests

This is an integration-test module, not part of ciwi's Go unit-test suite.
`go test ./...` does not require Node, npm, Playwright, or a browser.

Run it explicitly from this directory:

```sh
npm ci
npx playwright install chromium
npm test
```

The module is intended to move into the repository's containerized integration
test workflow once that environment is available.
