module Utils.FormValidation
    exposing
        ( validateDate
        , ValidatedMatcher
        , validatedMatcherToMatcher
        , validatedMatchersToMatchers
        )

import Utils.Date exposing (timeFromString)
import Utils.Types exposing (Matcher)
import Time exposing (Time)
import Tuple exposing (second)


validateDate : String -> Result ( String, String ) ( String, Time )
validateDate dateString =
    let
        parsedDate =
            timeFromString dateString
    in
        case parsedDate of
            Ok date ->
                Ok ( dateString, date )

            Err err ->
                Err ( dateString, err )


type alias ValidatedMatcher =
    { name : Result ( String, String ) String, value : Result ( String, String ) String, isRegex : Bool }


validatedMatchersToMatchers : List ValidatedMatcher -> Result String (List Matcher)
validatedMatchersToMatchers matchers =
    fold matchers (Ok [])


fold : List ValidatedMatcher -> Result String (List Matcher) -> Result String (List Matcher)
fold matchers agg =
    case matchers of
        [] ->
            agg

        m :: list ->
            fold list (aggregateValidatedMatchers agg m)


aggregateValidatedMatchers : Result String (List Matcher) -> ValidatedMatcher -> Result String (List Matcher)
aggregateValidatedMatchers agg matcher =
    Result.andThen
        (\matchers ->
            Result.map (\m -> m :: matchers) (validatedMatcherToMatcher matcher)
        )
        agg


validatedMatcherToMatcher : ValidatedMatcher -> Result String Matcher
validatedMatcherToMatcher matcher =
    Result.map2 (\n v -> Matcher n v matcher.isRegex) matcher.name matcher.value
        |> Result.mapError second
