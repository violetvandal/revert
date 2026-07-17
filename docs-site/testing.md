# Testing

The rule: **write unit tests with new Go code, keep `go test ./...` green, and fuzz the
byte-critical codecs.** Byte-perfection is boot safety, so the codecs carry the most tests.

## Unit tests (hermetic)

Every package in `thugkit` has package-local `*_test.go` tests, and the whole suite is
hermetic: it needs **no game data**.

```sh
cd tools/thugkit
go test ./...
```

Coverage lives with the code it exercises: the `prx`, `apply`, `build`, `tag`, `grf`, and
`imgxbx` packages each ship their own tests. The `prx` and `apply` packages carry the most,
because they are where a wrong byte breaks a boot.

## Fuzzing the byte-critical codec

LZSS is the one codec where a subtle encoder bug could produce output the game decompresses
incorrectly, so it has a fuzz target:

```sh
go test ./prx -run x -fuzz FuzzLZSS
```

Run it when you touch `prx/lzss.go` or anything that feeds it. New byte-critical codecs should
get their own fuzz target the same way.

## Parity harnesses (integration)

The Go engine replaced an earlier Python/bash reference pipeline. The `verify_apply*.sh` and
`verify_parity.py` scripts at the thugkit root are **integration harnesses** that diff the Go
output against that reference, byte-for-byte. Unlike the unit tests, they need the surrounding
project layout and real inputs, so they are not part of `go test`. Run them when you change the
build or apply steps and want to prove the output is still identical to the reference.

There is also a round-trip harness for the compiler (every modding-relevant `.qb` should
round-trip byte-identical); see [Codecs](codecs.md).

## CI

The only workflow today is `.github/workflows/windows-defender-scan.yml`, which scans the
Windows bundle. **There is no CI job running `go test` yet.** If you add one, keep it fast and
hermetic (the unit suite already is), and leave the parity harnesses as an on-demand
integration check rather than a per-push gate, since they need real inputs.

## Before you open a PR

- `go build ./cmd/thugkit && go test ./...` is green.
- If you touched a codec, its round-trip still holds and, for LZSS, the fuzzer runs clean for
  a bit.
- If you touched a front-end or boot-pack file, you **boot-tested** the resulting edition.
  Nothing else proves boot safety.
