module Tests exposing (all)

import Expect exposing (FloatingPointTolerance(..))
import FloatWithinTests exposing (floatWithinTests)
import Fuzz exposing (..)
import FuzzerTests exposing (fuzzerTests)
import Helpers exposing (..)
import Random
import RunnerTests
import Shrink
import Test exposing (..)
import Test.Runner
import Test.Runner.Failure exposing (Reason(..))


all : Test
all =
    Test.concat
        [ readmeExample
        , regressions
        , testTests
        , expectationTests
        , fuzzerTests
        , floatWithinTests
        , RunnerTests.all
        ]


readmeExample : Test
readmeExample =
    describe "The String module"
        [ describe "String.reverse"
            [ test "has no effect on a palindrome" <|
                \_ ->
                    let
                        palindrome =
                            "hannah"
                    in
                    Expect.equal palindrome (String.reverse palindrome)
            , test "reverses a known string" <|
                \_ ->
                    "ABCDEFG"
                        |> String.reverse
                        |> Expect.equal "GFEDCBA"
            , fuzz string "restores the original string if you run it again" <|
                \randomlyGeneratedString ->
                    randomlyGeneratedString
                        |> String.reverse
                        |> String.reverse
                        |> Expect.equal randomlyGeneratedString
            ]
        ]


expectationTests : Test
expectationTests =
    describe "Expectations"
        [ describe "Expect.err"
            [ test "passes on Err _" <|
                \_ ->
                    Err 12 |> Expect.err
            , test "passes on Ok _" <|
                \_ ->
                    Ok 12
                        |> Expect.err
                        |> expectToFail
            ]
        , describe "Expect.all"
            [ test "fails with empty list" <|
                \_ ->
                    "dummy subject"
                        |> Expect.all []
                        |> expectToFail
            ]
        , describe "Expect.equal"
            [ test "fails when equating two floats (see #230)" <|
                \_ ->
                    1.41
                        |> Expect.equal 1.41
                        |> expectToFail
            , test "succeeds when equating two ints" <|
                \_ -> 141 |> Expect.equal 141
            ]
        ]


regressions : Test
regressions =
    describe "regression tests"
        [ fuzz (intRange 1 32) "for #39" <|
            \positiveInt ->
                positiveInt
                    |> Expect.greaterThan 0
        , test "for #127" <|
            {- If fuzz tests actually run 100 times, then asserting that no number
               in 1..8 equals 5 fails with 0.999998 probability. If they only run
               once, or stop after a duplicate due to #127, then it's much more
               likely (but not guaranteed) that the 5 won't turn up. See #128.
            -}
            \() ->
                fuzz
                    (custom (Random.int 1 8) Shrink.noShrink)
                    "fuzz tests run 100 times"
                    (Expect.notEqual 5)
                    |> expectTestToFail
        ]


testTests : Test
testTests =
    describe "functions that create tests"
        [ describe "describe"
            [ test "fails with empty list" <|
                \() ->
                    describe "x" []
                        |> expectTestToFail
            , test "fails with empty description" <|
                \() ->
                    describe "" [ test "x" expectPass ]
                        |> expectTestToFail
            ]
        , describe "test"
            [ test "fails with empty name" <|
                \() ->
                    test "" expectPass
                        |> expectTestToFail
            ]
        , describe "fuzz"
            [ test "fails with empty name" <|
                \() ->
                    fuzz Fuzz.bool "" expectPass
                        |> expectTestToFail
            ]
        , describe "fuzzWith"
            [ test "fails with fewer than 1 run" <|
                \() ->
                    fuzzWith { runs = 0 } Fuzz.bool "nonpositive" expectPass
                        |> expectTestToFail
            , test "fails with empty name" <|
                \() ->
                    fuzzWith { runs = 1 } Fuzz.bool "" expectPass
                        |> expectTestToFail
            ]
        , describe "Test.todo"
            [ test "causes test failure" <|
                \() ->
                    todo "a TODO test fails"
                        |> expectTestToFail
            , test "Passes are not TODO"
                (\_ -> Expect.pass |> Test.Runner.isTodo |> Expect.false "was true")
            , test "Simple failures are not TODO" <|
                \_ ->
                    Expect.fail "reason" |> Test.Runner.isTodo |> Expect.false "was true"
            ]
        , identicalNamesAreRejectedTests
        ]


identicalNamesAreRejectedTests : Test
identicalNamesAreRejectedTests =
    describe "Identically-named sibling and parent/child tests fail"
        [ test "a describe with two identically named children" <|
            \() ->
                describe "x"
                    [ test "foo" expectPass
                    , test "foo" expectPass
                    ]
                    |> expectTestToFail
        , test "a describe with the same name as a child test" <|
            \() ->
                describe "A"
                    [ test "A" expectPass ]
                    |> expectTestToFail
        , test "a describe with the same name as a child describe fails" <|
            \() ->
                describe "A"
                    [ describe "A"
                        [ test "x" expectPass ]
                    ]
                    |> expectTestToFail
        , test "a describe with the same name as a sibling describe fails" <|
            \() ->
                Test.concat
                    [ describe "A" [ test "x" expectPass ]
                    , describe "A" [ test "y" expectPass ]
                    ]
                    |> expectTestToFail
        , test "a describe with the same name as a de facto sibling describe fails" <|
            \() ->
                Test.concat
                    [ Test.concat
                        [ describe "A" [ test "x" expectPass ]
                        ]
                    , describe "A" [ test "y" expectPass ]
                    ]
                    |> expectTestToFail
        , test "a describe with the same name as a de facto sibling describe fails (2)" <|
            \() ->
                Test.concat
                    [ Test.concat
                        [ describe "A" [ test "x" expectPass ]
                        ]
                    , Test.concat
                        [ describe "A" [ test "y" expectPass ]
                        ]
                    ]
                    |> expectTestToFail
        ]
