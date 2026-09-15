<!--
    - Please give your PR a title in the form "area: short description".  For example "dispatcher: improve performance"

    - Please sign-off your commits by adding the -s / --signoff flag to `git commit`. See https://github.com/apps/dco for more information.

    - If the PR adds or changes a behaviour or fixes a bug of an exported API it would need a unit/e2e test.

    - Where possible use only exported APIs for tests to simplify the review and make it as close as possible to an actual library usage.

    - Performance improvements would need a benchmark test to prove it.

    - All exposed objects should have a comment.

    - All comments should start with a capital letter and end with a full stop.
 -->

#### Pull Request Checklist
Please check all the applicable boxes.

- Please list all open issue(s) discussed with maintainers related to this change
    - Fixes #<issue number>
    <!--
    If it applies.
    Automatically closes linked issue when PR is merged.
    Usage: `Fixes #<issue number>`, or `Fixes (paste link of issue)`.
    More at https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue#linking-a-pull-request-to-an-issue-using-a-keyword
    -->
- Is this a new Receiver integration?
    - [ ] I have already tried to use the [Webhook Receiver Integration](https://prometheus.io/docs/alerting/latest/configuration/#webhook_config) and [3rd party integrations](https://prometheus.io/docs/operating/integrations/#alertmanager-webhook-receiver) before adding this new Receiver Integration
- Is this a bugfix?
    - [ ] I have added tests that can reproduce the bug which pass with this bugfix applied
- Is this a new feature?
    - [ ] I have added tests that test the new feature's functionality
- Does this change affect performance?
    - [ ] I have provided benchmarks comparison that shows performance is improved or is not degraded
        - You can use [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) to compare benchmarks
    - [ ] I have added new benchmarks if required or requested by maintainers
- Is this a breaking change?
    - [ ] My changes do not break the existing cluster messages
    - [ ] My changes do not break the existing api
- [ ] I have added/updated the required documentation
- [ ] I have signed-off my commits
- [ ] I will follow [best practices for contributing to this project](https://docs.github.com/en/get-started/exploring-projects-on-github/contributing-to-open-source)

#### Which user-facing changes does this PR introduce?
<!--
If no, just write "NONE" in the release-notes block below.
Otherwise, please describe what should be mentioned in the CHANGELOG. Use the following prefixes:
[FEATURE] [ENHANCEMENT] [PERF] [BUGFIX] [SECURITY] [CHANGE]
Refer to the existing CHANGELOG for inspiration:  https://github.com/prometheus/alertmanager/blob/main/CHANGELOG.md
A concrete example may look as follows (be sure to leave out the surrounding quotes): "[FEATURE] API: Add /api/v1/features for clients to understand which features are supported".
If you need help formulating your entries, consult the reviewer(s).
-->
```release-notes

```
