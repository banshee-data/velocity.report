# libpcap-static

Build infrastructure that produces a fully-static `libpcap.a` for
linux/amd64 and linux/arm64, used by `make build-radar-static` (see
[D-26](../../docs/DECISIONS.md)).

The libpcap source lives in the sibling submodule
[`third_party/libpcap`](../libpcap), pinned to the
`libpcap-1.10.6` release tag.

## Layout

```
build.sh              # invoked by the static-build Dockerfile (not on host)
toolchains/
  linux-amd64.cmake   # CMake toolchain file → wrappers/amd64/zig-cc
  linux-arm64.cmake   # CMake toolchain file → wrappers/arm64/zig-cc
  wrappers/{amd64,arm64}/{zig-cc,zig-ar,zig-ranlib}
                      # Shell wrappers that exec `zig cc -target …`
  zig-*.sh.in         # Templates the wrappers were rendered from
```

`build.sh <arch>` writes:

- `out/<arch>/lib/libpcap.a`
- `out/<arch>/include/pcap.h` (and the rest of the public headers)

These outputs are ephemeral inside the Docker image and gitignored on
the host.

## Upgrading libpcap

```bash
cd third_party/libpcap
git fetch --tags origin
git checkout libpcap-1.10.7      # or whatever the next release is
cd ../..
git add third_party/libpcap
make build-radar-static           # rebuild + smoke-test
git commit -m "[ai][sh] bump libpcap submodule to 1.10.7"
```

Always pin to a release tag — never to an arbitrary `master` SHA — so
that CVE tracking and release-note review have a stable point of
reference.

## Why this exists in-tree

velocity-report's gopacket/pcap usage requires `libpcap` at link time.
The distro packages link dynamically against `libpcap.so`, which
defeats the "self-contained binary" property the static build promises.
Vendoring the source as a submodule and building a static archive
in-tree means the binary's libpcap version is whatever this tree pins,
nothing else.
