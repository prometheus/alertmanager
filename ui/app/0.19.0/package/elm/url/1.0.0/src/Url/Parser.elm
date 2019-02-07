module Url.Parser exposing
  ( Parser, string, int, s
  , (</>), map, oneOf, top, custom
  , (<?>), query
  , fragment
  , parse
  )

{-| In [the URI spec](https://tools.ietf.org/html/rfc3986), Tim Berners-Lee
says a URL looks like this:

```
  https://example.com:8042/over/there?name=ferret#nose
  \___/   \______________/\_________/ \_________/ \__/
    |            |            |            |        |
  scheme     authority       path        query   fragment
```

This module is primarily for parsing the `path` part.


# Primitives
@docs Parser, string, int, s

# Path
@docs (</>), map, oneOf, top, custom

# Query
@docs (<?>), query

# Fragment
@docs fragment

# Run Parsers
@docs parse

-}

import Dict exposing (Dict)
import Url exposing (Url)
import Url.Parser.Query as Query
import Url.Parser.Internal as Q



-- INFIX TABLE


infix right 7 (</>) = slash
infix left  8 (<?>) = questionMark



-- PARSERS


{-| Turn URLs like `/blog/42/cat-herding-techniques` into nice Elm data.
-}
type Parser a b =
  Parser (State a -> List (State b))


type alias State value =
  { visited : List String
  , unvisited : List String
  , params : Dict String (List String)
  , frag : Maybe String
  , value : value
  }



-- PARSE SEGMENTS


{-| Parse a segment of the path as a `String`.

    -- /alice/  ==>  Just "alice"
    -- /bob     ==>  Just "bob"
    -- /42/     ==>  Just "42"
    -- /        ==>  Nothing
-}
string : Parser (String -> a) a
string =
  custom "STRING" Just


{-| Parse a segment of the path as an `Int`.

    -- /alice/  ==>  Nothing
    -- /bob     ==>  Nothing
    -- /42/     ==>  Just 42
    -- /        ==>  Nothing
-}
int : Parser (Int -> a) a
int =
  custom "NUMBER" String.toInt


{-| Parse a segment of the path if it matches a given string. It is almost
always used with [`</>`](#</>) or [`oneOf`](#oneOf). For example:

    blog : Parser (Int -> a) a
    blog =
      s "blog" </> int

    -- /blog/42  ==>  Just 42
    -- /tree/42  ==>  Nothing

The path segment must be an exact match!
-}
s : String -> Parser a a
s str =
  Parser <| \{ visited, unvisited, params, frag, value } ->
    case unvisited of
      [] ->
        []

      next :: rest ->
        if next == str then
          [ State (next :: visited) rest params frag value ]

        else
          []


{-| Create a custom path segment parser. Here is how it is used to define the
`int` parser:

    int : Parser (Int -> a) a
    int =
      custom "NUMBER" String.toInt

You can use it to define something like “only CSS files” like this:

    css : Parser (String -> a) a
    css =
      custom "CSS_FILE" <| \segment ->
        if String.endsWith ".css" segment then
          Just segment
        else
          Nothing
-}
custom : String -> (String -> Maybe a) -> Parser (a -> b) b
custom tipe stringToSomething =
  Parser <| \{ visited, unvisited, params, frag, value } ->
    case unvisited of
      [] ->
        []

      next :: rest ->
        case stringToSomething next of
          Just nextValue ->
            [ State (next :: visited) rest params frag (value nextValue) ]

          Nothing ->
            []



-- COMBINING PARSERS


{-| Parse a path with multiple segments.

    blog : Parser (Int -> a) a
    blog =
      s "blog" </> int

    -- /blog/35/  ==>  Just 35
    -- /blog/42   ==>  Just 42
    -- /blog/     ==>  Nothing
    -- /42/       ==>  Nothing

    search : Parser (String -> a) a
    search =
      s "search" </> string

    -- /search/wolf/  ==>  Just "wolf"
    -- /search/frog   ==>  Just "frog"
    -- /search/       ==>  Nothing
    -- /wolf/         ==>  Nothing
-}
slash : Parser a b -> Parser b c -> Parser a c
slash (Parser parseBefore) (Parser parseAfter) =
  Parser <| \state ->
    List.concatMap parseAfter (parseBefore state)


{-| Transform a path parser.

    type alias Comment = { user : String, id : Int }

    userAndId : Parser (String -> Int -> a) a
    userAndId =
      s "user" </> string </> s "comment" </> int

    comment : Parser (Comment -> a) a
    comment =
      map Comment userAndId

    -- /user/bob/comment/42  ==>  Just { user = "bob", id = 42 }
    -- /user/tom/comment/35  ==>  Just { user = "tom", id = 35 }
    -- /user/sam/             ==>  Nothing
-}
map : a -> Parser a b -> Parser (b -> c) c
map subValue (Parser parseArg) =
  Parser <| \{ visited, unvisited, params, frag, value } ->
    List.map (mapState value) <| parseArg <|
      State visited unvisited params frag subValue


mapState : (a -> b) -> State a -> State b
mapState func { visited, unvisited, params, frag, value } =
  State visited unvisited params frag (func value)


{-| Try a bunch of different path parsers.

    type Route
      = Topic String
      | Blog Int
      | User String
      | Comment String Int

    route : Parser (Route -> a) a
    route =
      oneOf
        [ map Topic   (s "topic" </> string)
        , map Blog    (s "blog" </> int)
        , map User    (s "user" </> string)
        , map Comment (s "user" </> string </> s "comment" </> int)
        ]

    -- /topic/wolf           ==>  Just (Topic "wolf")
    -- /topic/               ==>  Nothing

    -- /blog/42               ==>  Just (Blog 42)
    -- /blog/wolf             ==>  Nothing

    -- /user/sam/             ==>  Just (User "sam")
    -- /user/bob/comment/42  ==>  Just (Comment "bob" 42)
    -- /user/tom/comment/35  ==>  Just (Comment "tom" 35)
    -- /user/                 ==>  Nothing

If there are multiple parsers that could succeed, the first one wins.
-}
oneOf : List (Parser a b) -> Parser a b
oneOf parsers =
  Parser <| \state ->
    List.concatMap (\(Parser parser) -> parser state) parsers


{-| A parser that does not consume any path segments.

    type Route = Overview | Post Int

    blog : Parser (BlogRoute -> a) a
    blog =
      s "blog" </>
        oneOf
          [ map Overview top
          , map Post (s "post" </> int)
          ]

    -- /blog/         ==>  Just Overview
    -- /blog/post/42  ==>  Just (Post 42)
-}
top : Parser a a
top =
  Parser <| \state -> [state]



-- QUERY


{-| The [`Url.Parser.Query`](Url-Parser-Query) module defines its own
[`Parser`](Url-Parser-Query#Parser) type. This function helps you use those
with normal parsers. For example, maybe you want to add a search feature to
your blog website:

    import Url.Parser.Query as Query

    type Route
      = Overview (Maybe String)
      | Post Int

    blog : Parser (Route -> a) a
    blog =
      oneOf
        [ map Overview (s "blog" <?> Query.string "q")
        , map Post (s "blog" </> int)
        ]

    -- /blog/           ==>  Just (Overview Nothing)
    -- /blog/?q=wolf    ==>  Just (Overview (Just "wolf"))
    -- /blog/wolf       ==>  Nothing
    -- /blog/42         ==>  Just (Post 42)
    -- /blog/42?q=wolf  ==>  Just (Post 42)
    -- /blog/42/wolf    ==>  Nothing
-}
questionMark : Parser a (query -> b) -> Query.Parser query -> Parser a b
questionMark parser queryParser =
  slash parser (query queryParser)


{-| The [`Url.Parser.Query`](Url-Parser-Query) module defines its own
[`Parser`](Url-Parser-Query#Parser) type. This function is a helper to convert
those into normal parsers.

    import Url.Parser.Query as Query

    -- the following expressions are both the same!
    --
    -- s "blog" <?> Query.string "search"
    -- s "blog" </> query (Query.string "search")

This may be handy if you need query parameters but are not parsing any path
segments.
-}
query : Query.Parser query -> Parser (query -> a) a
query (Q.Parser queryParser) =
  Parser <| \{ visited, unvisited, params, frag, value } ->
    [ State visited unvisited params frag (value (queryParser params))
    ]



-- FRAGMENT


{-| Create a parser for the URL fragment, the stuff after the `#`. This can
be handy for handling links to DOM elements within a page. Pages like this one!

    type alias Docs =
      (String, Maybe String)

    docs : Parser (Docs -> a) a
    docs =
      map Tuple.pair (string </> fragment identity)

    -- /List/map   ==>  Nothing
    -- /List/#map  ==>  Just ("List", Just "map")
    -- /List#map   ==>  Just ("List", Just "map")
    -- /List#      ==>  Just ("List", Just "")
    -- /List       ==>  Just ("List", Nothing)
    -- /           ==>  Nothing

-}
fragment : (Maybe String -> fragment) -> Parser (fragment -> a) a
fragment toFrag =
  Parser <| \{ visited, unvisited, params, frag, value } ->
    [ State visited unvisited params frag (value (toFrag frag))
    ]



-- PARSE


{-| Actually run a parser! You provide some [`Url`](Url#Url) that
represent a valid URL. From there `parse` runs your parser on the path, query
parameters, and fragment.

    import Url
    import Url.Parser exposing (Parser, parse, int, map, oneOf, s, top)

    type Route = Home | Blog Int | NotFound

    route : Parser (Route -> a) a
    route =
      oneOf
        [ map Home top
        , map Blog (s "blog" </> int)
        ]

    toRoute : String -> Route
    toRoute string =
      case Url.fromString string of
        Nothing ->
          NotFound

        Just url ->
          Maybe.withDefault NotFound (parse route url)

    -- toRoute "/blog/42"                            ==  NotFound
    -- toRoute "https://example.com/"                ==  Home
    -- toRoute "https://example.com/blog"            ==  NotFound
    -- toRoute "https://example.com/blog/42"         ==  Blog 42
    -- toRoute "https://example.com/blog/42/"        ==  Blog 42
    -- toRoute "https://example.com/blog/42#wolf"    ==  Blog 42
    -- toRoute "https://example.com/blog/42?q=wolf"  ==  Blog 42
    -- toRoute "https://example.com/settings"        ==  NotFound

Functions like `toRoute` are useful when creating single-page apps with
[`Browser.fullscreen`][fs]. I use them in `init` and `onNavigation` to handle
the initial URL and any changes.

[fs]: /packages/elm/browser/latest/Browser#fullscreen
-}
parse : Parser (a -> a) a -> Url -> Maybe a
parse (Parser parser) url =
  getFirstMatch <| parser <|
    State [] (preparePath url.path) (prepareQuery url.query) url.fragment identity


getFirstMatch : List (State a) -> Maybe a
getFirstMatch states =
  case states of
    [] ->
      Nothing

    state :: rest ->
      case state.unvisited of
        [] ->
          Just state.value

        [""] ->
          Just state.value

        _ ->
          getFirstMatch rest



-- PREPARE PATH


preparePath : String -> List String
preparePath path =
  case String.split "/" path of
    "" :: segments ->
      removeFinalEmpty segments

    segments ->
      removeFinalEmpty segments


removeFinalEmpty : List String -> List String
removeFinalEmpty segments =
  case segments of
    [] ->
      []

    "" :: [] ->
      []

    segment :: rest ->
      segment :: removeFinalEmpty rest



-- PREPARE QUERY


prepareQuery : Maybe String -> Dict String (List String)
prepareQuery maybeQuery =
  case maybeQuery of
    Nothing ->
      Dict.empty

    Just qry ->
      List.foldr addParam Dict.empty (String.split "&" qry)


addParam : String -> Dict String (List String) -> Dict String (List String)
addParam segment dict =
  case String.split "=" segment of
    [rawKey, rawValue] ->
      case Url.percentDecode rawKey of
        Nothing ->
          dict

        Just key ->
          case Url.percentDecode rawValue of
            Nothing ->
              dict

            Just value ->
              Dict.update key (addToParametersHelp value) dict

    _ ->
      dict


addToParametersHelp : a -> Maybe (List a) -> Maybe (List a)
addToParametersHelp value maybeList =
  case maybeList of
    Nothing ->
      Just [value]

    Just list ->
      Just (value :: list)
