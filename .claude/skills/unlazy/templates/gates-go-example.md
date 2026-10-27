# Gates: catalog rejects inverted year and price ranges

OWNS: backend/internal/httpapi/lot_handlers.go, backend/internal/httpapi/lot_handlers_test.go

Scope: GET /api/lots answers 400 when year_min > year_max or price_min_kgs > price_max_kgs, and nothing else in the backend regresses

- [ ] G0: this ledger states outcomes that can fail
  CHECK: node .claude/skills/unlazy/scripts/gate-lint.mjs GATES.md
  EXPECT: LINT OK
  EVIDENCE: pending

- [ ] G1: inverted year and price ranges return 400 with an error body
  CHECK: go test ./internal/httpapi -run "^TestListActiveLotsRejectsInvertedRanges$" -v -count=1
  EXPECT: /--- PASS: TestListActiveLotsRejectsInvertedRanges \(/
  CWD: backend
  EVIDENCE: pending

- [ ] G2: the whole backend still vets and every test package passes
  CHECK: go vet ./... && go test ./... -count=1
  EXPECT: /^ok\s+autoauction\/internal\/httpapi\s/m
  CWD: backend
  EVIDENCE: pending

- [ ] G3: the G1 test failed against the code before the fix (positive control)
  EVIDENCE: pending

<!--
Example for this repository (Go module `autoauction` in backend/, Windows,
checker default shell cmd.exe). Copy to the repository root as GATES.md.

- G1 names the test in EXPECT on purpose: `go test -run` that matches nothing
  still exits 0 with "no tests to run", so a missing test cannot pass.
- G2 matches a package `ok` line; any failing package makes go test exit
  nonzero, and `&&` stops on a go vet failure.
- G3 is manual: record the FAIL line seen before the handler change, e.g.
  "before fix: --- FAIL: TestListActiveLotsRejectsInvertedRanges (got 500, want 400)".
-->
