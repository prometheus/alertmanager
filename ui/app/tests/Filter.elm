module Filter exposing (all)

import Test exposing (..)
import Expect
import Fuzz exposing (list, int, tuple, string)
import Utils.Filter exposing (Matcher, MatchOperator(Eq, RegexMatch))
import Helpers exposing (isNotEmptyTrimmedAlphabetWord)


all : Test
all =
    describe "Filter"
        [ parseMatcher
        , generateQueryString
        , stringifyFilter
        ]


parseMatcher : Test
parseMatcher =
    describe "parseMatcher"
        [ test "should parse empty matcher string" <|
            \() ->
                Expect.equal Nothing (Utils.Filter.parseMatcher "")
        , fuzz (tuple ( string, string )) "should parse random matcher string" <|
            \( key, value ) ->
                if List.map isNotEmptyTrimmedAlphabetWord [ key, value ] /= [ True, True ] then
                    Expect.equal
                        Nothing
                        (Utils.Filter.parseMatcher <| String.join "" [ key, "=", value ])
                else
                    Expect.equal
                        (Just (Matcher key Eq value))
                        (Utils.Filter.parseMatcher <| String.join "" [ key, "=", "\"", value, "\"" ])
        ]


generateQueryString : Test
generateQueryString =
    describe "generateQueryString"
        [ test "should not render keys with Nothing value" <|
            \() ->
                Expect.equal ""
                    (Utils.Filter.generateQueryString { receiver = Nothing, text = Nothing, showSilenced = Nothing })
        , test "should not render filter key with empty value" <|
            \() ->
                Expect.equal ""
                    (Utils.Filter.generateQueryString { receiver = Nothing, text = Just "", showSilenced = Nothing })
        , test "should render filter key with values" <|
            \() ->
                Expect.equal "?filter=%7Bfoo%3D%22bar%22%2C%20baz%3D~%22quux.*%22%7D"
                    (Utils.Filter.generateQueryString { receiver = Nothing, text = Just "{foo=\"bar\", baz=~\"quux.*\"}", showSilenced = Nothing })
        , test "should render silenced key with bool" <|
            \() ->
                Expect.equal "?silenced=true"
                    (Utils.Filter.generateQueryString { receiver = Nothing, text = Nothing, showSilenced = Just True })
        ]


stringifyFilter : Test
stringifyFilter =
    describe "stringifyFilter"
        [ test "empty" <|
            \() ->
                Expect.equal ""
                    (Utils.Filter.stringifyFilter [])
        , test "non-empty" <|
            \() ->
                Expect.equal "{foo=\"bar\", baz=~\"quux.*\"}"
                    (Utils.Filter.stringifyFilter
                        [ { key = "foo", op = Eq, value = "bar" }
                        , { key = "baz", op = RegexMatch, value = "quux.*" }
                        ]
                    )
        ]
