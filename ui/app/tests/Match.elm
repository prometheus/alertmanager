module Match exposing (testConsecutiveChars, testJaroWinkler)

import Expect
import Test exposing (..)
import Utils.Match exposing (consecutiveChars, jaroWinkler)


testJaroWinkler : Test
testJaroWinkler =
    describe "jaroWinkler"
        [ test "should find the right values 1" <|
            \() ->
                Expect.greaterThan (jaroWinkler "zi" "zone")
                    (jaroWinkler "zo" "zone")
        , test "should find the right values 2" <|
            \() ->
                Expect.greaterThan (jaroWinkler "hook" "alertname")
                    (jaroWinkler "de" "dev")
        , test "should find the right values 3" <|
            \() ->
                Expect.equal 0.0
                    (jaroWinkler "l" "zone")
        , test "should find the right values 4" <|
            \() ->
                Expect.equal 1.0
                    (jaroWinkler "zone" "zone")
        , test "should find the right values 5" <|
            \() ->
                Expect.greaterThan 0.688
                    (jaroWinkler "atleio3tefdoisahdf" "attributefdoiashfoihfeowfh9w8f9afaw9fahw")
        ]


testConsecutiveChars : Test
testConsecutiveChars =
    describe "consecutiveChars"
        [ test "should find the consecutiveChars 1" <|
            \() ->
                Expect.equal "zo"
                    (consecutiveChars "zo" "bozo")
        , test "should find the consecutiveChars 2" <|
            \() ->
                Expect.equal "zo"
                    (consecutiveChars "zol" "zone")
        , test "should find the consecutiveChars 3" <|
            \() ->
                Expect.equal "oon"
                    (consecutiveChars "oon" "baboone")
        , test "should find the consecutiveChars 4" <|
            \() ->
                Expect.equal "dom"
                    (consecutiveChars "dom" "random")
        ]
