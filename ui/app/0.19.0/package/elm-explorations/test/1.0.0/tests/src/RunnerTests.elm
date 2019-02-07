module RunnerTests exposing (all)

import Expect
import Fuzz exposing (..)
import Helpers exposing (expectPass)
import Random
import Test exposing (..)
import Test.Runner exposing (SeededRunners(..))
import Test.Runner.Failure


all : Test
all =
    Test.concat
        [ fromTest ]


toSeededRunners : Test -> SeededRunners
toSeededRunners =
    Test.Runner.fromTest 5 (Random.initialSeed 42)


fromTest : Test
fromTest =
    describe "TestRunner.fromTest"
        [ describe "test length"
            [ fuzz2 int int "only positive tests runs are valid" <|
                \runs intSeed ->
                    case Test.Runner.fromTest runs (Random.initialSeed intSeed) passing of
                        Invalid str ->
                            if runs > 0 then
                                Expect.fail ("Expected a run count of " ++ String.fromInt runs ++ " to be valid, but was invalid with this message: " ++ Debug.toString str)

                            else
                                Expect.pass

                        val ->
                            if runs > 0 then
                                Expect.pass

                            else
                                Expect.fail ("Expected a run count of " ++ String.fromInt runs ++ " to be invalid, but was valid with this value: " ++ Debug.toString val)
            , test "an only inside another only has no effect" <|
                \_ ->
                    let
                        seededRunners =
                            toSeededRunners <|
                                describe "three tests"
                                    [ test "passes" expectPass
                                    , Test.only <|
                                        describe "two tests"
                                            [ test "fails" <|
                                                \_ -> Expect.fail "failed on purpose"
                                            , Test.only <|
                                                test "is an only" <|
                                                    \_ -> Expect.fail "failed on purpose"
                                            ]
                                    ]
                    in
                    case seededRunners of
                        Only runners ->
                            runners
                                |> List.length
                                |> Expect.equal 2

                        val ->
                            Expect.fail ("Expected SeededRunner to be Only, but was " ++ Debug.toString val)
            , test "a skip inside an only takes effect" <|
                \_ ->
                    let
                        seededRunners =
                            toSeededRunners <|
                                describe "three tests"
                                    [ test "passes" expectPass
                                    , Test.only <|
                                        describe "two tests"
                                            [ test "fails" <|
                                                \_ -> Expect.fail "failed on purpose"
                                            , Test.skip <|
                                                test "is skipped" <|
                                                    \_ -> Expect.fail "failed on purpose"
                                            ]
                                    ]
                    in
                    case seededRunners of
                        Only runners ->
                            runners
                                |> List.length
                                |> Expect.equal 1

                        val ->
                            Expect.fail ("Expected SeededRunner to be Only, but was " ++ Debug.toString val)
            , test "an only inside a skip has no effect" <|
                \_ ->
                    let
                        seededRunners =
                            toSeededRunners <|
                                describe "three tests"
                                    [ test "passes" expectPass
                                    , Test.skip <|
                                        describe "two tests"
                                            [ test "fails" <|
                                                \_ -> Expect.fail "failed on purpose"
                                            , Test.only <|
                                                test "is skipped" <|
                                                    \_ -> Expect.fail "failed on purpose"
                                            ]
                                    ]
                    in
                    case seededRunners of
                        Skipping runners ->
                            runners
                                |> List.length
                                |> Expect.equal 1

                        val ->
                            Expect.fail ("Expected SeededRunner to be Skipping, but was " ++ Debug.toString val)
            , test "a test that uses only is an Only summary" <|
                \_ ->
                    case toSeededRunners (Test.only <| test "passes" expectPass) of
                        Only runners ->
                            runners
                                |> List.length
                                |> Expect.equal 1

                        val ->
                            Expect.fail ("Expected SeededRunner to be Only, but was " ++ Debug.toString val)
            , test "a skip inside another skip has no effect" <|
                \_ ->
                    let
                        seededRunners =
                            toSeededRunners <|
                                describe "three tests"
                                    [ test "passes" expectPass
                                    , Test.skip <|
                                        describe "two tests"
                                            [ test "fails" <|
                                                \_ -> Expect.fail "failed on purpose"
                                            , Test.skip <|
                                                test "is skipped" <|
                                                    \_ -> Expect.fail "failed on purpose"
                                            ]
                                    ]
                    in
                    case seededRunners of
                        Skipping runners ->
                            runners
                                |> List.length
                                |> Expect.equal 1

                        val ->
                            Expect.fail ("Expected SeededRunner to be Skipping, but was " ++ Debug.toString val)
            , test "a pair of tests where one uses skip is a Skipping summary" <|
                \_ ->
                    let
                        seededRunners =
                            toSeededRunners <|
                                describe "two tests"
                                    [ test "passes" expectPass
                                    , Test.skip <|
                                        test "fails" <|
                                            \_ -> Expect.fail "failed on purpose"
                                    ]
                    in
                    case seededRunners of
                        Skipping runners ->
                            runners
                                |> List.length
                                |> Expect.equal 1

                        val ->
                            Expect.fail ("Expected SeededRunner to be Skipping, but was " ++ Debug.toString val)
            , test "when all tests are skipped, we get an empty Skipping summary" <|
                \_ ->
                    case toSeededRunners (Test.skip <| test "passes" expectPass) of
                        Skipping runners ->
                            runners
                                |> List.length
                                |> Expect.equal 0

                        val ->
                            Expect.fail ("Expected SeededRunner to be Skipping, but was " ++ Debug.toString val)
            , test "a test that does not use only or skip is a Plain summary" <|
                \_ ->
                    case toSeededRunners (test "passes" expectPass) of
                        Plain runners ->
                            runners
                                |> List.length
                                |> Expect.equal 1

                        val ->
                            Expect.fail ("Expected SeededRunner to be Plain, but was " ++ Debug.toString val)
            ]
        , describe "catching exceptions"
            [ test "when a test raises an exception, it is turned into a failure" <|
                \() ->
                    case toSeededRunners (test "crashes" <| \() -> Debug.todo "crash") of
                        Plain [ runner ] ->
                            runner.run ()
                                |> List.head
                                |> Maybe.andThen Test.Runner.getFailureReason
                                |> Expect.equal
                                    (Just
                                        { given = Nothing
                                        , description = "This test failed because it threw an exception: \"Error: TODO in module `RunnerTests` on line 197\n\ncrash\""
                                        , reason = Test.Runner.Failure.Custom
                                        }
                                    )

                        Plain runners ->
                            Expect.fail ("Expected SeededRunner to have one runner, but had " ++ Debug.toString runners)

                        val ->
                            Expect.fail ("Expected SeededRunner to be Plain, but was " ++ Debug.toString val)
            ]
        ]


passing : Test
passing =
    test "A passing test" expectPass
