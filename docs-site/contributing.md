# Contributing

Thanks for wanting to dig in. This page covers the conventions, the repo layout, and the few
hard rules.

## Get set up

Follow [Getting started](index.md#build-from-source): clone `--recursive`, `go build
./cmd/thugkit`, `go test ./...`, then `./revert doctor` and `./revert build`. `revert doctor`
tells you exactly what is missing if a prerequisite is absent.

## Conventions

- **Go for anything shipped or cross-platform.** Compiled, zero runtime dependencies. Python
  is fine for one-off reverse-engineering or author-side tools (the CAS renderer stays
  Python), never for something users have to run to play.
- **Write unit tests with new Go code**, and keep `go test ./...` green. Fuzz the byte-critical
  codecs. See [Testing](testing.md).
- **Byte-perfection = boot safety.** Verify round-trips. The `qb_scripts.prx` compressed load
  ceiling is about 1.43 MiB; inject compressed. **Always boot-test** after touching any
  front-end or boot-pack file. See [Codecs](codecs.md).
- **No game data, ever.** Nothing licensed or derivative gets committed to a public repo: not
  the pristine base, the no-CD exe, the HQ packs, the licensed decks/guest models, or the
  derivative `.ns` mod sources. Those are user-supplied and gitignored, or live in the private
  `mods` repo. When in doubt, leave it out.

## Repo topology and the two-repo flow

The monorepo root is the `violetvandal/revert` toolkit. `tools/thugkit`, `tools/neverscript`,
and `mods` are **independent git repos** nested under it and gitignored by the root; the root
and `thugkit` communicate only through the built binary's CLI, never a Go import (see
[Architecture](architecture.md#the-cross-repo-boundary-the-one-hard-seam)).

Development happens in a private working root; a **curated export** publishes the
public-safe subset to the public repos. So when you send a change, keep public and private
concerns separate: engine, orchestrator, config, and docs are publishable; anything
reproducing game data or derivative sources is not.

## Privacy

This is a passion project published under the **Violet Vandal** persona. All public git work,
releases, and deploys go out as that persona over SSH remotes, never a personal identity.
Screenshots and videos in the player docs pass an identity-leak checklist before publishing
(no personal account names, hostnames, or private game-source links on camera). If you
contribute upstream, your own commits are your own; just keep persona-owned artifacts
persona-owned.

## Commits and PRs

- Keep `go test ./...` green and the affected round-trips intact.
- If you changed a front-end or boot-pack file, say in the PR that you boot-tested it, and on
  what.
- Small, focused changes over sweeping ones. Match the surrounding code's style.
- The parity harnesses ([Testing](testing.md)) are the way to prove the Go output still equals
  the reference pipeline byte-for-byte when you touch build or apply.

## Where things are documented

- System map: [Architecture](architecture.md)
- What a build does: [Build pipeline](build-pipeline.md)
- The formats and the boot ceiling: [Codecs](codecs.md)
- Tests, fuzzing, parity: [Testing](testing.md)
- The mod model: [Authoring mods](modding.md)
- Lanes and platforms: [Platform lanes](platform-lanes.md)
