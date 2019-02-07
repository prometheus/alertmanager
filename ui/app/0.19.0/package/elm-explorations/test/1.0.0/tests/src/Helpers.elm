module Helpers exposing (different, expectPass, expectTestToFail, expectToFail, randomSeedFuzzer, same, succeeded, testShrinking, testStringLengthIsPreserved)

import Expect exposing (Expectation)
import Fuzz exposing (Fuzzer)
import Random
import Shrink
import Test exposing (Test)
import Test.Runner exposing (Runner, SeededRunners)
import Test.Runner.Failure exposing (Reason(..))


expectPass : a -> Expectation
expectPass _ =
    Expect.pass


testStringLengthIsPreserved : List String -> Expectation
testStringLengthIsPreserved strings =
    strings
        |> List.map String.length
        |> List.sum
        |> Expect.equal (String.length (List.foldl (++) "" strings))


expectToFail : Expectation -> Expectation
expectToFail expectation =
    case Test.Runner.getFailureReason expectation of
        Nothing ->
            Expect.fail "Expected this test to fail, but it passed!"

        Just _ ->
            Expect.pass


expectTestToFail : Test -> Expectation
expectTestToFail test =
    let
        seed =
            Random.initialSeed 99
    in
    test
        |> Test.Runner.fromTest 100 seed
        |> getRunners
        |> List.concatMap (.run >> (\run -> run ()))
        |> List.map expectToFail
        |> List.map always
        |> Expect.all
        |> (\all -> all ())


succeeded : Expectation -> Bool
succeeded expectation =
    case Test.Runner.getFailureReason expectation of
        Nothing ->
            True

        Just _ ->
            False


passToFail :
    ({ reason : Reason
     , description : String
     , given : Maybe String
     }
     -> Result String ()
    )
    -> Expectation
    -> Expectation
passToFail f expectation =
    let
        result =
            case Test.Runner.getFailureReason expectation of
                Nothing ->
                    Err "Expected this test to fail, but it passed!"

                Just record ->
                    f record
    in
    case result of
        Ok () ->
            Expect.pass

        Err message ->
            Expect.fail message


getRunners : SeededRunners -> List Runner
getRunners seededRunners =
    case seededRunners of
        Test.Runner.Plain runners ->
            runners

        Test.Runner.Only runners ->
            runners

        Test.Runner.Skipping runners ->
            runners

        Test.Runner.Invalid _ ->
            []


expectFailureHelper :
    ({ description : String
     , given : Maybe String
     , reason : Reason
     }
     -> Result String ()
    )
    -> Test
    -> Test
expectFailureHelper f test =
    let
        seed =
            Random.initialSeed 99
    in
    test
        |> Test.Runner.fromTest 100 seed
        |> getRunners
        |> List.concatMap (.run >> (\run -> run ()))
        |> List.map (passToFail f)
        |> List.map (\result -> \() -> result)
        |> List.indexedMap (\i t -> Test.test (String.fromInt i) t)
        |> Test.describe "testShrinking"


testShrinking : Test -> Test
testShrinking =
    let
        handleFailure { given, description } =
            let
                acceptable =
                    String.split "|" description
            in
            case given of
                Nothing ->
                    Err "Expected this test to have a given value!"

                Just g ->
                    if List.member g acceptable then
                        Ok ()

                    else
                        Err <| "Got shrunken value " ++ g ++ " but expected " ++ String.join " or " acceptable
    in
    expectFailureHelper handleFailure


{-| get a good distribution of random seeds, and don't shrink our seeds!
-}
randomSeedFuzzer : Fuzzer Random.Seed
randomSeedFuzzer =
    Fuzz.custom (Random.int 0 0xFFFFFFFF) Shrink.noShrink |> Fuzz.map Random.initialSeed


same : Expectation -> Expectation -> Expectation
same a b =
    case ( Test.Runner.getFailureReason a, Test.Runner.getFailureReason b ) of
        ( Nothing, Nothing ) ->
            Expect.pass

        ( Just _, Just _ ) ->
            Expect.pass

        ( reasonA, reasonB ) ->
            Expect.equal reasonA reasonB
                |> Expect.onFail "expected both arguments to fail, or both to succeed"


different : Expectation -> Expectation -> Expectation
different a b =
    case ( Test.Runner.getFailureReason a, Test.Runner.getFailureReason b ) of
        ( Nothing, Just _ ) ->
            Expect.pass

        ( Just _, Nothing ) ->
            Expect.pass

        ( Nothing, Nothing ) ->
            Expect.fail "expected only one argument to fail, but both passed"

        ( Just reasonA, Just reasonB ) ->
            [ reasonA, reasonB ]
                |> Expect.equal []
                |> Expect.onFail "expected only one argument to fail"
