# Browser integration specifications

These Playwright specifications exercise the production browser adapter in a
real DOM. They are intentionally not Go unit tests and are never run by
`go test ./...`.

The isolated runner and setup instructions are in
[`integration/browser-dom`](../../../../integration/browser-dom/README.md).

Future reconciliation expectations are named `[expected failure: slice N]`
and use Playwright's `test.fail` annotation. They execute today but do not make
the integration run fail until their corresponding delivery slice lands.
