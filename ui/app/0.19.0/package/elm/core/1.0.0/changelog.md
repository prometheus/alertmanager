# 0.15

### Syntax

New `import` syntax with keyword `exposing`.

### Module Changes

  * Move `Http` to `elm-http` package and totally redo API
  * Remove `WebSocket` module
  * Add `Task` module

### Channels become Mailboxes

`Graphics.Input` now works with this API (from module `Signal`):

```elm
type alias Mailbox a = { address : Address a, signal : Signal a }

mailbox : a -> Mailbox a
```

You can then send messages to the `Address` with functions like `Signal.send`
and `Signal.message`, or create forwarding addresses with `Signal.forwardTo`.

### Text in Collages

`Graphics.Collage` now has two new functions:

```elm
text : Text -> Form
outlinedText : LineStyle -> Text -> Form
```

These functions render text with the canvas, making things quite a bit faster.
The underlying implementation of `Text` has also been improved dramatically.

### Miscellaneous

  * Change types of `head`, `tail`, `maximum`, `minimum` by wrapping output in `Maybe`
  * Move `leftAligned`, `centered`, `rightAligned` from `Text` to `Graphics.Element`
  * Move `asText` from `Text` to `Graphics.Element`, renaming it to `show` in the process
  * Remove `Text.plainText` (can be replaced by `Graphics.Element.leftAligned << Text.fromString`)
  * Change type of `Keyboard.keysDown` from `Signal (List KeyCode)` to `Signal (Set KeyCode)`
  * Remove `Keyboard.directions`
  * Rename `Keyboard.lastPressed` to `Keyboard.presses`


# 0.14

### Syntax

  * Keyword `type` becomes `type alias`
  * Keyword `data` becomes `type`
  * Remove special list syntax in types, so `[a]` becomes `List a`


### Reduce Default Imports

The set of default imports has been reduced to the following:

```haskell
import Basics (..)
import Maybe ( Maybe( Just, Nothing ) )
import Result ( Result( Ok, Err ) )
import List ( List )
import Signal ( Signal )
```

### Make JSON parsing easy

  * Added `Json.Decode` and `Json.Encode` libraries


### Use more natural names

  * Rename `String.show` to `String.toString`

  * Replace `List.zip` with `List.map2 (,)`
  * Replace `List.zipWith f` with `List.map2 f`

  * Rename `Signal.liftN` to `Signal.mapN`
  * Rename `Signal.merges` to `Signal.mergeMany`


### Simplify Signal Library

  * Revamp `Input` concept as `Signal.Channel`
  * Remove `Signal.count`
  * Remove `Signal.countIf`
  * Remove `Signal.combine`


### Randomness Done Right

  * No longer signal-based
  * Use a `Generator` to create random values



### Revamp Maybes and Error Handling

  * Add the following functions to `Maybe`

        withDefault : a -> Maybe a -> a
        oneOf : List (Maybe a) -> Maybe a
        map : (a -> b) -> Maybe a -> Maybe b
        andThen : Maybe a -> (a -> Maybe b) -> Maybe b

  * Remove `Maybe.maybe` so `maybe 0 sqrt Nothing` becomes `withDefault 0 (map sqrt Nothing)`

  * Remove `Maybe.isJust` and `Maybe.isNothing` in favor of pattern matching

  * Add `Result` library for proper error handling. This is for cases when
    you want a computation to succeed, but if there is a mistake, it should
    produce a nice error message.

  * Remove `Either` in favor of `Result` or custom union types

  * Revamp functions that result in a `Maybe`.

      - Remove `Dict.getOrElse` and `Dict.getOrFail` in favor of `withDefault 0 (Dict.get key dict)`
      - Remove `Array.getOrElse` and `Array.getOrFail` in favor of `withDefault 0 (Array.get index array)`
      - Change `String.toInt : String -> Maybe Int` to `String.toInt : String -> Result String Int`
      - Change `String.toFloat : String -> Maybe Float` to `String.toFloat : String -> Result String Float`


### Make appending more logical

  * Add the following functions to `Text`:
      
        empty : Text
        append : Text -> Text -> Text
        concat : [Text] -> Text
        join : Text -> [Text] -> Text

  * Make the following changes in `List`:
      - Replace `(++)` with `append`
      - Remove `join`

### Miscellaneous

  * Rename `Text.toText` to `Text.fromString`
