# Changelog

## [0.4.0](https://github.com/kamaal111/namu/compare/v0.3.0...v0.4.0) (2026-10-31)


### ⚠ BREAKING CHANGES

* **cli:** create/open stdout now abbreviates home paths with ~. Scripts must expand leading ~ before using the returned path. Reload shell integration after updating Namu to retain automatic navigation.

### Features

* **cli:** load branch completion through shell initialization ([3637762](https://github.com/kamaal111/namu/commit/363776241d1c0ea324a05e30d64cc427c101a0c9))
* **cli:** show optional PR numbers in list output ([4f7740e](https://github.com/kamaal111/namu/commit/4f7740eb14ea0de547a8883e020786a6d654f093))
* **integration:** add typed worktree lifecycle hooks ([3f88048](https://github.com/kamaal111/namu/commit/3f8804830a17a98ca8faf58301c7d43a038dd417))
* **integration:** open worktrees in reusable Rex sessions ([d9d1aed](https://github.com/kamaal111/namu/commit/d9d1aed489d05e96164d68755f9320f8d67cc42f))
* **worktree:** allow deleting external checkouts ([4d184e0](https://github.com/kamaal111/namu/commit/4d184e0258a75804b425be09a8f3055226fb1377))


### Bug Fixes

* **cli:** abbreviate home paths with a tilde ([823b4e2](https://github.com/kamaal111/namu/commit/823b4e2aa7b82b260f633aaa2635aa3411210a9c))
* **cli:** explain worktree conditions in list output ([2c83374](https://github.com/kamaal111/namu/commit/2c83374b879ee6d74b59a789f110335a429af281))

## [0.3.0](https://github.com/kamaal111/namu/compare/v0.2.0...v0.3.0) (2026-10-31)


### Features

* install shell navigation and delete multiple worktrees ([70a5821](https://github.com/kamaal111/namu/commit/70a5821a0382486445236490269a3b14b5a95d52))

## [0.2.0](https://github.com/kamaal111/namu/compare/v0.1.0...v0.2.0) (2026-10-30)


### Features

* integrate Herdr worktrees and shell navigation ([2e3c72e](https://github.com/kamaal111/namu/commit/2e3c72e69a1e32c48dd8a4be765124c5fc527616))

## 0.1.0 (2026-10-29)


### Features

* bootstrap the initial Namu release ([3558e8d](https://github.com/kamaal111/namu/commit/3558e8dda0004ae52593763c3c8500eba9a37f4f))


### Bug Fixes

* **release:** start the first release at 0.1.0 ([f8ba201](https://github.com/kamaal111/namu/commit/f8ba201a6d327b232fc1e1964520c2db6c5c8bf6))

## Changelog

Release notes are maintained by Release Please.
