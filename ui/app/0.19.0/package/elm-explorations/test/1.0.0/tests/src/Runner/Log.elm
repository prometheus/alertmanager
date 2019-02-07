module Runner.Log exposing (logOutput, run, runWithOptions)

{-| Log Runner

Runs a test and outputs its results using `Debug.log`, then calls `Debug.crash`
if there are any failures.

This is not the prettiest runner, but it is simple and cross-platform. For
example, you can use it as a crude Node runner like so:

    $ elm-make LogRunnerExample.elm --output=elm.js
    $ node elm.js

This will log the test results to the console, then exit with exit code 0
if the tests all passed, and 1 if any failed.

@docs run, runWithOptions

-}

import Random
import Runner.String exposing (Summary)
import String
import Test exposing (Test)


{-| Run the test using the default `Test.Runner.String` options.
-}
run : Test -> ()
run test =
    Runner.String.run test
        |> logOutput


{-| Run the test using the provided options.
-}
runWithOptions : Int -> Random.Seed -> Test -> ()
runWithOptions runs seed test =
    Runner.String.runWithOptions runs seed test
        |> logOutput


summarize : Summary -> String
summarize { output, passed, failed, autoFail } =
    let
        headline =
            if failed > 0 then
                output ++ "\n\nTEST RUN FAILED"

            else
                case autoFail of
                    Nothing ->
                        "TEST RUN PASSED"

                    Just reason ->
                        "TEST RUN FAILED because " ++ reason
    in
    String.join "\n"
        [ output
        , headline ++ "\n"
        , "Passed: " ++ String.fromInt passed
        , "Failed: " ++ String.fromInt failed
        ]


logOutput : Summary -> ()
logOutput summary =
    let
        output =
            summarize summary ++ "\n\nExit code"

        _ =
            if summary.failed > 0 || summary.autoFail /= Nothing then
                output
                    |> (\a -> Debug.log a 1)
                    |> (\_ -> Debug.todo "FAILED TEST RUN")
                    |> (\_ -> ())

            else
                output
                    |> (\a -> Debug.log a 0)
                    |> (\_ -> ())
    in
    ()
