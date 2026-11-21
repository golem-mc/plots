# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

It is a private copy the agent system puts into the working tree; git ignores it
and the public repository does not have it. Read it, never edit it, and never
mention it in a commit, PR or issue. Contributors' public rules are in
`CONTRIBUTING.md`.

## Project

A plugin for golem (github.com/golem-mc/golem), a Minecraft server in Go with
Bukkit-style plugins written in Go (golem's `plugin/` package and
`docs/plugins.md`). It ports a Bukkit/Paper plugin, by repository: essentials =
EssentialsX, perms = LuckPerms, worldedit = FastAsyncWorldEdit, spark = spark,
multiverse = Multiverse, vault = Vault. `PORTING.md`, where there is one, records
the port's decisions.

The reference is the original plugin's behaviour on Paper: its commands and
their syntax, permission nodes, messages and config files, from its
documentation and source, not from memory. Not vanilla: golem already matches
vanilla, and the plugin matches the original plugin on top of it.

## golem

- golem is a dependency: never edit it, its module cache or a copy of it. When
  golem lacks or breaks something this work needs, file a precise issue in
  golem-mc/golem and make the plugin issue wait on it with
  `Depends on: golem-mc/golem#N` (the job's prompt says how).
- Once that issue is closed, bump golem:
  `mise exec -- go get github.com/golem-mc/golem@main && mise exec -- go mod tidy`.
- golem's server and plugin API never get an economy or other Vault-style
  shared-service interface: those belong to golem-mc/vault.

## Tests

Tests use golem's `plugintest` (github.com/golem-mc/golem/plugintest) and stay
cheap: every PR waits for the whole suite, so a test starts only the server,
chunks and ticks its assertion needs, shares its package's fixture, and runs in
well under 10s. Never run the whole suite locally: only the new or changed
tests, by name.

## Toolchain

Run Go through mise: `mise exec -- go ...`. Use the repository's mise tasks when
it has them (`mise tasks`): `mise run fmt`, `fix`, `tidy`, `build`, `lint`, then
`mise run test:changed`. Otherwise: `mise exec -- gofmt -l .`,
`mise exec -- go vet ./...`, `mise exec -- go mod tidy -diff`, and
`mise exec -- go test ./pkg -run 'TestA|TestB'` for the changed packages.

## README

The README is the user guide for a server owner: everything they need to
install, set up and use the plugin without reading its source, and nothing
about how it is built. A user should not have to open the original plugin's
documentation for everyday use; link it for the rarely used corners.

- intro (two or three lines) and install (a copyable `golem-launcher plugin add <package
  path>` command for every module, saying which are optional)
- for a plugin other plugins build on (an API such as vault's services or
  WorldEdit's and WorldGuard's APIs): a "For plugin developers" section with
  the `go get` line and short, compiling Go examples of providing (registering)
  and using each service; for vault this is the main section
- getting started: the first steps after installing, with a short example
- commands: a table per command group with the usage, what it does and the
  permission, plus the aliases a user would type; one row per command, not
  per flag (mention the common flags in the usage)
- permissions: the important nodes and their defaults
- configuration: the files a user edits and their main options with defaults
- migrating from Bukkit or Paper, and which other golem plugins it works with
  (Vault, WorldEdit, ...), when they apply
- differences from the original a user would notice, one line each
- no "Not yet" or to-do list: the README is not an issue tracker. Every
  missing feature or known gap is an open issue (in this repo, with a
  `Depends on: golem-mc/golem#N` line when golem must change first), and the
  README describes only what works
- leave out implementation details: internal storage of files such as
  translations, caching, internal quirks and package layout; those go in PORTING.md
- a feature PR updates the sections its feature touches

## Commits and pull requests

PR titles and commit messages follow Conventional Commits: PRs are
squash-merged with the title as the commit, and release-please builds the
version and CHANGELOG.md from them. A check refuses any other title.

- `fix: ...` a bug a player or admin can notice, `feat: ...` new behaviour or a
  new command, and `perf`, `refactor`, `test`, `docs`, `build`, `ci`, `chore`
  for changes no user notices; `!` after the type only for a breaking change
  to commands, permissions, config or saved data
- the summary is lower case, imperative and says what changes for a user; never
  put the issue number in the title, write `Fixes #N` in the body
