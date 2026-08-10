# Browser DOM integration tests

This is an integration-test module, not part of ciwi's Go unit-test suite.
`go test ./...` does not require Node, npm, Playwright, or a browser.

Run it explicitly from this directory:

```sh
npm ci
npx playwright install chromium
npm test
```

The `build` pipeline runs this module in the matching official Playwright
container as its `integration-tests` job. That job is separate from
`unit-tests`; both must succeed before `build-cross-platform` starts. The
container runs with the agent user's UID and GID so its bind-mounted test
output remains removable by the agent on the next execution.
