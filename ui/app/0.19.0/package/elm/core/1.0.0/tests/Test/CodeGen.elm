module Test.CodeGen exposing (tests)

import Basics exposing (..)
import Test exposing (..)
import Expect
import Maybe
import Maybe exposing (..)


type Wrapper a
    = Wrapper a


caseUnderscore : Maybe number -> number
caseUnderscore m_ =
    case m_ of
        Just x ->
            x

        Nothing ->
            0


patternUnderscore : number
patternUnderscore =
    case Just 42 of
        Just x_ ->
            x_

        Nothing ->
            0


letQualified : number
letQualified =
    let
        (Wrapper x) =
            Wrapper 42
    in
        x


caseQualified : number
caseQualified =
    case Just 42 of
        Maybe.Just x ->
            x

        Nothing ->
            0


caseScope : String
caseScope =
    case "Not this one!" of
        string ->
            case "Hi" of
                string ->
                    string


tests : Test
tests =
    let
        -- We don't strictly speaking need annotations in this let-expression,
        -- but having these here exercises the parser to avoid regressions like
        -- https://github.com/elm-lang/elm-compiler/issues/1535
        underscores : Test
        underscores =
            describe "Underscores"
                [ test "case" <| \() -> Expect.equal 42 (caseUnderscore (Just 42))
                , test "pattern" <| \() -> Expect.equal 42 patternUnderscore
                ]

        qualifiedPatterns : Test
        qualifiedPatterns =
            describe "Qualified Patterns"
                [ test "let" <| \() -> Expect.equal 42 letQualified
                , test "case" <| \() -> Expect.equal 42 caseQualified
                ]

        scope : Test
        scope =
            describe "Scoping"
                [ test "case" <| \() -> Expect.equal "Hi" caseScope ]

        hex : Test
        hex =
            describe "Hex"
                [ test "0xFFFFFFFF" <|
                    \() ->
                        0xFFFFFFFF
                            |> Expect.equal 4294967295
                , test "0xD066F00D" <|
                    \() ->
                        0xD066F00D
                            |> Expect.equal 3496407053
                , test "0x00" <|
                    \() ->
                        0x00
                            |> Expect.equal 0
                ]
    in
        describe "CodeGen"
            [ underscores
            , qualifiedPatterns
            , scope
            , hex
            ]
