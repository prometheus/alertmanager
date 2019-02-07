# Semantics

The goal of this document is to explain how different parsers fit together. When will it backtrack? When will it not?

<br>

### `keyword : String -> Parser ()`

Say we have `keyword "import"`:

| String        | Result     |
|---------------|------------|
| `"import"`    | `OK{false}` |
| `"imp"`       | `ERR{true}` |
| `"export"`    | `ERR{true}` |

In our `OK{false}` notation, we are indicating:

1. Did the parser succeed? `OK` if yes. `ERR` if not.
2. Is it possible to backtrack? So when `keyword` succeeds, backtracking is not allowed anymore. You must continue along that path.

<br>


### `map : (a -> b) -> Parser a -> Parser b`

Say we have `map func parser`:

| `parser` | Result   |
|----------|----------|
| `OK{b}`  | `OK{b}`  |
| `ERR{b}` | `ERR{b}` |

So result of `map func parser` is always the same as the result of the `parser` itself.

<br>


### `map2 : (a -> b -> c) -> Parser a -> Parser b -> Parser c`

Say we have `map2 func parserA parserB`:

| `parserA` | `parserB` | Result         |
|-----------|-----------|----------------|
| `OK{b}`   | `OK{b'}`  | `OK{b && b'}`  |
| `OK{b}`   | `ERR{b'}` | `ERR{b && b'}` |
| `ERR{b}`  |           | `ERR{b}`       |

If `parserA` succeeds, we try `parserB`. If they are both backtrackable, the combined result is backtrackable.

If `parserA` fails, that is our result.

This is used to define our pipeline operators like this:

```elm
(|.) a b = map2 (\keep ignore -> keep) a b
(|=) a b = map2 (\func arg -> func arg) a b
```

<br>


### `either : Parser a -> Parser a -> Parser a`

Say we have `either parserA parserB`:

| `parserA`    | `parserB` | Result       |
|--------------|-----------|--------------|
| `OK{b}`      |           | `OK{b}`      |
| `ERR{true}`  | `OK{b}`   | `OK{b}`      |
| `ERR{true}`  | `ERR{b}`  | `ERR{b}`     |
| `ERR{false}` |           | `ERR{false}` |

The 4th case is very important! **If `parserA` is not backtrackable, you do not even try `parserB`.**

The `either` function does not appear in the public API, but I used it here because it makes the rules a bit easier to read. In the public API, we have `oneOf` instead. You can think of `oneOf` as trying `either` the head of the list, or `oneOf` the parsers in the tail of the list.

<br>


### `andThen : (a -> Parser b) -> Parser a -> Parser b`

Say we have `andThen callback parserA` where `callback a` produces `parserB`:

| `parserA` | `parserB` | Result         |
|-----------|-----------|----------------|
| `ERR{b}`  |           | `ERR{b}`       |
| `OK{b}`   | `OK{b'}`  | `OK{b && b'}`  |
| `OK{b}`   | `ERR{b'}` | `ERR{b && b'}` |

If both parts are backtrackable, the overall result is backtrackable.

<br>


### `backtrackable : Parser a -> Parser a`

Say we have `backtrackable parser`:

| `parser` | Result      |
|----------|-------------|
| `OK{b}`  | `OK{true}`  |
| `ERR{b}` | `ERR{true}` |

No matter how `parser` was defined, it is backtrackable now. This becomes very interesting when paired with `oneOf`. You can have one of the options start with a `backtrackable` segment, so even if you do start down that path, you can still try the next parser if something fails. **This has important yet subtle implications on performance, so definitely read on!**

<br>


## Examples

This parser is intended to give you very precise control over backtracking behavior, and I think that is best explained through examples.

<br>

### `backtrackable`

Say we have `map2 func (backtrackable spaces) (symbol ",")` which can eat a bunch of spaces followed by a comma. Here is how it would work on different strings:

| String  | Result      |
|---------|-------------|
| `"  ,"` | `OK{false}` |
| `"  :"` | `ERR{true}` |
| `"abc"` | `ERR{true}` |

Remember how `map2` is backtrackable only if both parsers are backtrackable. So in the first case, the overall result is not backtrackable because `symbol ","` succeeded.

This becomes useful when paired with `either`!

<br>


### `backtrackable` + `oneOf` (inefficient)

Say we have the following `parser` definition:

```elm
parser : Parser (Maybe Int)
parser =
  oneOf
    [ succeed Just
        |. backtrackable spaces
        |. symbol ","
        |. spaces
        |= int
    , succeed Nothing
        |. spaces
        |. symbol "]"
    ]
```

Here is how it would work on different strings:

| String    | Result       |
|-----------|--------------|
| `"  , 4"` | `OK{false}`  |
| `"  ,"`   | `ERR{false}` |
| `"  , a"` | `ERR{false}` |
| `"  ]"`   | `OK{false}`  |
| `"  a"`   | `ERR{false}` |
| `"abc"`   | `ERR{true}`  |

Some of these cases are tricky, so let's look at them in more depth:

- `"  , a"` &mdash; `backtrackable spaces`, `symbol ","`, and `spaces` all succeed. At that point we have `OK{false}`. The `int` parser then fails on `a`, so we finish with `ERR{false}`. That means `oneOf` will NOT try the second possibility.
- `"  ]"` &mdash; `backtrackable spaces` succeeds, but `symbol ","` fails. At that point we have `ERR{true}`, so `oneOf` tries the second possibility. After backtracking, `spaces` and `symbol "]"` succeed with `OK{false}`.
- `"  a"` &mdash; `backtrackable spaces` succeeds, but `symbol ","` fails. At that point we have `ERR{true}`, so `oneOf` tries the second possibility. After backtracking, `spaces` succeeds with `OK{false}` and `symbol "]"` fails resulting in `ERR{false}`.

<br>


### `oneOf` (efficient)

Notice that in the previous example, we parsed `spaces` twice in some cases. This is inefficient, especially in large files with lots of whitespace. Backtracking is very inefficient in general though, so **if you are interested in performance, it is worthwhile to try to eliminate as many uses of `backtrackable` as possible.**

So we can rewrite that last example to never backtrack:

```elm
parser : Parser (Maybe Int)
parser =
  succeed identity
  	|. spaces
  	|= oneOf
        [ succeed Just
            |. symbol ","
            |. spaces
            |= int
        , succeed Nothing
            |. symbol "]"
        ]
```

Now we are guaranteed to consume the spaces only one time. After that, we decide if we are looking at a `,` or `]`, so we never backtrack and reparse things.

If you are strategic in shuffling parsers around, you can write parsers that do not need `backtrackable` at all. The resulting parsers are quite fast. They are essentially the same as [LR(k)](https://en.wikipedia.org/wiki/Canonical_LR_parser) parsers, but more pleasant to write. I did this in Elm compiler for parsing Elm code, and it was very significantly faster.
