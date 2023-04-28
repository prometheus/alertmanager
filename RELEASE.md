# Releases
This page describes the release process and the currently planned schedule for upcoming releases as well as the respective release shepherd. Release shepherds are chosen on a voluntary basis.

## Release Schedule

Release cadence of first pre-releases being cut is 6 weeks.

| release series | date (year-month-day) | release shepherd              |
|----------------|-----------------------|-------------------------------|
| v0.26          | 2023-05-31            | Josh Abreu (Github: @gotjosh) |
| v0.27          | 2023-07-15            | Josh Abreu (Github: @gotjosh) |

If you are interested in volunteering please create a pull request against the [prometheus/alertmanager](https://github.com/prometheus/alertmanager) repository and propose yourself for the release of your choice.

## Release shepherd responsibilities

The release shepherd is responsible for the entire release series of a minor release. 

* We aim to keep the main branch in a working state at all times. In principle, it should be possible to cut a release from main at any time. In practice, things might not work out as nicely. A few days before the pre-release is scheduled, the shepherd should check the state of main. Following their best judgement, the shepherd should try to expedite bug fixes that are still in progress but should make it into the release. On the other hand, the shepherd may hold back merging last-minute invasive and risky changes that are better suited for the next minor release.
* On the date listed in the table above, the release shepherd cuts the first pre-release (using the suffix `-rc.0`) and creates a new branch called  `release-<major>.<minor>` starting at the commit tagged for the pre-release. In general, a pre-release is considered a release candidate (that's what `rc` stands for) and should therefore not contain any known bugs that are planned to be fixed in the final release.
* With the pre-release, the release shepherd is responsible for making sure the release candidate is in a good state.  After the pre-release is out for 3 days it is promoted to a stable release.
* If regressions or critical bugs are detected, they need to get fixed before cutting a new pre-release (called `-rc.1`, `-rc.2`, etc.).

See the next section for details on cutting an individual release.

## Release Instructions

TBD
