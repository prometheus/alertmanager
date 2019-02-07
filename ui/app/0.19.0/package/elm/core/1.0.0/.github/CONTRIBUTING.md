# Contributing to the core libraries

Thanks helping with the development of Elm! This document describes the basic
standards for opening pull requests and making the review process as smooth as
possible.

## Ground rules

  * Always make pull requests minimal. If it can be split up, it should be split up.
  * Use style consistent with the file you are modifying.
  * Use descriptive titles for PRs
  * Provide all the necessary context for evaluation in the PR.
    If there are relevant issues or examples or discussions, add them.
    If things can be summarized, summarize them. The easiest PRs are ones
    that already address the reviewers questions and concerns.

## Documentation Fixes

If you want to fix docs, just open a PR. This is super helpful!

## Bug Fixes

If you find an issue or see one you want to work on, go for it!

The best strategy is often to dive in. Asking for directions usually
does not work. If someone knew the specifics and knew how how to fix
it, it is likely they would have already sent the PR themselves!

Also, be sure you are testing.

## Adding New Functions

We are fairly conservative about adding new functions to core libraries.
If you want to augment the `List` or `Array` library, we recommend creating
small packages called `list-extras` or `array-extras` that have all the
features you want. There are already several such packages maintained at
the [Elm Community organization](https://github.com/elm-community) that
welcome contributions in the form of pull requests.

Long term, we will set up a process to review `*-extras` packages to move
stuff into core. By going through packages, it will be much easier to assess
whether a function is pleasant and useful in practice before committing to it
in the core libraries.
