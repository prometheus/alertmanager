module Match exposing (..)

import Test exposing (..)
import Expect
import Utils.Match exposing (jaroWinkler, commonPrefix)


testJaroWinkler : Test
testJaroWinkler =
    describe "jaroWinkler"
        [ test "should find the right values 1" <|
            \() ->
                Expect.greaterThan (jaroWinkler "zi" "zone")
                    (jaroWinkler "zo" "zone")
        , test "should find the right values 2" <|
            \() ->
                Expect.greaterThan (jaroWinkler "de" "alertname")
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


testCommonPrefix : Test
testCommonPrefix =
    describe "commonPrefix"
        [ test "should find the commonPrefix 1" <|
            \() ->
                Expect.equal "zo"
                    (commonPrefix "zo" "zone")
        , test "should find the commonPrefix 2" <|
            \() ->
                Expect.equal "zo"
                    (commonPrefix "zol" "zone")
        , test "should find the commonPrefix 3" <|
            \() ->
                Expect.equal ""
                    (commonPrefix "oon" "zone")
        , test "should find the commonPrefix 4" <|
            \() ->
                Expect.equal "zone"
                    (commonPrefix "zone123" "zone123")
        ]
