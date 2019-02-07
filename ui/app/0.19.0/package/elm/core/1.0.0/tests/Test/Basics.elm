module Test.Basics exposing (tests)

import Array
import Tuple exposing (first, second)
import Basics exposing (..)
import Date
import Set
import Dict
import Test exposing (..)
import Expect
import List
import String


tests : Test
tests =
    let
        comparison =
            describe "Comparison"
                [ test "max" <| \() -> Expect.equal 42 (max 32 42)
                , test "min" <| \() -> Expect.equal 42 (min 91 42)
                , test "clamp low" <| \() -> Expect.equal 10 (clamp 10 20 5)
                , test "clamp mid" <| \() -> Expect.equal 15 (clamp 10 20 15)
                , test "clamp high" <| \() -> Expect.equal 20 (clamp 10 20 25)
                , test "5 < 6" <| \() -> Expect.equal True (5 < 6)
                , test "6 < 5" <| \() -> Expect.equal False (6 < 5)
                , test "6 < 6" <| \() -> Expect.equal False (6 < 6)
                , test "5 > 6" <| \() -> Expect.equal False (5 > 6)
                , test "6 > 5" <| \() -> Expect.equal True (6 > 5)
                , test "6 > 6" <| \() -> Expect.equal False (6 > 6)
                , test "5 <= 6" <| \() -> Expect.equal True (5 <= 6)
                , test "6 <= 5" <| \() -> Expect.equal False (6 <= 5)
                , test "6 <= 6" <| \() -> Expect.equal True (6 <= 6)
                , test "compare \"A\" \"B\"" <| \() -> Expect.equal LT (compare "A" "B")
                , test "compare 'f' 'f'" <| \() -> Expect.equal EQ (compare 'f' 'f')
                , test "compare (1, 2, 3, 4, 5, 6) (0, 1, 2, 3, 4, 5)" <| \() -> Expect.equal GT (compare ( 1, 2, 3, 4, 5, 6 ) ( 0, 1, 2, 3, 4, 5 ))
                , test "compare ['a'] ['b']" <| \() -> Expect.equal LT (compare [ 'a' ] [ 'b' ])
                , test "array equality" <| \() -> Expect.equal (Array.fromList [ 1, 1, 1, 1 ]) (Array.repeat 4 1)
                , test "set equality" <| \() -> Expect.equal (Set.fromList [ 1, 2 ]) (Set.fromList [ 2, 1 ])
                , test "dict equality" <| \() -> Expect.equal (Dict.fromList [ ( 1, 1 ), ( 2, 2 ) ]) (Dict.fromList [ ( 2, 2 ), ( 1, 1 ) ])
                , test "char equality" <| \() -> Expect.notEqual '0' '饑'
                , test "date equality" <| \() -> Expect.equal (Date.fromString "2/7/1992") (Date.fromString "2/7/1992")
                , test "date equality" <| \() -> Expect.notEqual (Date.fromString "11/16/1995") (Date.fromString "2/7/1992")
                ]

        toStringTests =
            describe "toString Tests"
                [ test "toString Int" <| \() -> Expect.equal "42" (toString 42)
                , test "toString Float" <| \() -> Expect.equal "42.52" (toString 42.52)
                , test "toString Char" <| \() -> Expect.equal "'c'" (toString 'c')
                , test "toString Char single quote" <| \() -> Expect.equal "'\\''" (toString '\'')
                , test "toString Char double quote" <| \() -> Expect.equal "'\"'" (toString '"')
                , test "toString String single quote" <| \() -> Expect.equal "\"not 'escaped'\"" (toString "not 'escaped'")
                , test "toString String double quote" <| \() -> Expect.equal "\"are \\\"escaped\\\"\"" (toString "are \"escaped\"")
                , test "toString record" <| \() -> Expect.equal "{ field = [0] }" (toString { field = [ 0 ] })
                  -- TODO
                  --, test "toString record, special case" <| \() -> Expect.equal "{ ctor = [0] }" (toString { ctor = [ 0 ] })
                ]

        trigTests =
            describe "Trigonometry Tests"
                [ test "radians 0" <| \() -> Expect.equal 0 (radians 0)
                , test "radians positive" <| \() -> Expect.equal 5 (radians 5)
                , test "radians negative" <| \() -> Expect.equal -5 (radians -5)
                , test "degrees 0" <| \() -> Expect.equal 0 (degrees 0)
                , test "degrees 90" <| \() -> Expect.lessThan 0.01 (abs (1.57 - degrees 90))
                  -- This should test to enough precision to know if anything's breaking
                , test "degrees -145" <| \() -> Expect.lessThan 0.01 (abs (-2.53 - degrees -145))
                  -- This should test to enough precision to know if anything's breaking
                , test "turns 0" <| \() -> Expect.equal 0 (turns 0)
                , test "turns 8" <| \() -> Expect.lessThan 0.01 (abs (50.26 - turns 8))
                  -- This should test to enough precision to know if anything's breaking
                , test "turns -133" <| \() -> Expect.lessThan 0.01 (abs (-835.66 - turns -133))
                  -- This should test to enough precision to know if anything's breaking
                , test "fromPolar (0, 0)" <| \() -> Expect.equal ( 0, 0 ) (fromPolar ( 0, 0 ))
                , test "fromPolar (1, 0)" <| \() -> Expect.equal ( 1, 0 ) (fromPolar ( 1, 0 ))
                , test "fromPolar (0, 1)" <| \() -> Expect.equal ( 0, 0 ) (fromPolar ( 0, 1 ))
                , test "fromPolar (1, 1)" <|
                    \() ->
                        Expect.equal True
                            (let
                                ( x, y ) =
                                    fromPolar ( 1, 1 )
                             in
                                0.54 - x < 0.01 && 0.84 - y < 0.01
                            )
                , test "toPolar (0, 0)" <| \() -> Expect.equal ( 0, 0 ) (toPolar ( 0, 0 ))
                , test "toPolar (1, 0)" <| \() -> Expect.equal ( 1, 0 ) (toPolar ( 1, 0 ))
                , test "toPolar (0, 1)" <|
                    \() ->
                        Expect.equal True
                            (let
                                ( r, theta ) =
                                    toPolar ( 0, 1 )
                             in
                                r == 1 && abs (1.57 - theta) < 0.01
                            )
                , test "toPolar (1, 1)" <|
                    \() ->
                        Expect.equal True
                            (let
                                ( r, theta ) =
                                    toPolar ( 1, 1 )
                             in
                                abs (1.41 - r) < 0.01 && abs (0.78 - theta) < 0.01
                            )
                , test "cos" <| \() -> Expect.equal 1 (cos 0)
                , test "sin" <| \() -> Expect.equal 0 (sin 0)
                , test "tan" <| \() -> Expect.lessThan 0.01 (abs (12.67 - tan 17.2))
                , test "acos" <| \() -> Expect.lessThan 0.01 (abs (3.14 - acos -1))
                , test "asin" <| \() -> Expect.lessThan 0.01 (abs (0.3 - asin 0.3))
                , test "atan" <| \() -> Expect.lessThan 0.01 (abs (1.57 - atan 4567.8))
                , test "atan2" <| \() -> Expect.lessThan 0.01 (abs (1.55 - atan2 36 0.65))
                , test "pi" <| \() -> Expect.lessThan 0.01 (abs (3.14 - pi))
                ]

        basicMathTests =
            describe "Basic Math Tests"
                [ test "add float" <| \() -> Expect.equal 159 (155.6 + 3.4)
                , test "add int" <| \() -> Expect.equal 17 ((round 10) + (round 7))
                , test "subtract float" <| \() -> Expect.equal -6.3 (1 - 7.3)
                , test "subtract int" <| \() -> Expect.equal 1130 ((round 9432) - (round 8302))
                , test "multiply float" <| \() -> Expect.equal 432 (96 * 4.5)
                , test "multiply int" <| \() -> Expect.equal 90 ((round 10) * (round 9))
                , test "divide float" <| \() -> Expect.equal 13.175 (527 / 40)
                , test "divide int" <| \() -> Expect.equal 23 (70 // 3)
                , test "2 |> rem 7" <| \() -> Expect.equal 1 (2 |> rem 7)
                , test "4 |> rem -1" <| \() -> Expect.equal -1 (4 |> rem -1)
                , test "7 % 2" <| \() -> Expect.equal 1 (7 % 2)
                , test "-1 % 4" <| \() -> Expect.equal 3 (-1 % 4)
                , test "3^2" <| \() -> Expect.equal 9 (3 ^ 2)
                , test "sqrt" <| \() -> Expect.equal 9 (sqrt 81)
                , test "negate 42" <| \() -> Expect.equal -42 (negate 42)
                , test "negate -42" <| \() -> Expect.equal 42 (negate -42)
                , test "negate 0" <| \() -> Expect.equal 0 (negate 0)
                , test "abs -25" <| \() -> Expect.equal 25 (abs -25)
                , test "abs 76" <| \() -> Expect.equal 76 (abs 76)
                , test "logBase 10 100" <| \() -> Expect.equal 2 (logBase 10 100)
                , test "logBase 2 256" <| \() -> Expect.equal 8 (logBase 2 256)
                , test "e" <| \() -> Expect.lessThan 0.01 (abs (2.72 - e))
                ]

        booleanTests =
            describe "Boolean Tests"
                [ test "False && False" <| \() -> Expect.equal False (False && False)
                , test "False && True" <| \() -> Expect.equal False (False && True)
                , test "True && False" <| \() -> Expect.equal False (True && False)
                , test "True && True" <| \() -> Expect.equal True (True && True)
                , test "False || False" <| \() -> Expect.equal False (False || False)
                , test "False || True" <| \() -> Expect.equal True (False || True)
                , test "True || False" <| \() -> Expect.equal True (True || False)
                , test "True || True" <| \() -> Expect.equal True (True || True)
                , test "xor False False" <| \() -> Expect.equal False (xor False False)
                , test "xor False True" <| \() -> Expect.equal True (xor False True)
                , test "xor True False" <| \() -> Expect.equal True (xor True False)
                , test "xor True True" <| \() -> Expect.equal False (xor True True)
                , test "not True" <| \() -> Expect.equal False (not True)
                , test "not False" <| \() -> Expect.equal True (not False)
                ]

        conversionTests =
            describe "Conversion Tests"
                [ test "round 0.6" <| \() -> Expect.equal 1 (round 0.6)
                , test "round 0.4" <| \() -> Expect.equal 0 (round 0.4)
                , test "round 0.5" <| \() -> Expect.equal 1 (round 0.5)
                , test "truncate -2367.9267" <| \() -> Expect.equal -2367 (truncate -2367.9267)
                , test "floor -2367.9267" <| \() -> Expect.equal -2368 (floor -2367.9267)
                , test "ceiling 37.2" <| \() -> Expect.equal 38 (ceiling 37.2)
                , test "toFloat 25" <| \() -> Expect.equal 25 (toFloat 25)
                ]

        miscTests =
            describe "Miscellaneous Tests"
                [ test "isNaN (0/0)" <| \() -> Expect.equal True (isNaN (0 / 0))
                , test "isNaN (sqrt -1)" <| \() -> Expect.equal True (isNaN (sqrt -1))
                , test "isNaN (1/0)" <| \() -> Expect.equal False (isNaN (1 / 0))
                , test "isNaN 1" <| \() -> Expect.equal False (isNaN 1)
                , test "isInfinite (0/0)" <| \() -> Expect.equal False (isInfinite (0 / 0))
                , test "isInfinite (sqrt -1)" <| \() -> Expect.equal False (isInfinite (sqrt -1))
                , test "isInfinite (1/0)" <| \() -> Expect.equal True (isInfinite (1 / 0))
                , test "isInfinite 1" <| \() -> Expect.equal False (isInfinite 1)
                , test "\"hello\" ++ \"world\"" <| \() -> Expect.equal "helloworld" ("hello" ++ "world")
                , test "[1, 1, 2] ++ [3, 5, 8]" <| \() -> Expect.equal [ 1, 1, 2, 3, 5, 8 ] ([ 1, 1, 2 ] ++ [ 3, 5, 8 ])
                , test "first (1, 2)" <| \() -> Expect.equal 1 (first ( 1, 2 ))
                , test "second (1, 2)" <| \() -> Expect.equal 2 (second ( 1, 2 ))
                ]

        higherOrderTests =
            describe "Higher Order Helpers"
                [ test "identity 'c'" <| \() -> Expect.equal 'c' (identity 'c')
                , test "always 42 ()" <| \() -> Expect.equal 42 (always 42 ())
                , test "<|" <| \() -> Expect.equal 9 (identity <| 3 + 6)
                , test "|>" <| \() -> Expect.equal 9 (3 + 6 |> identity)
                , test "<<" <| \() -> Expect.equal True (not << xor True <| True)
                , test "<<" <| \() -> Expect.equal True (not << xor True <| True)
                , describe ">>"
                    [ test "with xor" <|
                        \() ->
                            (True |> xor True >> not)
                                |> Expect.equal True
                    , test "with a record accessor" <|
                        \() ->
                            [ { foo = "NaS", bar = "baz" } ]
                                |> List.map (.foo >> String.reverse)
                                |> Expect.equal [ "SaN" ]
                    ]
                , test "flip" <| \() -> Expect.equal 10 ((flip (//)) 2 20)
                , test "curry" <| \() -> Expect.equal 1 ((curry (\( a, b ) -> a + b)) -5 6)
                , test "uncurry" <| \() -> Expect.equal 1 ((uncurry (+)) ( -5, 6 ))
                ]
    in
        describe "Basics"
            [ comparison
            , toStringTests
            , trigTests
            , basicMathTests
            , booleanTests
            , miscTests
            , higherOrderTests
            ]
