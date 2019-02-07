# Elm in the Browser!

This package allows you to create Elm programs that run in browsers.


## Learning Path

**I highly recommend working through [guide.elm-lang.org][guide] to learn how to use Elm.** It is built around a learning path that introduces concepts gradually.

[guide]: https://guide.elm-lang.org/

You can see the outline of that learning path in the `Browser` module. It lets you create Elm programs with the following functions:

  1. [`sandbox`](Browser#sandbox) &mdash; react to user input, like buttons and checkboxes
  2. [`element`](Browser#element) &mdash; talk to the outside world, like HTTP and JS interop
  3. [`document`](Browser#document) &mdash; control the `<title>` and `<body>`
  4. [`application`](Browser#application) &mdash; create single-page apps

This order works well because important concepts and techniques are introduced at each stage. If you jump ahead, it is like building a house by starting with the roof! So again, **work through [guide.elm-lang.org][guide] to see examples and really *understand* how Elm works!**

This order also works well because it mirrors how most people introduce Elm at work. Start small. Try using Elm in a single element in an existing JavaScript project. If that goes well, try doing a bit more. Etc.
