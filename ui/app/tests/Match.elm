module Match exposing (all)

import Test exposing (..)
import Expect
import Views.AutoComplete.Match exposing (jaroWinkler, commonPrefix)


all : Test
all =
    describe "Filter"
        [ testJaroWinkler
        , testCommonPrefix
        ]


testJaroWinkler : Test
testJaroWinkler =
    describe "jaroWinkler"
        [ test "should find the right values" <|
            \() ->
                Expect.greaterThan (jaroWinkler "z" "zone")
                    (jaroWinkler "zo" "zone")
        , test "should find the right values" <|
            \() ->
                Expect.equal 0.0
                    (jaroWinkler "l" "zone")
        , test "should find the right values" <|
            \() ->
                Expect.equal 1.0
                    (jaroWinkler "zone" "zone")
        ]


testCommonPrefix : Test
testCommonPrefix =
    describe "commonPrefix"
        [ test "should find the commonPrefix" <|
            \() ->
                Expect.equal "zo"
                    (commonPrefix "zo" "zone")
        , test "should find the commonPrefix" <|
            \() ->
                Expect.equal "zo"
                    (commonPrefix "zol" "zone")
        , test "should find the commonPrefix" <|
            \() ->
                Expect.equal ""
                    (commonPrefix "oon" "zone")
        , test "should find the commonPrefix" <|
            \() ->
                Expect.equal "zone"
                    (commonPrefix "zone123" "zone123")
        ]
