module Url.Builder exposing
  ( absolute, relative, crossOrigin, custom, Root(..)
  , QueryParameter, string, int, toQuery
  )


{-| In [the URI spec](https://tools.ietf.org/html/rfc3986), Tim Berners-Lee
says a URL looks like this:

```
  https://example.com:8042/over/there?name=ferret#nose
  \___/   \______________/\_________/ \_________/ \__/
    |            |            |            |        |
  scheme     authority       path        query   fragment
```

This module helps you create these!


# Builders
@docs absolute, relative, crossOrigin, custom, Root

# Queries
@docs QueryParameter, string, int, toQuery

-}


import Url



-- BUILDERS


{-| Create an absolute URL:

    absolute [] []
    -- "/"

    absolute [ "packages", "elm", "core" ] []
    -- "/packages/elm/core"

    absolute [ "blog", String.fromInt 42 ] []
    -- "/blog/42"

    absolute [ "products" ] [ string "search" "hat", int "page" 2 ]
    -- "/products?search=hat&page=2"

Notice that the URLs start with a slash!
-}
absolute : List String -> List QueryParameter -> String
absolute pathSegments parameters =
  "/" ++ String.join "/" pathSegments ++ toQuery parameters


{-| Create a relative URL:

    relative [] []
    -- ""

    relative [ "elm", "core" ] []
    -- "elm/core"

    relative [ "blog", String.fromInt 42 ] []
    -- "blog/42"

    relative [ "products" ] [ string "search" "hat", int "page" 2 ]
    -- "products?search=hat&page=2"

Notice that the URLs **do not** start with a slash!
-}
relative : List String -> List QueryParameter -> String
relative pathSegments parameters =
  String.join "/" pathSegments ++ toQuery parameters


{-| Create a cross-origin URL.

    crossOrigin "https://example.com" [ "products" ] []
    -- "https://example.com/products"

    crossOrigin "https://example.com" [] []
    -- "https://example.com/"

    crossOrigin
      "https://example.com:8042"
      [ "over", "there" ]
      [ string "name" "ferret" ]
    -- "https://example.com:8042/over/there?name=ferret"

**Note:** Cross-origin requests are slightly restricted for security.
For example, the [same-origin policy][sop] applies when sending HTTP requests,
so the appropriate `Access-Control-Allow-Origin` header must be enabled on the
*server* to get things working. Read more about the security rules [here][cors].

[sop]: https://developer.mozilla.org/en-US/docs/Web/Security/Same-origin_policy
[cors]: https://developer.mozilla.org/en-US/docs/Web/HTTP/Access_control_CORS
-}
crossOrigin : String -> List String -> List QueryParameter -> String
crossOrigin prePath pathSegments parameters =
  prePath ++ "/" ++ String.join "/" pathSegments ++ toQuery parameters



-- CUSTOM BUILDER


{-| Specify whether a [`custom`](#custom) URL is absolute, relative, or
cross-origin.
-}
type Root = Absolute | Relative | CrossOrigin String


{-| Create custom URLs that may have a hash on the end:

    custom Absolute
      [ "packages", "elm", "core", "latest", "String" ]
      []
      (Just "length")
    -- "/packages/elm/core/latest/String#length"

    custom Relative [ "there" ] [ string "name" "ferret" ] Nothing
    -- "there?name=ferret"

    custom
      (CrossOrigin "https://example.com:8042")
      [ "over", "there" ]
      [ string "name" "ferret" ]
      (Just "nose")
    -- "https://example.com:8042/over/there?name=ferret#nose"
-}
custom : Root -> List String -> List QueryParameter -> Maybe String -> String
custom root pathSegments parameters maybeFragment =
  let
    fragmentless =
      rootToPrePath root ++ String.join "/" pathSegments ++ toQuery parameters
  in
  case maybeFragment of
    Nothing ->
      fragmentless

    Just fragment ->
      fragmentless ++ "#" ++ fragment


rootToPrePath : Root -> String
rootToPrePath root =
  case root of
    Absolute ->
      "/"

    Relative ->
      ""

    CrossOrigin prePath ->
      prePath ++ "/"



-- QUERY PARAMETERS


{-| Represents query parameter. Builder functions like `absolute` percent-encode
all the query parameters they get, so you do not need to worry about it!
-}
type QueryParameter =
  QueryParameter String String


{-| Create a percent-encoded query parameter.

    absolute ["products"] [ string "search" "hat" ]
    -- "/products?search=hat"

    absolute ["products"] [ string "search" "coffee table" ]
    -- "/products?search=coffee%20table"
-}
string : String -> String -> QueryParameter
string key value =
  QueryParameter (Url.percentEncode key) (Url.percentEncode value)


{-| Create a percent-encoded query parameter.

    absolute ["products"] [ string "search" "hat", int "page" 2 ]
    -- "/products?search=hat&page=2"

Writing `int key n` is the same as writing `string key (String.fromInt n)`.
So this is just a convenience function, making your code a bit shorter!
-}
int : String -> Int -> QueryParameter
int key value =
  QueryParameter (Url.percentEncode key) (String.fromInt value)


{-| Convert a list of query parameters to a percent-encoded query. This
function is used by `absolute`, `relative`, etc.

    toQuery [ string "search" "hat" ]
    -- "?search=hat"

    toQuery [ string "search" "coffee table" ]
    -- "?search=coffee%20table"

    toQuery [ string "search" "hat", int "page" 2 ]
    -- "?search=hat&page=2"

    toQuery []
    -- ""
-}
toQuery : List QueryParameter -> String
toQuery parameters =
  case parameters of
    [] ->
      ""

    _ ->
      "?" ++ String.join "&" (List.map toQueryPair parameters)


toQueryPair : QueryParameter -> String
toQueryPair (QueryParameter key value) =
  key ++ "=" ++ value
