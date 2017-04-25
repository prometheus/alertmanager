module Tests exposing (..)

import Test exposing (..)
import Expect
import Fuzz exposing (list, int, tuple, string)
import Utils.Filter exposing (parseMatcher, Matcher, MatchOperator(Eq))
import Helpers exposing (isNotEmptyTrimmedAlphabetWord)


all : Test
all =
    describe "Tests"
        [ utils
        ]


utils : Test
utils =
    describe "Utils"
        [ filter
        ]


filter : Test
filter =
    describe "Filter"
        [ describe "parseMatcher"
            [ test "should parse empty matcher string" <|
                \() ->
                    Expect.equal Nothing <| parseMatcher ""
            , fuzz (tuple ( string, string )) "should parse random matcher string" <|
                \( key, value ) ->
                    if List.map isNotEmptyTrimmedAlphabetWord [ key, value ] /= [ True, True ] then
                        Expect.equal
                            Nothing
                            (parseMatcher <| String.join "" [ key, "=", value ])
                    else
                        Expect.equal
                            (Just (Matcher key Eq value))
                            (parseMatcher <| String.join "" [ key, "=", "\"", value, "\"" ])
            ]
        ]
