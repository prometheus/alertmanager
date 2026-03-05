# Contributing

This document describes how to:

- Set up your dev environment
- Become familiar with [Elm](http://elm-lang.org/)
- Develop against AlertManager

## Dev Environment Setup

You can either use our default Docker setup or install all dev dependencies
locally. For the former you only need Docker installed, for the latter you need
to set the environment flag `NO_DOCKER` to `true` and have the following
dependencies installed:

- [Elm](https://guide.elm-lang.org/install.html#install)
- [Elm-Format](https://github.com/avh4/elm-format) is installed

In addition for easier development you
can [configure](https://guide.elm-lang.org/install.html#configure-your-editor)
your editor.

**All submitted elm code must be formatted with `elm-format`**. Install and
execute it however works best for you. We recommend having formatting the file
on save, similar to how many developers use `gofmt`.

If you prefer, there's a make target available to format all elm source files:

```
# make format
```

## Elm Resources

- The [Official Elm Guide](https://guide.elm-lang.org/) is a great place to
  start. Going through the entire guide takes about an hour, and is a good
  primer to get involved in our codebase. Once you've worked through it, you
  should be able to start writing your feature with the help of the compiler.
- Check the [syntax reference](http://elm-lang.org/docs/syntax) when you need a
  reminder of how the language works.
- Read up on [how to write elm code](http://elm-lang.org/docs/style-guide).
- Watch videos from the
  latest [elm-conf](https://www.youtube.com/channel/UCOpGiN9AkczVjlpGDaBwQrQ)
- Learn how to use the debugger! Elm comes packaged with an excellent
  [debugger](http://elm-lang.org/blog/the-perfect-bug-report). We've found this
  tool to be invaluable in understanding how the app is working as we're
  debugging behavior.

## Local development workflow

At the top level of this repo, follow the HA AlertManager instructions. Compile
the binary, then run with `goreman`. Add example alerts with the file provided
in the HA example folder. Then start the development server:

```
# cd ui/app
# make dev-server
```

Your app should be available at `http://localhost:<port>`. Navigate to
`src/Main.elm`. Any changes to the file system are detected automatically,
triggering a recompile of the project.

## Committing changes

Before you commit changes, please run `make build-all` on the root level
Makefile.
