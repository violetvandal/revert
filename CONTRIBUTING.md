# Contributing to Revert

Full contributor docs live at **<https://docs.thug2vandal.com/contributing/>** (the developer
docs site). The short version:

- **Build from source:** `git clone --recursive`, then `cd tools/thugkit && go build
  ./cmd/thugkit && go test ./...`, then `./revert doctor` and `./revert build` from the root.
- **Go for anything shipped or cross-platform**; Python only for one-off RE / author-side tools.
- **Write unit tests with new Go code**, keep `go test ./...` green, and fuzz the byte-critical
  codecs.
- **Byte-perfection = boot safety.** Inject `qb_scripts` compressed (about 1.43 MiB ceiling),
  verify round-trips, and always boot-test after touching any front-end or boot-pack file.
- **No game data, ever.** Nothing licensed or derivative goes in a public repo.

Architecture, the build pipeline, the codecs, testing, mod authoring, and how to add a
platform lane are all documented at <https://docs.thug2vandal.com>.
