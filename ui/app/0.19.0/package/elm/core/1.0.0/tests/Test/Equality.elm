module Test.Equality exposing (tests)

import Basics exposing (..)
import Maybe exposing (..)
import Test exposing (..)
import Expect


type Different
    = A String
    | B (List Int)


tests : Test
tests =
    let
        diffTests =
            describe "ADT equality"
                [ test "As eq" <| \() -> Expect.equal True (A "a" == A "a")
                , test "Bs eq" <| \() -> Expect.equal True (B [ 1 ] == B [ 1 ])
                , test "A left neq" <| \() -> Expect.equal True (A "a" /= B [ 1 ])
                , test "A left neq" <| \() -> Expect.equal True (B [ 1 ] /= A "a")
                ]

        recordTests =
            describe "Record equality"
                [ test "empty same" <| \() -> Expect.equal True ({} == {})
                , test "ctor same" <| \() -> Expect.equal True ({ field = Just 3 } == { field = Just 3 })
                , test "ctor same, special case" <| \() -> Expect.equal True ({ ctor = Just 3 } == { ctor = Just 3 })
                , test "ctor diff" <| \() -> Expect.equal True ({ field = Just 3 } /= { field = Nothing })
                , test "ctor diff, special case" <| \() -> Expect.equal True ({ ctor = Just 3 } /= { ctor = Nothing })
                ]
    in
        describe "Equality Tests" [ diffTests, recordTests ]
