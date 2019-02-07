module Test.Tuple exposing (tests)

import Basics exposing (..)
import Tuple exposing (..)
import Test exposing (..)
import Expect


tests : Test
tests =
    describe "Tuple Tests"
        [ describe "first"
            [ test "extracts first element" <|
                \() -> Expect.equal 1 (first ( 1, 2 ))
            ]
        , describe "second"
            [ test "extracts second element" <|
                \() -> Expect.equal 2 (second ( 1, 2 ))
            ]
        , describe "mapFirst"
            [ test "applies function to first element" <|
                \() -> Expect.equal ( 5, 1 ) (mapFirst ((*) 5) ( 1, 1 ))
            ]
        , describe "mapSecond"
            [ test "applies function to second element" <|
                \() -> Expect.equal ( 1, 5 ) (mapSecond ((*) 5) ( 1, 1 ))
            ]
        ]
