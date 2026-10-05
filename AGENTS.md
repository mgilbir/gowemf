# Project rules

- Implement from MS-WMF, MS-EMF, and MS-EMFPLUS specifications. Keep coverage and
  limitations explicit; framing acceptance is not semantic or rendering support.
- The approved external Go dependency is github.com/mgilbir/golittlecms for color
  management. Keep other implementation code dependency-free. Correctness,
  security, and performance are required.
- Read the existing code and relevant specification before editing. Keep bounds,
  timeouts, and resource limits enabled. Verify arithmetic on 32-bit platforms.
- Keep third-party fixtures/spec downloads in the ignored `.external/` directory
  or `/tmp`; never commit external binary examples. Extend the pinned manifest
  and download target when adding corpus inputs.
- Generate committed test inputs in code. Check licenses before porting anything;
  retain required legal notices. Use GPL/MPL renderers as execution-only oracles,
  without reading or porting their implementation source.
- Run `make check` for code changes, `make fuzz` for parser changes, and
  `make test-external` for framing/corpus changes. Report unavailable checks.
  Prove new regression assertions detect the defect they are meant to watch.
- Filesystem access is limited to this repository and `/tmp`. To inspect another
  project, check it out from GitHub into `/tmp`; do not commit to that checkout.
- No AI attribution in commit messages or PR descriptions. Commit, push, or open
  a PR only when requested. No subagent delegation unless explicitly requested.
