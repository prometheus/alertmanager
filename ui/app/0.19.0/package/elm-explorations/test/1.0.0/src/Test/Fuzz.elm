module Test.Fuzz exposing (fuzzTest)

import Dict exposing (Dict)
import Fuzz exposing (Fuzzer)
import Fuzz.Internal exposing (ValidFuzzer)
import Lazy.List
import Random exposing (Generator)
import RoseTree exposing (RoseTree(..))
import Test.Expectation exposing (Expectation(..))
import Test.Internal as Internal exposing (Test(..), blankDescriptionFailure, failNow)
import Test.Runner.Failure exposing (InvalidReason(..), Reason(..))


{-| Reject always-failing tests because of bad names or invalid fuzzers.
-}
fuzzTest : Fuzzer a -> String -> (a -> Expectation) -> Test
fuzzTest fuzzer untrimmedDesc getExpectation =
    let
        desc =
            String.trim untrimmedDesc
    in
    if String.isEmpty desc then
        blankDescriptionFailure

    else
        case fuzzer of
            Err reason ->
                failNow
                    { description = reason
                    , reason = Invalid InvalidFuzzer
                    }

            Ok validFuzzer ->
                -- Preliminary checks passed; run the fuzz test
                validatedFuzzTest validFuzzer desc getExpectation


{-| Knowing that the fuzz test isn't obviously invalid, run the test and package up the results.
-}
validatedFuzzTest : ValidFuzzer a -> String -> (a -> Expectation) -> Test
validatedFuzzTest fuzzer desc getExpectation =
    let
        run seed runs =
            let
                failures =
                    getFailures fuzzer getExpectation seed runs
            in
            -- Make sure if we passed, we don't do any more work.
            if Dict.isEmpty failures then
                [ Pass ]

            else
                failures
                    |> Dict.toList
                    |> List.map formatExpectation
    in
    Labeled desc (FuzzTest run)


type alias Failures =
    Dict String Expectation


getFailures : ValidFuzzer a -> (a -> Expectation) -> Random.Seed -> Int -> Dict String Expectation
getFailures fuzzer getExpectation initialSeed totalRuns =
    {- Fuzz test algorithm with memoization and opt-in RoseTrees:
       Generate a single value from the fuzzer's genVal random generator
       Determine if the value is memoized. If so, skip. Otherwise continue.
       Run the test on that value. If it fails:
           Generate the rosetree by passing the fuzzer False *and the same random seed*
           Find the new failure by looking at the children for any shrunken values:
               If a shrunken value causes a failure, recurse on its children
               If no shrunken value replicates the failure, use the root
       Whether it passes or fails, do this n times
    -}
    let
        genVal =
            Random.map RoseTree.root fuzzer

        initialFailures =
            Dict.empty

        helper currentSeed remainingRuns failures =
            let
                ( value, nextSeed ) =
                    Random.step genVal currentSeed

                newFailures =
                    findNewFailure fuzzer getExpectation failures currentSeed value
            in
            if remainingRuns <= 1 then
                newFailures

            else
                helper nextSeed (remainingRuns - 1) newFailures
    in
    helper initialSeed totalRuns initialFailures


{-| Knowing that a value in not in the cache, determine if it causes the test to pass or fail.
-}
findNewFailure :
    ValidFuzzer a
    -> (a -> Expectation)
    -> Failures
    -> Random.Seed
    -> a
    -> Failures
findNewFailure fuzzer getExpectation failures currentSeed value =
    case getExpectation value of
        Pass ->
            failures

        failedExpectation ->
            let
                ( rosetree, nextSeed ) =
                    -- nextSeed is not used here because caller function has currentSeed
                    Random.step fuzzer currentSeed
            in
            shrinkAndAdd rosetree getExpectation failedExpectation failures


{-| Knowing that the rosetree's root already failed, finds the shrunken failure.
Returns the updated failures dictionary.
-}
shrinkAndAdd :
    RoseTree a
    -> (a -> Expectation)
    -> Expectation
    -> Failures
    -> Failures
shrinkAndAdd rootTree getExpectation rootsExpectation failures =
    let
        shrink : Expectation -> RoseTree a -> ( a, Expectation )
        shrink oldExpectation (Rose failingValue branches) =
            case Lazy.List.headAndTail branches of
                Just ( (Rose possiblyFailingValue _) as rosetree, moreLazyRoseTrees ) ->
                    -- either way, recurse with the most recent failing expectation, and failing input with its list of shrunken values
                    case getExpectation possiblyFailingValue of
                        Pass ->
                            shrink oldExpectation
                                (Rose failingValue moreLazyRoseTrees)

                        newExpectation ->
                            let
                                ( minimalValue, finalExpectation ) =
                                    shrink newExpectation rosetree
                            in
                            ( minimalValue
                            , finalExpectation
                            )

                Nothing ->
                    ( failingValue, oldExpectation )

        ( rootMinimalValue, rootFinalExpectation ) =
            shrink rootsExpectation rootTree
    in
    Dict.insert (Internal.toString rootMinimalValue) rootFinalExpectation failures


formatExpectation : ( String, Expectation ) -> Expectation
formatExpectation ( given, expectation ) =
    Test.Expectation.withGiven given expectation
