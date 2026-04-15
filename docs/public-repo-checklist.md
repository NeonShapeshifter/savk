# Public Repo Checklist

SAVK can be made public when the repository does not overclaim beyond what is
actually verified today.

## Already Done

- README and base docs
- contract and report specs
- examples/
- changelog
- release notes
- release checklist
- `.gitignore`
- `CONTRIBUTING.md`
- `SECURITY.md`
- `LICENSE`

## Final Review Before Opening It

1. confirm that `README` does not make stronger claims than the tests
2. verify that you do not want to publish `bin/` or `dist/` in the repo
3. confirm that there are no secrets or local data in history
4. run `GOCACHE=/tmp/savk-go-build make test GO=/usr/local/go/bin/go`
5. run the real path from [docs/integration.md](integration.md) on a `linux-systemd` host
6. if you publish artifacts, attach them to the release rather than committing them into the repo
