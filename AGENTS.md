# Repository workflows

- Use existing `just` recipes for repeatable development and verification commands.
- Add or extend a recipe in `justfile` before running a missing workflow so future agents can reuse it.
- Use `mise exec -- just …` when `just` is not available directly. Recipes load the project toolchain through mise.
- Run targeted tests with `just test-run PATTERN PACKAGE`, then run `mise exec -- just check` before every code push, including PR repairs. This runs the same verification suite as CI: formatting, module verification, vet, tests, race tests, vulnerability scanning, release-tool tests, shell syntax checks, and cross-platform builds.
- Rerun `mise exec -- just check` after rebasing onto new upstream changes. Do not push when checks fail; fix the cause and rerun them first.
- Local checks run on the current operating system; cross-platform builds do not execute tests on other operating systems. Monitor both macOS and Linux CI results when requested, and add regression coverage for platform-specific failures.
- Report toolchain setup failures; do not substitute another toolchain and claim verification passed.
- Follow the applicable skills for Git commits and GitHub publishing.
