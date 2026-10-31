# Repository workflows

- Use existing `just` recipes for repeatable development and verification commands.
- Add or extend a recipe in `justfile` before running a missing workflow so future agents can reuse it.
- Use `mise exec -- just …` when `just` is not available directly. Recipes load the project toolchain through mise.
- Run targeted tests with `just test-run PATTERN PACKAGE`, then run `just check` before publishing code changes.
- Report toolchain setup failures; do not substitute another toolchain and claim verification passed.
- Follow the applicable skills for Git commits and GitHub publishing.
