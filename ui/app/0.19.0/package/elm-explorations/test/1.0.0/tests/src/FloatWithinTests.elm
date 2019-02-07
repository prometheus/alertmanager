module FloatWithinTests exposing (floatWithinTests)

import Expect exposing (FloatingPointTolerance(..))
import Fuzz exposing (..)
import Helpers exposing (..)
import Test exposing (..)


floatWithinTests : Test
floatWithinTests =
    describe "Expect.within"
        [ describe "use-cases"
            [ fuzz float "pythagorean identity" <|
                \x ->
                    sin x ^ 2 + cos x ^ 2 |> Expect.within (AbsoluteOrRelative 0.000001 0.00001) 1.0
            , test "floats known to not add exactly" <|
                \_ -> 0.1 + 0.2 |> Expect.within (Absolute 0.000000001) 0.3
            , test "approximation of pi" <|
                \_ -> 3.14 |> Expect.within (Absolute 0.01) pi
            , fuzz (floatRange 0.000001 100000) "relative tolerance of circle circumference using pi approximation" <|
                \radius ->
                    (radius * pi)
                        |> Expect.within (Relative 0.001) (radius * 3.14)
            , test "approximation of pi is not considered too accurate" <|
                \_ ->
                    3.14
                        |> Expect.within (Absolute 0.001) pi
                        |> expectToFail
            , test "too high absolute tolerance of circle circumference using pi approximation" <|
                \() ->
                    expectTestToFail <|
                        fuzz (floatRange 0.000001 100000) "x" <|
                            \radius ->
                                (radius * pi)
                                    |> Expect.within (Absolute 0.001) (radius * 3.14)
            , test "too high relative tolerance of circle circumference using pi approximation" <|
                \() ->
                    expectTestToFail <|
                        fuzz (floatRange 0.000001 100000) "x" <|
                            \radius ->
                                (radius * pi)
                                    |> Expect.within (Relative 0.0001) (radius * 3.14)
            ]
        , describe "use-cases with negative nominal and/or actual values"
            [ test "negative nominal and actual with Absolute" <|
                \_ -> -2.9 |> Expect.within (Absolute 0.1) -3
            , test "negative nominal and actual with Relative" <|
                \_ -> -2.9 |> Expect.within (Relative 0.1) -3
            , test "negative nominal and actual with AbsoluteOrRelative and pass on Absolute" <|
                \_ -> -2.9 |> Expect.within (AbsoluteOrRelative 0.1 0.0001) -3
            , test "negative nominal and actual with AbsoluteOrRelative and pass on Relative" <|
                \_ -> -2.9 |> Expect.within (AbsoluteOrRelative 0.001 0.05) -3
            , test "negative nominal and positive actual with Absolute" <|
                \_ -> 0.001 |> Expect.within (Absolute 3.3) -3
            , test "negative nominal and positive actual with Relative" <|
                \_ -> 0.001 |> Expect.within (Relative 1.1) -3
            , test "negative actual and positive nominal with Absolute" <|
                \_ -> -0.001 |> Expect.within (Absolute 3.3) 3
            , test "negative actual and positive nominal with Relative" <|
                \_ -> -0.001 |> Expect.within (Relative 1.1) 3
            , test "negative nominal should fail as actual is close, but positive with Absolute" <|
                \_ ->
                    2.9
                        |> Expect.within (Absolute 0.1) -3
                        |> expectToFail
            , test "negative nominal should fail as actual is close, but positive with Relative" <|
                \_ ->
                    2.9
                        |> Expect.within (Relative 0.1) -3
                        |> expectToFail
            ]
        , describe "edge-cases"
            [ fuzz2 float float "self equality" <|
                \epsilon value ->
                    let
                        eps =
                            if epsilon /= 0 then
                                epsilon

                            else
                                1
                    in
                    value |> Expect.within (Relative (abs eps)) value
            , fuzz float "NaN inequality" <|
                \epsilon ->
                    let
                        nan =
                            0.0 / 0.0
                    in
                    nan |> Expect.notWithin (Relative (abs epsilon)) nan
            , fuzz2 float float "NaN does not equal anything" <|
                \epsilon a ->
                    let
                        nan =
                            0.0 / 0.0
                    in
                    nan |> Expect.notWithin (Relative (abs epsilon)) a
            , fuzz float "Infinity equality" <|
                \epsilon ->
                    let
                        infinity =
                            1.0 / 0.0
                    in
                    infinity |> Expect.within (Relative (abs epsilon)) infinity
            , fuzz float "Negative infinity equality" <|
                \epsilon ->
                    let
                        negativeInfinity =
                            -1.0 / 0.0
                    in
                    negativeInfinity |> Expect.within (Relative (abs epsilon)) negativeInfinity
            , fuzz3 float float float "within and notWithin should never agree on relative tolerance" <|
                \epsilon a b ->
                    let
                        withinTest =
                            a |> Expect.within (Relative (abs epsilon)) b

                        notWithinTest =
                            a |> Expect.notWithin (Relative (abs epsilon)) b
                    in
                    different withinTest notWithinTest
            , fuzz3 float float float "within and notWithin should never agree on absolute tolerance" <|
                \epsilon a b ->
                    let
                        withinTest =
                            a |> Expect.within (Absolute (abs epsilon)) b

                        notWithinTest =
                            a |> Expect.notWithin (Absolute (abs epsilon)) b
                    in
                    different withinTest notWithinTest
            , fuzz2 (tuple ( float, float )) (tuple ( float, float )) "within and notWithin should never agree on absolute or relative tolerance" <|
                \( absoluteEpsilon, relativeEpsilon ) ( a, b ) ->
                    let
                        withinTest =
                            a |> Expect.within (AbsoluteOrRelative (abs absoluteEpsilon) (abs relativeEpsilon)) b

                        notWithinTest =
                            a |> Expect.notWithin (AbsoluteOrRelative (abs absoluteEpsilon) (abs relativeEpsilon)) b
                    in
                    different withinTest notWithinTest
            , fuzz float "Zero equality" <|
                \epsilon -> 0.0 |> Expect.within (Relative (abs epsilon)) 0.0
            , fuzz3 float float float "within absolute commutativity" <|
                \epsilon a b ->
                    same (Expect.within (Absolute (abs epsilon)) a b) (Expect.within (Absolute (abs epsilon)) b a)
            , fuzz3 float float float "notWithin absolute commutativity" <|
                \epsilon a b ->
                    same (Expect.notWithin (Absolute (abs epsilon)) a b) (Expect.notWithin (Absolute (abs epsilon)) b a)
            , fuzz2 float float "within absolute reflexive" <|
                \epsilon a ->
                    Expect.within (Absolute (abs epsilon)) a a
            , fuzz3 float float float "within relative commutativity" <|
                \epsilon a b ->
                    same (Expect.within (Relative (abs epsilon)) a b) (Expect.within (Relative (abs epsilon)) b a)
            , fuzz3 float float float "notWithin relative commutativity" <|
                \epsilon a b ->
                    same (Expect.notWithin (Relative (abs epsilon)) a b) (Expect.notWithin (Relative (abs epsilon)) b a)
            , fuzz2 float float "within relative reflexive" <|
                \epsilon a ->
                    Expect.within (Relative (abs epsilon)) a a
            ]
        ]
