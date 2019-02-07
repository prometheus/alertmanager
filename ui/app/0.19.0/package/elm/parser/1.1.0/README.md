# Parser

Regular expressions are quite confusing and difficult to use. This library provides a coherent alternative that handles more cases and produces clearer code.

The particular goals of this library are:

  - Make writing parsers as simple and fun as possible.
  - Produce excellent error messages.
  - Go pretty fast.

This is achieved with a couple concepts that I have not seen in any other parser libraries: [parser pipelines](#parser-pipelines), [backtracking](#backtracking), and [tracking context](#tracking-context).


## Parser Pipelines

To parse a 2D point like `( 3, 4 )`, you might create a `point` parser like this:

```elm
import Parser exposing (Parser, (|.), (|=), succeed, symbol, float, spaces)

type alias Point =
  { x : Float
  , y : Float
  }

point : Parser Point
point =
  succeed Point
    |. symbol "("
    |. spaces
    |= float
    |. spaces
    |. symbol ","
    |. spaces
    |= float
    |. spaces
    |. symbol ")"
```

All the interesting stuff is happening in `point`. It uses two operators:

  - [`(|.)`][ignore] means “parse this, but **ignore** the result”
  - [`(|=)`][keep] means “parse this, and **keep** the result”

So the `Point` function only gets the result of the two `float` parsers.

[ignore]: https://package.elm-lang.org/packages/elm/parser/latest/Parser#|.
[keep]: https://package.elm-lang.org/packages/elm/parser/latest/Parser#|=

The theory is that `|=` introduces more “visual noise” than `|.`, making it pretty easy to pick out which lines in the pipeline are important.

I recommend having one line per operator in your parser pipeline. If you need multiple lines for some reason, use a `let` or make a helper function.



## Backtracking

To make fast parsers with precise error messages, all of the parsers in this package do not backtrack by default. Once you start going down a path, you keep going down it.

This is nice in a string like `[ 1, 23zm5, 3 ]` where you want the error at the `z`. If we had backtracking by default, you might get the error on `[` instead. That is way less specific and harder to fix!

So the defaults are nice, but sometimes the easiest way to write a parser is to look ahead a bit and see what is going to happen. It is definitely more costly to do this, but it can be handy if there is no other way. This is the role of [`backtrackable`](https://package.elm-lang.org/packages/elm/parser/latest/Parser#backtrackable) parsers. Check out the [semantics](https://github.com/elm/parser/blob/master/semantics.md) page for more details!


## Tracking Context

Most parsers tell you the row and column of the problem:

    Something went wrong at (4:17)

That may be true, but it is not how humans think. It is how text editors think! It would be better to say:

    I found a problem with this list:

        [ 1, 23zm5, 3 ]
             ^
    I wanted an integer, like 6 or 90219.

Notice that the error messages says `this list`. That is context! That is the language my brain speaks, not rows and columns.

Once you get comfortable with the `Parser` module, you can switch over to `Parser.Advanced` and use [`inContext`](https://package.elm-lang.org/packages/elm/parser/latest/Parser-Advanced#inContext) to track exactly what your parser thinks it is doing at the moment. You can let the parser know “I am trying to parse a `"list"` right now” so if an error happens anywhere in that context, you get the hand annotation!

This technique is used by the parser in the Elm compiler to give more helpful error messages.


<br>

<br>

## [Comparison with Prior Work](https://github.com/elm/parser/blob/master/comparison.md)
