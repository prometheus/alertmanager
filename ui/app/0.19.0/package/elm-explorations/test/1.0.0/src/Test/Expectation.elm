module Test.Expectation exposing (Expectation(..), fail, withGiven)

import Test.Runner.Failure exposing (Reason)


type Expectation
    = Pass
    | Fail { given : Maybe String, description : String, reason : Reason }


{-| Create a failure without specifying the given.
-}
fail : { description : String, reason : Reason } -> Expectation
fail { description, reason } =
    Fail { given = Nothing, description = description, reason = reason }


{-| Set the given (fuzz test input) of an expectation.
-}
withGiven : String -> Expectation -> Expectation
withGiven newGiven expectation =
    case expectation of
        Fail failure ->
            Fail { failure | given = Just newGiven }

        Pass ->
            expectation
