module Test.Bitwise exposing (tests)

import Basics exposing (..)
import Bitwise
import Test exposing (..)
import Expect


tests : Test
tests =
    describe "Bitwise"
        [ describe "and"
            [ test "and with 32 bit integers" <| \() -> Expect.equal 1 (Bitwise.and 5 3)
            , test "and with 0 as first argument" <| \() -> Expect.equal 0 (Bitwise.and 0 1450)
            , test "and with 0 as second argument" <| \() -> Expect.equal 0 (Bitwise.and 274 0)
            , test "and with -1 as first argument" <| \() -> Expect.equal 2671 (Bitwise.and -1 2671)
            , test "and with -1 as second argument" <| \() -> Expect.equal 96 (Bitwise.and 96 -1)
            ]
        , describe "or"
            [ test "or with 32 bit integers" <| \() -> Expect.equal 15 (Bitwise.or 9 14)
            , test "or with 0 as first argument" <| \() -> Expect.equal 843 (Bitwise.or 0 843)
            , test "or with 0 as second argument" <| \() -> Expect.equal 19 (Bitwise.or 19 0)
            , test "or with -1 as first argument" <| \() -> Expect.equal -1 (Bitwise.or -1 2360)
            , test "or with -1 as second argument" <| \() -> Expect.equal -1 (Bitwise.or 3 -1)
            ]
        , describe "xor"
            [ test "xor with 32 bit integers" <| \() -> Expect.equal 604 (Bitwise.xor 580 24)
            , test "xor with 0 as first argument" <| \() -> Expect.equal 56 (Bitwise.xor 0 56)
            , test "xor with 0 as second argument" <| \() -> Expect.equal -268 (Bitwise.xor -268 0)
            , test "xor with -1 as first argument" <| \() -> Expect.equal -25 (Bitwise.xor -1 24)
            , test "xor with -1 as second argument" <| \() -> Expect.equal 25601 (Bitwise.xor -25602 -1)
            ]
        , describe "complement"
            [ test "complement a positive" <| \() -> Expect.equal -9 (Bitwise.complement 8)
            , test "complement a negative" <| \() -> Expect.equal 278 (Bitwise.complement -279)
            ]
        , describe "shiftLeftBy"
            [ test "8 |> shiftLeftBy 1 == 16" <| \() -> Expect.equal 16 (8 |> Bitwise.shiftLeftBy 1)
            , test "8 |> shiftLeftby 2 == 32" <| \() -> Expect.equal 32 (8 |> Bitwise.shiftLeftBy 2)
            ]
        , describe "shiftRightBy"
            [ test "32 |> shiftRight 1 == 16" <| \() -> Expect.equal 16 (32 |> Bitwise.shiftRightBy 1)
            , test "32 |> shiftRight 2 == 8" <| \() -> Expect.equal 8 (32 |> Bitwise.shiftRightBy 2)
            , test "-32 |> shiftRight 1 == -16" <| \() -> Expect.equal -16 (-32 |> Bitwise.shiftRightBy 1)
            ]
        , describe "shiftRightZfBy"
            [ test "32 |> shiftRightZfBy 1 == 16" <| \() -> Expect.equal 16 (32 |> Bitwise.shiftRightZfBy 1)
            , test "32 |> shiftRightZfBy 2 == 8" <| \() -> Expect.equal 8 (32 |> Bitwise.shiftRightZfBy 2)
            , test "-32 |> shiftRightZfBy 1 == 2147483632" <| \() -> Expect.equal 2147483632 (-32 |> Bitwise.shiftRightZfBy 1)
            ]
        ]
