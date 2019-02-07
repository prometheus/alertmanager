module Main exposing (..)

{-| HOW TO RUN THESE TESTS

$ npm test

Note that this always uses an initial seed of 902101337, since it can't do effects.

-}

import Browser
import Html
import Platform
import Runner.Log
import Runner.String exposing (Summary)
import SeedTests
import Tests


main : Program () () msg
main =
    let
        program =
            --TODO: ideally use Platform.worker here, but it doesn't compile with the current Elm 0.19 alpha (2018-05-19)
            Browser.staticPage (Html.text "")
    in
    runAllTests program


runAllTests : a -> a
runAllTests a =
    let
        runSeedTest =
            Runner.String.runWithOptions 1 SeedTests.fixedSeed

        _ =
            [ [ Runner.String.run Tests.all ]
            , List.map runSeedTest SeedTests.tests
            , List.map (runSeedTest >> removeAutoFail) SeedTests.noAutoFail
            ]
                |> List.concat
                |> List.foldl combineSummaries emptySummary
                |> Runner.Log.logOutput
    in
    a


emptySummary : Summary
emptySummary =
    { output = "", passed = 0, failed = 0, autoFail = Nothing }


{-| Considers autoFail as pass so we can actually write tests about Test.skip
and Test.only which do not automatically fail.
-}
removeAutoFail : Summary -> Summary
removeAutoFail summary =
    { summary | autoFail = Nothing }


combineSummaries : Summary -> Summary -> Summary
combineSummaries first second =
    { output = first.output ++ second.output
    , passed = first.passed + second.passed
    , failed = first.failed + second.failed
    , autoFail =
        case ( first.autoFail, second.autoFail ) of
            ( Nothing, Nothing ) ->
                Nothing

            ( Nothing, secondAutoFail ) ->
                secondAutoFail

            ( firstAutoFail, Nothing ) ->
                firstAutoFail

            ( Just firstAutoFail, Just secondAutoFail ) ->
                [ firstAutoFail, secondAutoFail ]
                    |> String.join "\n"
                    |> Just
    }
