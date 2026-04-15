# SAVK Release Checklist

## v0.1.3

Before publishing:

- run `GOCACHE=/tmp/savk-go-build make test GO=/usr/local/go/bin/go`
- run `SAVK_RUN_SYSTEMD_INTEGRATION=1 GOCACHE=/tmp/savk-go-build make integration GO=/usr/local/go/bin/go`
- verify that `./bin/savk version` shows the correct version, commit, and build date
- regenerate artifacts with `GOCACHE=/tmp/savk-go-build make dist GO=/usr/local/go/bin/go VERSION=0.1.3 COMMIT=<git-sha> BUILD_DATE=<utc-rfc3339>`
- regenerate `dist/SHA256SUMS` if the artifacts change
- verify that `examples/` still parse and still build checks
- confirm that the release is still limited to `linux-systemd`

Publication:

- publish `dist/savk-<version>-<goos>-<goarch>`
- publish `dist/savk-<version>-<goos>-<goarch>.tar.gz`
- publish `dist/SHA256SUMS`
- create the `v0.1.3` git tag
- attach the summary from [CHANGELOG.md](../CHANGELOG.md)
