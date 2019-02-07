module Test.Maybe exposing (tests)

import Basics exposing (..)
import Maybe exposing (..)
import Test exposing (..)
import Expect

tests : Test
tests =
  describe "Maybe Tests"

    [ describe "Common Helpers Tests"

      [ describe "withDefault Tests"
        [ test "no default used" <|
            \() -> Expect.equal 0 (Maybe.withDefault 5 (Just 0))
        , test "default used" <|
            \() -> Expect.equal 5 (Maybe.withDefault 5 (Nothing))
        ]

      , describe "map Tests"
        ( let f = (\n -> n + 1) in
          [ test "on Just" <|
              \() ->
                Expect.equal
                  (Just 1)
                  (Maybe.map f (Just 0))
          , test "on Nothing" <|
              \() ->
                Expect.equal
                  Nothing
                  (Maybe.map f Nothing)
          ]
        )

      , describe "map2 Tests"
        ( let f = (+) in
          [ test "on (Just, Just)" <|
              \() ->
                Expect.equal
                  (Just 1)
                  (Maybe.map2 f (Just 0) (Just 1))
          , test "on (Just, Nothing)" <|
              \() ->
                Expect.equal
                  Nothing
                  (Maybe.map2 f (Just 0) Nothing)
          , test "on (Nothing, Just)" <|
              \() ->
                Expect.equal
                  Nothing
                  (Maybe.map2 f Nothing (Just 0))
          ]
        )

        , describe "map3 Tests"
          ( let f = (\a b c -> a + b + c) in
            [ test "on (Just, Just, Just)" <|
                \() ->
                  Expect.equal
                    (Just 3)
                    (Maybe.map3 f (Just 1) (Just 1) (Just 1))
            , test "on (Just, Just, Nothing)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map3 f (Just 1) (Just 1) Nothing)
            , test "on (Just, Nothing, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map3 f (Just 1) Nothing (Just 1))
            , test "on (Nothing, Just, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map3 f Nothing (Just 1) (Just 1))
            ]
          )

        , describe "map4 Tests"
          ( let f = (\a b c d -> a + b + c + d) in
            [ test "on (Just, Just, Just, Just)" <|
                \() ->
                  Expect.equal
                    (Just 4)
                    (Maybe.map4 f (Just 1) (Just 1) (Just 1) (Just 1))
            , test "on (Just, Just, Just, Nothing)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map4 f (Just 1) (Just 1) (Just 1) Nothing)
            , test "on (Just, Just, Nothing, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map4 f (Just 1) (Just 1) Nothing (Just 1))
            , test "on (Just, Nothing, Just, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map4 f (Just 1) Nothing (Just 1) (Just 1))
            , test "on (Nothing, Just, Just, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map4 f Nothing (Just 1) (Just 1) (Just 1))
            ]
          )

        , describe "map5 Tests"
          ( let f = (\a b c d e -> a + b + c + d + e) in
            [ test "on (Just, Just, Just, Just, Just)" <|
                \() ->
                  Expect.equal
                    (Just 5)
                    (Maybe.map5 f (Just 1) (Just 1) (Just 1) (Just 1) (Just 1))
            , test "on (Just, Just, Just, Just, Nothing)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map5 f (Just 1) (Just 1) (Just 1) (Just 1) Nothing)
            , test "on (Just, Just, Just, Nothing, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map5 f (Just 1) (Just 1) (Just 1) Nothing (Just 1))
            , test "on (Just, Just, Nothing, Just, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map5 f (Just 1) (Just 1) Nothing (Just 1) (Just 1))
            , test "on (Just, Nothing, Just, Just, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map5 f (Just 1) Nothing (Just 1) (Just 1) (Just 1))
            , test "on (Nothing, Just, Just, Just, Just)" <|
                \() ->
                  Expect.equal
                    Nothing
                    (Maybe.map5 f Nothing (Just 1) (Just 1) (Just 1) (Just 1))
            ]
          )

      ]

    , describe "Chaining Maybes Tests"

      [ describe "andThen Tests"
        [ test "succeeding chain" <|
            \() ->
              Expect.equal
                (Just 1)
                (Maybe.andThen (\a -> Just a) (Just 1))
        , test "failing chain (original Maybe failed)" <|
            \() ->
              Expect.equal
                Nothing
                (Maybe.andThen (\a -> Just a) Nothing)
        , test "failing chain (chained function failed)" <|
            \() ->
              Expect.equal
                Nothing
                (Maybe.andThen (\a -> Nothing) (Just 1))
        ]
      ]

    ]
