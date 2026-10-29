# Releases
This page describes the release process and the currently planned schedule for upcoming releases as well as the respective release shepherd. Release shepherds are chosen on a voluntary basis.

## Release Schedule

Release cadence of first pre-releases being cut is approximately 8 weeks.

| release series | date (year-month-day) | release shepherd                          |
|----------------|-----------------------|-------------------------------------------|
| v0.26          | 2023-08-23            | Josh Abreu (Github: @gotjosh)             |
| v0.27          | 2024-02-28            | Josh Abreu (Github: @gotjosh)             |
| v0.28          | 2024-05-28            | Josh Abreu (Github: @gotjosh)             |
| v0.29          | 2025-11-01            | Joe Adams (Github: @sysadmind)            |
| v0.30          | 2025-12-12            | Solomon Jacobs (Github: @SoloJacobs)      |
| v0.31          | 2026-01-31            | Solomon Jacobs (Github: @SoloJacobs)      |
| v0.32          | 2026-04-08            | Solomon Jacobs (Github: @SoloJacobs)      |
| v0.33          | 2026-06-08            | Ethan Hunter (Github: @Spaceman1701)      |
| v0.34          | 2026-08-12            | Solomon Jacobs (Github: @SoloJacobs)      |

## Versioning

There is no committed `VERSION` file. promu derives the version from
`git describe --tags --always --dirty`, minus the leading `v`, so a commit
tagged `v0.35.0` builds as `0.35.0` and any other commit as the nearest tag plus
its distance and hash, e.g. `0.34.2-17-gf4ef433b`. Source tarballs made by
`git archive`, including GitHub's "Source code" downloads, get their version
from `VERSION.in` instead. To override the version locally, write it to
`VERSION`; `make` warns while the file is present. Remove it to resume
versioning from git. Release CI writes the triggering tag to `VERSION` itself,
so rebuilding a release candidate after the final tag exists on the same commit
still produces the candidate's version.

The release steps therefore differ from the generic Prometheus process:

1. The release PR only updates `CHANGELOG.md`. There is no version bump commit.
2. Tag the release commit `v0.35.0-rc.0` with `git tag -s` and push it. CI
   builds and publishes the pre-release.
3. Once the candidate has soaked, tag the *same* commit `v0.35.0`, again with
   `git tag -s`, and push. CI builds the final release from the identical tree.
   When several tags point at one commit, `git describe` uses the newest
   annotated tag, so create the final tag after the candidate and never as a
   lightweight tag from the GitHub release page.
4. After a release from a `release-0.NN` branch, merge the branch back into
   `main` with a merge commit, so the tag is reachable from `main` and `main`
   builds describe themselves relative to the latest patch release.

If you are interested in volunteering please create a pull request against the [prometheus/alertmanager](https://github.com/prometheus/alertmanager) repository and propose yourself for the release of your choice.

If you'd like to know more about the shepherd responsibilities or the release instructions please [refer to the `RELEASE.MD`](https://github.com/prometheus/prometheus/blob/main/RELEASE.md) in [prometheus/prometheus](https://github.com/prometheus/prometheus).
