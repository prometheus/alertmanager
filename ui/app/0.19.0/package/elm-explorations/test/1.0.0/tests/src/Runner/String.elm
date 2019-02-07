module Runner.String exposing (Summary, run, runWithOptions)

{-| String Runner

Run a test and present its results as a nicely-formatted String, along with
a count of how many tests passed and failed.

Note that this always uses an initial seed of 902101337, since it can't do effects.

@docs Summary, run, runWithOptions

-}

import Expect exposing (Expectation)
import Random
import Runner.String.Format
import Test exposing (Test)
import Test.Runner exposing (Runner, SeededRunners(..))


{-| The output string, the number of passed tests,
and the number of failed tests.
-}
type alias Summary =
    { output : String, passed : Int, failed : Int, autoFail : Maybe String }


toOutput : Summary -> SeededRunners -> Summary
toOutput summary seededRunners =
    let
        render =
            List.foldl toOutputHelp
    in
    case seededRunners of
        Plain runners ->
            render { summary | autoFail = Nothing } runners

        Only runners ->
            render { summary | autoFail = Just "Test.only was used" } runners

        Skipping runners ->
            render { summary | autoFail = Just "Test.skip was used" } runners

        Invalid message ->
            { output = message, passed = 0, failed = 0, autoFail = Nothing }


toOutputHelp : Runner -> Summary -> Summary
toOutputHelp runner summary =
    runner.run ()
        |> List.foldl (fromExpectation runner.labels) summary


fromExpectation : List String -> Expectation -> Summary -> Summary
fromExpectation labels expectation summary =
    case Test.Runner.getFailureReason expectation of
        Nothing ->
            { summary | passed = summary.passed + 1 }

        Just { given, description, reason } ->
            let
                message =
                    Runner.String.Format.format description reason

                prefix =
                    case given of
                        Nothing ->
                            ""

                        Just g ->
                            "Given " ++ g ++ "\n\n"

                newOutput =
                    "\n\n" ++ outputLabels labels ++ "\n" ++ (prefix ++ indentLines message) ++ "\n"
            in
            { summary
                | output = summary.output ++ newOutput
                , failed = summary.failed + 1
                , passed = summary.passed
            }


outputLabels : List String -> String
outputLabels labels =
    labels
        |> Test.Runner.formatLabels ((++) "↓ ") ((++) "✗ ")
        |> String.join "\n"


defaultSeed : Random.Seed
defaultSeed =
    Random.initialSeed 902101337


defaultRuns : Int
defaultRuns =
    100


indentLines : String -> String
indentLines str =
    str
        |> String.split "\n"
        |> List.map ((++) "    ")
        |> String.join "\n"


{-| Run a test and return a tuple of the output message and the number of
tests that failed.

Fuzz tests use a default run count of 100, and a fixed initial seed.

-}
run : Test -> Summary
run =
    runWithOptions defaultRuns defaultSeed


{-| Run a test and return a tuple of the output message and the number of
tests that failed.
-}
runWithOptions : Int -> Random.Seed -> Test -> Summary
runWithOptions runs seed test =
    let
        seededRunners =
            Test.Runner.fromTest runs seed test
    in
    toOutput
        { output = ""
        , passed = 0
        , failed = 0
        , autoFail = Just "no tests were run"
        }
        seededRunners
