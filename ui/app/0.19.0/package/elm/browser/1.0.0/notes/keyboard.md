# Which key was pressed?

When you listening for global keyboard events, you very likely want to know *which* key was pressed. Unfortunately different browsers implement the [`KeyboardEvent`][ke] values in different ways, so there is no one-size-fit-all solution.

[ke]: https://developer.mozilla.org/en-US/docs/Web/API/KeyboardEvent

## `charCode` vs `keyCode` vs `which` vs `key` vs `code`

As of this writing, it seems that the `KeyboardEvent` API recommends using [`key`][key]. It can tell you which symbol was pressed, taking keyboard layout into account. So it will tell you if it was a `x`, `か`, `ø`, `β`, etc.

[key]: https://developer.mozilla.org/en-US/docs/Web/API/KeyboardEvent/key

According to [the docs][ke], everything else is deprecated. So `charCode`, `keyCode`, and `which` are only useful if you need to support browsers besides [these](http://caniuse.com/#feat=keyboardevent-key).


## Writing a `key` decoder

The simplest approach is to just decode the string value:

```elm
import Json.Decode as Decode

keyDecoder : Decode.Decoder String
keyDecoder =
  Decode.field "key" Decode.string
```

Depending on your scenario, you may want something more elaborate though!


### Decoding for User Input

If you are handling user input, maybe you want to distinguish actual characters from all the different [key values](https://developer.mozilla.org/en-US/docs/Web/API/KeyboardEvent/key/Key_Values) that may be produced for non-character keys. This way pressing `h` then `i` then `Backspace` does not turn into `"hiBackspace"`. You could do this:

```elm
import Json.Decode as Decode

type Key
  = Character Char
  | Control String

keyDecoder : Decode.Decoder Key
keyDecoder =
  Decode.map toKey (Decode.field "key" Decode.string)

toKey : String -> Key
toKey string =
  case String.uncons string of
    Just (char, "") ->
      Character char

    _ ->
      Control string
```

> **Note:** The `String.uncons` function chomps surrogate pairs properly, so it works with characters outside of the BMP. If that does not mean anything to you, you are lucky! In summary, a tricky character encoding problem of JavaScript is taken care of with this code and you do not need to worry about it. Congratulations!


### Decoding for Games

Or maybe you want to handle left and right arrows specially for a game or a presentation viewer. You could do something like this:

```elm
import Json.Decode as Decode

type Direction
  = Left
  | Right
  | Other

keyDecoder : Decode.Decoder Direction
keyDecoder =
  Decode.map toDirection (Decode.field "key" Decode.string)

toDirection : String -> Direction
toDirection string =
  case string of
    "ArrowLeft" ->
      Left

    "ArrowRight" ->
      Right

    _ ->
      Other
```

By converting to a specialized `Direction` type, the compiler can guarantee that you never forget to handle one of the valid inputs. If it was a `String`, new code could have typos or missing branches that would be hard to find.

Hope that helps you write a decoder that works for your scenario!
