module Test.Result exposing (tests)

import Basics exposing (..)
import Result
import Result exposing (Result(..))
import String
import Test exposing (..)
import Expect


isEven n =
    if n % 2 == 0 then
        Ok n
    else
        Err "number is odd"


add3 a b c =
    a + b + c


add4 a b c d =
    a + b + c + d


add5 a b c d e =
    a + b + c + d + e


tests : Test
tests =
    let
        mapTests =
            describe "map Tests"
                [ test "map Ok" <| \() -> Expect.equal (Ok 3) (Result.map ((+) 1) (Ok 2))
                , test "map Err" <| \() -> Expect.equal (Err "error") (Result.map ((+) 1) (Err "error"))
                ]

        mapNTests =
            describe "mapN Tests"
                [ test "map2 Ok" <| \() -> Expect.equal (Ok 3) (Result.map2 (+) (Ok 1) (Ok 2))
                , test "map2 Err" <| \() -> Expect.equal (Err "x") (Result.map2 (+) (Ok 1) (Err "x"))
                , test "map3 Ok" <| \() -> Expect.equal (Ok 6) (Result.map3 add3 (Ok 1) (Ok 2) (Ok 3))
                , test "map3 Err" <| \() -> Expect.equal (Err "x") (Result.map3 add3 (Ok 1) (Ok 2) (Err "x"))
                , test "map4 Ok" <| \() -> Expect.equal (Ok 10) (Result.map4 add4 (Ok 1) (Ok 2) (Ok 3) (Ok 4))
                , test "map4 Err" <| \() -> Expect.equal (Err "x") (Result.map4 add4 (Ok 1) (Ok 2) (Ok 3) (Err "x"))
                , test "map5 Ok" <| \() -> Expect.equal (Ok 15) (Result.map5 add5 (Ok 1) (Ok 2) (Ok 3) (Ok 4) (Ok 5))
                , test "map5 Err" <| \() -> Expect.equal (Err "x") (Result.map5 add5 (Ok 1) (Ok 2) (Ok 3) (Ok 4) (Err "x"))
                ]

        andThenTests =
            describe "andThen Tests"
                [ test "andThen Ok" <| \() -> Expect.equal (Ok 42) ((String.toInt "42") |> Result.andThen isEven)
                , test "andThen first Err" <|
                    \() ->
                        Expect.equal
                            (Err "could not convert string '4.2' to an Int")
                            (String.toInt "4.2" |> Result.andThen isEven)
                , test "andThen second Err" <|
                    \() ->
                        Expect.equal
                            (Err "number is odd")
                            (String.toInt "41" |> Result.andThen isEven)
                ]
    in
        describe "Result Tests"
            [ mapTests
            , mapNTests
            , andThenTests
            ]
