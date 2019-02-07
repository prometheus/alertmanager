module Url.Parser.Query exposing
  ( Parser, string, int, enum, custom
  , map, map2, map3, map4, map5, map6, map7, map8
  )

{-| In [the URI spec](https://tools.ietf.org/html/rfc3986), Tim Berners-Lee
says a URL looks like this:

```
  https://example.com:8042/over/there?name=ferret#nose
  \___/   \______________/\_________/ \_________/ \__/
    |            |            |            |        |
  scheme     authority       path        query   fragment
```

This module is for parsing the `query` part.

In this library, a valid query looks like `?search=hats&page=2` where each
query parameter has the format `key=value` and is separated from the next
parameter by the `&` character.

# Parse Query Parameters
@docs Parser, string, int, enum, custom

# Mapping
@docs map, map2, map3, map4, map5, map6, map7, map8

-}

import Dict
import Url.Parser.Internal as Q



-- PARSERS


{-| Parse a query like `?search=hat&page=2` into nice Elm data.
-}
type alias Parser a =
  Q.QueryParser a



-- PRIMITIVES


{-| Handle `String` parameters.

    search : Parser (Maybe String)
    search =
      string "search"

    -- ?search=cats             == Just "cats"
    -- ?search=42               == Just "42"
    -- ?branch=left             == Nothing
    -- ?search=cats&search=dogs == Nothing

Check out [`custom`](#custom) if you need to handle multiple `search`
parameters for some reason.
-}
string : String -> Parser (Maybe String)
string key =
  custom key <| \stringList ->
    case stringList of
      [str] ->
        Just str

      _ ->
        Nothing


{-| Handle `Int` parameters. Maybe you want to show paginated search results:

    page : Parser (Maybe Int)
    page =
      int "page"

    -- ?page=2        == Just 2
    -- ?page=17       == Just 17
    -- ?page=two      == Nothing
    -- ?sort=date     == Nothing
    -- ?page=2&page=3 == Nothing

Check out [`custom`](#custom) if you need to handle multiple `page` parameters
or something like that.
-}
int : String -> Parser (Maybe Int)
int key =
  custom key <| \stringList ->
    case stringList of
      [str] ->
        String.toInt str

      _ ->
        Nothing


{-| Handle enumerated parameters. Maybe you want a true-or-false parameter:

    import Dict

    debug : Parser (Maybe Bool)
    debug =
      enum "debug" (Dict.fromList [ ("true", True), ("false", False) ])

    -- ?debug=true            == Just True
    -- ?debug=false           == Just False
    -- ?debug=1               == Nothing
    -- ?debug=0               == Nothing
    -- ?true=true             == Nothing
    -- ?debug=true&debug=true == Nothing

You could add `0` and `1` to the dictionary if you want to handle those as
well. You can also use [`map`](#map) to say `map (Result.withDefault False) debug`
to get a parser of type `Parser Bool` that swallows any errors and defaults to
`False`.

**Note:** Parameters like `?debug` with no `=` are not supported by this library.
-}
enum : String -> Dict.Dict String a -> Parser (Maybe a)
enum key dict =
  custom key <| \stringList ->
    case stringList of
      [str] ->
        Dict.get str dict

      _ ->
        Nothing



-- CUSTOM PARSERS


{-| Create a custom query parser. The [`string`](#string), [`int`](#int), and
[`enum`](#enum) parsers are defined using this function. It can help you handle
anything though!

Say you are unlucky enough to need to handle `?post=2&post=7` to show a couple
posts on screen at once. You could say:

    posts : Parser (Maybe (List Int))
    posts =
      custom "post" (List.maybeMap String.toInt)

    -- ?post=2        == [2]
    -- ?post=2&post=7 == [2, 7]
    -- ?post=2&post=x == [2]
    -- ?hats=2        == []
-}
custom : String -> (List String -> a) -> Parser a
custom key func =
  Q.Parser <| \dict ->
    func (Maybe.withDefault [] (Dict.get key dict))



-- MAPPING


{-| Transform a parser in some way. Maybe you want your `page` query parser to
default to `1` if there is any problem?

    page : Parser Int
    page =
      map (Result.withDefault 1) (int "page")

-}
map : (a -> b) -> Parser a -> Parser b
map func (Q.Parser a) =
  Q.Parser <| \dict -> func (a dict)


{-| Combine two parsers. A query like `?search=hats&page=2` could be parsed
with something like this:

    type alias Query =
      { search : Maybe String
      , page : Maybe Int
      }

    query : Parser Query
    query =
      map2 Query (string "search") (int "page")

-}
map2 : (a -> b -> result) -> Parser a -> Parser b -> Parser result
map2 func (Q.Parser a) (Q.Parser b) =
  Q.Parser <| \dict ->
    func (a dict) (b dict)


{-| Combine three parsers. A query like `?search=hats&page=2&sort=ascending`
could be parsed with something like this:

    import Dict

    type alias Query =
      { search : Maybe String
      , page : Maybe Int
      , sort : Maybe Order
      }

    type Order = Ascending | Descending

    query : Parser Query
    query =
      map3 Query (string "search") (int "page") (enum "sort" order)

    order : Dict.Dict String Order
    order =
      Dict.fromList
        [ ( "ascending", Ascending )
        , ( "descending", Descending )
        ]
-}
map3 : (a -> b -> c -> result) -> Parser a -> Parser b -> Parser c -> Parser result
map3 func (Q.Parser a) (Q.Parser b) (Q.Parser c) =
  Q.Parser <| \dict ->
    func (a dict) (b dict) (c dict)


{-|-}
map4 : (a -> b -> c -> d -> result) -> Parser a -> Parser b -> Parser c -> Parser d -> Parser result
map4 func (Q.Parser a) (Q.Parser b) (Q.Parser c) (Q.Parser d) =
  Q.Parser <| \dict ->
    func (a dict) (b dict) (c dict) (d dict)



{-|-}
map5 : (a -> b -> c -> d -> e -> result) -> Parser a -> Parser b -> Parser c -> Parser d -> Parser e -> Parser result
map5 func (Q.Parser a) (Q.Parser b) (Q.Parser c) (Q.Parser d) (Q.Parser e) =
  Q.Parser <| \dict ->
    func (a dict) (b dict) (c dict) (d dict) (e dict)


{-|-}
map6 : (a -> b -> c -> d -> e -> f -> result) -> Parser a -> Parser b -> Parser c -> Parser d -> Parser e -> Parser f -> Parser result
map6 func (Q.Parser a) (Q.Parser b) (Q.Parser c) (Q.Parser d) (Q.Parser e) (Q.Parser f) =
  Q.Parser <| \dict ->
    func (a dict) (b dict) (c dict) (d dict) (e dict) (f dict)



{-|-}
map7 : (a -> b -> c -> d -> e -> f -> g -> result) -> Parser a -> Parser b -> Parser c -> Parser d -> Parser e -> Parser f -> Parser g -> Parser result
map7 func (Q.Parser a) (Q.Parser b) (Q.Parser c) (Q.Parser d) (Q.Parser e) (Q.Parser f) (Q.Parser g) =
  Q.Parser <| \dict ->
    func (a dict) (b dict) (c dict) (d dict) (e dict) (f dict) (g dict)



{-| If you need higher than eight, you can define a function like this:

    apply : Parser a -> Parser (a -> b) -> Parser b
    apply argParser funcParser =
      map2 (<|) funcParser argParser

And then you can chain it to do as many of these as you would like:

    map func (string "search")
      |> apply (int "page")
      |> apply (int "per-page")

-}
map8 : (a -> b -> c -> d -> e -> f -> g -> h -> result) -> Parser a -> Parser b -> Parser c -> Parser d -> Parser e -> Parser f -> Parser g -> Parser h -> Parser result
map8 func (Q.Parser a) (Q.Parser b) (Q.Parser c) (Q.Parser d) (Q.Parser e) (Q.Parser f) (Q.Parser g) (Q.Parser h) =
  Q.Parser <| \dict ->
    func (a dict) (b dict) (c dict) (d dict) (e dict) (f dict) (g dict) (h dict)
