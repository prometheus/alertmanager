module Url exposing
  ( Url
  , Protocol(..)
  , toString
  , fromString
  , percentEncode
  , percentDecode
  )


{-|

# URLs
@docs Url, Protocol, toString, fromString

# Percent-Encoding
@docs percentEncode, percentDecode

-}


import Elm.Kernel.Url



-- URL


{-| In [the URI spec](https://tools.ietf.org/html/rfc3986), Tim Berners-Lee
says a URL looks like this:

```
  https://example.com:8042/over/there?name=ferret#nose
  \___/   \______________/\_________/ \_________/ \__/
    |            |            |            |        |
  scheme     authority       path        query   fragment
```

When you are creating a single-page app with [`Browser.fullscreen`][fs], you
use the [`Url.Parser`](Url-Parser) module to turn a `Url` into even nicer data.

If you want to create your own URLs, check out the [`Url.Builder`](Url-Builder)
module as well!

[fs]: /packages/elm/browser/latest/Browser#fullscreen

**Note:** This is a subset of all the full possibilities listed in the URI
spec. Specifically, it does not accept the `userinfo` segment you see in email
addresses like `tom@example.com`.
-}
type alias Url =
  { protocol : Protocol
  , host : String
  , port_ : Maybe Int
  , path : String
  , query : Maybe String
  , fragment : Maybe String
  }


{-| Is the URL served over a secure connection or not?
-}
type Protocol = Http | Https


{-| Attempt to break a URL up into [`Url`](#Url). This is useful in
single-page apps when you want to parse certain chunks of a URL to figure out
what to show on screen.

    fromString "https://example.com:443"
    -- Just
    --   { protocol = Https
    --   , host = "example.com"
    --   , port_ = Just 443
    --   , path = "/"
    --   , query = Nothing
    --   , fragment = Nothing
    --   }

    fromString "https://example.com/hats?q=top%20hat"
    -- Just
    --   { protocol = Https
    --   , host = "example.com"
    --   , port_ = Nothing
    --   , path = "/hats"
    --   , query = Just "q=top%20hat"
    --   , fragment = Nothing
    --   }

    fromString "http://example.com/core/List/#map"
    -- Just
    --   { protocol = Http
    --   , host = "example.com"
    --   , port_ = Nothing
    --   , path = "/core/List/"
    --   , query = Nothing
    --   , fragment = Just "map"
    --   }

The conversion to segments can fail in some cases as well:

    fromString "example.com:443"        == Nothing  -- no protocol
    fromString "http://tom@example.com" == Nothing  -- userinfo disallowed
    fromString "http://#cats"           == Nothing  -- no host

**Note:** This function does not use [`percentDecode`](#percentDecode) anything.
It just splits things up. [`Url.Parser`](Url-Parser) actually _needs_ the raw
`query` string to parse it properly. Otherwise it could get confused about `=`
and `&` characters!
-}
fromString : String -> Maybe Url
fromString str =
  if String.startsWith "http://" str then
    chompAfterProtocol Http (String.dropLeft 7 str)

  else if String.startsWith "https://" str then
    chompAfterProtocol Https (String.dropLeft 8 str)

  else
    Nothing


chompAfterProtocol : Protocol -> String -> Maybe Url
chompAfterProtocol protocol str =
  if String.isEmpty str then
    Nothing
  else
    case String.indexes "#" str of
      [] ->
        chompBeforeFragment protocol Nothing str

      i :: _ ->
        chompBeforeFragment protocol (Just (String.dropLeft (i + 1) str)) (String.left i str)


chompBeforeFragment : Protocol -> Maybe String -> String -> Maybe Url
chompBeforeFragment protocol frag str =
  if String.isEmpty str then
    Nothing
  else
    case String.indexes "?" str of
      [] ->
        chompBeforeQuery protocol Nothing frag str

      i :: _ ->
        chompBeforeQuery protocol (Just (String.dropLeft (i + 1) str)) frag (String.left i str)


chompBeforeQuery : Protocol -> Maybe String -> Maybe String -> String -> Maybe Url
chompBeforeQuery protocol params frag str =
  if String.isEmpty str then
    Nothing
  else
    case String.indexes "/" str of
      [] ->
        chompBeforePath protocol "/" params frag str

      i :: _ ->
        chompBeforePath protocol (String.dropLeft i str) params frag (String.left i str)


chompBeforePath : Protocol -> String -> Maybe String -> Maybe String -> String -> Maybe Url
chompBeforePath protocol path params frag str =
  if String.isEmpty str || String.contains "@" str then
    Nothing
  else
    case String.indexes ":" str of
      [] ->
        Just <| Url protocol str Nothing path params frag

      i :: [] ->
        case String.toInt (String.dropLeft (i + 1) str) of
          Nothing ->
            Nothing

          port_ ->
            Just <| Url protocol (String.left i str) port_ path params frag

      _ ->
        Nothing


{-| Turn a [`Url`](#Url) into a `String`.
-}
toString : Url -> String
toString url =
  let
    http =
      case url.protocol of
        Http ->
          "http://"

        Https ->
          "https://"
  in
  addPort url.port_ (http ++ url.host) ++ url.path
    |> addPrefixed "?" url.query
    |> addPrefixed "#" url.fragment


addPort : Maybe Int -> String -> String
addPort maybePort starter =
  case maybePort of
    Nothing ->
      starter

    Just port_ ->
      starter ++ ":" ++ String.fromInt port_


addPrefixed : String -> Maybe String -> String -> String
addPrefixed prefix maybeSegment starter =
  case maybeSegment of
    Nothing ->
      starter

    Just segment ->
      starter ++ prefix ++ segment



-- PERCENT ENCODING


{-| **Use [Url.Builder](Url-Builder) instead!** Functions like `absolute`,
`relative`, and `crossOrigin` already do this automatically! `percentEncode`
is only available so that extremely custom cases are possible, if needed.

Percent-encoding is how [the official URI spec][uri] “escapes” special
characters. You can still represent a `?` even though it is reserved for
queries.

This function exists in case you want to do something extra custom. Here are
some examples:

    -- standard ASCII encoding
    percentEncode "hat"   == "hat"
    percentEncode "to be" == "to%20be"
    percentEncode "99%"   == "99%25"

    -- non-standard, but widely accepted, UTF-8 encoding
    percentEncode "$" == "%24"
    percentEncode "¢" == "%C2%A2"
    percentEncode "€" == "%E2%82%AC"

This is the same behavior as JavaScript's [`encodeURIComponent`][js] function,
and the rules are described in more detail officially [here][s2] and with some
notes about Unicode [here][wiki].

[js]: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/encodeURIComponent
[uri]: https://tools.ietf.org/html/rfc3986
[s2]: https://tools.ietf.org/html/rfc3986#section-2.1
[wiki]: https://en.wikipedia.org/wiki/Percent-encoding
-}
percentEncode : String -> String
percentEncode =
  Elm.Kernel.Url.percentEncode


{-| **Use [Url.Parser](Url-Parser) instead!** It will decode query
parameters appropriately already! `percentDecode` is only available so that
extremely custom cases are possible, if needed.

Check out the `percentEncode` function to learn about percent-encoding.
This function does the opposite! Here are the reverse examples:

    -- ASCII
    percentDecode "99%25"     == Just "hat"
    percentDecode "to%20be"   == Just "to be"
    percentDecode "hat"       == Just "99%"

    -- UTF-8
    percentDecode "%24"       == Just "$"
    percentDecode "%C2%A2"    == Just "¢"
    percentDecode "%E2%82%AC" == Just "€"

Why is it a `Maybe` though? Well, these strings come from strangers on the
internet as a bunch of bits and may have encoding problems. For example:

    percentDecode "%"   == Nothing  -- not followed by two hex digits
    percentDecode "%XY" == Nothing  -- not followed by two HEX digits
    percentDecode "%C2" == Nothing  -- half of the "¢" encoding "%C2%A2"

This is the same behavior as JavaScript's [`decodeURIComponent`][js] function.

[js]: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/decodeURIComponent
-}
percentDecode : String -> Maybe String
percentDecode =
  Elm.Kernel.Url.percentDecode
