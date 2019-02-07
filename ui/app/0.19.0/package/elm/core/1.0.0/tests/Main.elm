port module Main exposing (..)

import Basics exposing (..)
import Task exposing (..)
import Test exposing (..)
import Platform.Cmd exposing (Cmd)
import Json.Decode exposing (Value)
import Test.Runner.Node exposing (run, TestProgram)
import Test.Array as Array
import Test.Basics as Basics
import Test.Bitwise as Bitwise
import Test.Char as Char
import Test.CodeGen as CodeGen
import Test.Dict as Dict
import Test.Maybe as Maybe
import Test.Equality as Equality
import Test.Json as Json
import Test.List as List
import Test.Result as Result
import Test.Set as Set
import Test.String as String
import Test.Tuple as Tuple


tests : Test
tests =
    describe "Elm Standard Library Tests"
        [ Array.tests
        , Basics.tests
        , Bitwise.tests
        , Char.tests
        , CodeGen.tests
        , Dict.tests
        , Equality.tests
        , List.tests
        , Result.tests
        , Set.tests
        , String.tests
        , Maybe.tests
        , Tuple.tests
        ]


main : TestProgram
main =
    run emit tests


port emit : ( String, Value ) -> Cmd msg
