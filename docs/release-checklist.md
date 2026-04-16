# SAVK Release Checklist

## v0.1.5

Before publishing:

- run `make clean` so ignored `bin/` and `dist/` artifacts from older versions
  do not remain in the local release path
- run `GOCACHE=/tmp/savk-go-build make test GO=/usr/local/go/bin/go`
- run `SAVK_RUN_SYSTEMD_INTEGRATION=1 GOCACHE=/tmp/savk-go-build make integration GO=/usr/local/go/bin/go`
  as the narrow observer-local smoke path, not as a broad service-backed proof
- verify that the README quickstart still matches reality:
  `GOCACHE=/tmp/savk-go-build go test ./cmd/savk -run TestREADMEQuickstartValidatePasses -count=1`
- verify that `./bin/savk version` shows the correct version, commit, and build date
- regenerate artifacts with `GOCACHE=/tmp/savk-go-build make build dist GO=/usr/local/go/bin/go VERSION=0.1.5 COMMIT=<git-sha> BUILD_DATE=<utc-rfc3339>`
- regenerate `dist/SHA256SUMS` if the artifacts change
- confirm that local `bin/` and `dist/` contain only the current `v0.1.5`
  artifacts you intend to publish
- verify that `examples/` still parse and still build checks
- confirm that the release is still limited to `linux-systemd`

Publication:

- publish `dist/savk-<version>-<goos>-<goarch>`
- publish `dist/savk-<version>-<goos>-<goarch>.tar.gz`
- publish `dist/SHA256SUMS`
- create the `v0.1.5` git tag
- attach the summary from [CHANGELOG.md](../CHANGELOG.md)
