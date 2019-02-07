module Main exposing (..)

import Benchmark exposing (..)
import Benchmark.Runner as Runner
import Expect exposing (Expectation)
import Random.Pcg
import Snippets
import Test.Internal exposing (Test(Labeled, Test))


main : Runner.BenchmarkProgram
main =
    Runner.program suite


suite : Benchmark
suite =
    describe "Fuzz"
        [ describe "int"
            [ benchmark "generating" (benchTest Snippets.intPass)
            , benchmark "shrinking" (benchTest Snippets.intFail)
            ]
        , describe "intRange"
            [ benchmark "generating" (benchTest Snippets.intRangePass)
            , benchmark "shrinking" (benchTest Snippets.intRangeFail)
            ]
        , describe "string"
            [ benchmark "generating" (benchTest Snippets.stringPass)
            , benchmark "shrinking" (benchTest Snippets.stringFail)
            ]
        , describe "float"
            [ benchmark "generating" (benchTest Snippets.floatPass)
            , benchmark "shrinking" (benchTest Snippets.floatFail)
            ]
        , describe "bool"
            [ benchmark "generating" (benchTest Snippets.boolPass)
            , benchmark "shrinking" (benchTest Snippets.boolFail)
            ]
        , describe "char"
            [ benchmark "generating" (benchTest Snippets.charPass)
            , benchmark "shrinking" (benchTest Snippets.charFail)
            ]
        , describe "list of int"
            [ benchmark "generating" (benchTest Snippets.listIntPass)
            , benchmark "shrinking" (benchTest Snippets.listIntFail)
            ]
        , describe "maybe of int"
            [ benchmark "generating" (benchTest Snippets.maybeIntPass)
            , benchmark "shrinking" (benchTest Snippets.maybeIntFail)
            ]
        , describe "result of string and int"
            [ benchmark "generating" (benchTest Snippets.resultPass)
            , benchmark "shrinking" (benchTest Snippets.resultFail)
            ]
        , describe "map"
            [ benchmark "generating" (benchTest Snippets.mapPass)
            , benchmark "shrinking" (benchTest Snippets.mapFail)
            ]
        , describe "andMap"
            [ benchmark "generating" (benchTest Snippets.andMapPass)
            , benchmark "shrinking" (benchTest Snippets.andMapFail)
            ]
        , describe "map5"
            [ benchmark "generating" (benchTest Snippets.map5Pass)
            , benchmark "shrinking" (benchTest Snippets.map5Fail)
            ]
        , describe "andThen"
            [ benchmark "generating" (benchTest Snippets.andThenPass)
            , benchmark "shrinking" (benchTest Snippets.andThenFail)
            ]
        , describe "conditional"
            [ benchmark "generating" (benchTest Snippets.conditionalPass)
            , benchmark "shrinking" (benchTest Snippets.conditionalFail)
            ]
        ]


benchTest : Test -> (() -> List Expectation)
benchTest test =
    case test of
        Test fn ->
            \_ -> fn (Random.Pcg.initialSeed 0) 10

        Labeled _ test ->
            benchTest test

        test ->
            Debug.crash <| "No support for benchmarking this type of test: " ++ toString test
