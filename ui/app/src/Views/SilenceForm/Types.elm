module Views.SilenceForm.Types
    exposing
        ( SilenceFormMsg(..)
        , SilenceFormFieldMsg(..)
        , Model
        , SilenceForm
        , fromMatchersAndTime
        , fromSilence
        , toSilence
        , initSilenceForm
        , validateComment
        , validateCreatedBy
        , validateDuration
        , validateEndsAt
        , validateMatcher
        , validateStartsAt
        , validateMatcherName
        , validateMatcherValue
        )

import Silences.Types exposing (Silence, SilenceId, nullSilenceStatus)
import Alerts.Types exposing (Alert)
import Utils.Types exposing (Matcher, ApiData, Duration, ApiResponse(..))
import Time exposing (Time)
import Utils.Date exposing (timeToString, timeFromString, durationFormat, parseDuration)
import Tuple exposing (second)
import Time exposing (Time)
import Utils.FormValidation
    exposing
        ( validateDate
        , ValidatedMatcher
        , validatedMatchersToMatchers
        )


initSilenceForm : Model
initSilenceForm =
    { form = empty
    , silence = toSilence empty
    }


toSilence : SilenceForm -> Result String Silence
toSilence form =
    Result.map5
        (\comment matchers createdBy parsedStartsAt parsedEndsAt ->
            { id = form.id
            , comment = comment
            , matchers = matchers
            , createdBy = createdBy
            , startsAt = parsedStartsAt
            , endsAt = parsedEndsAt

            {- ignored -}
            , silencedAlerts = Initial

            {- ignored -}
            , updatedAt = 0

            {- ignored -}
            , status = nullSilenceStatus
            }
        )
        (Result.mapError second form.comment)
        (validatedMatchersToMatchers form.matchers)
        (Result.mapError second form.createdBy)
        (Result.map second <| Result.mapError second form.startsAt)
        (Result.map second <| Result.mapError second form.endsAt)


fromSilence : Silence -> SilenceForm
fromSilence { id, createdBy, comment, startsAt, endsAt, matchers } =
    { id = id
    , createdBy = validateCreatedBy createdBy
    , comment = validateComment comment
    , startsAt = validateStartsAt (timeToString startsAt)
    , endsAt = validateEndsAt (timeToString endsAt)
    , duration = validateDuration (durationFormat (endsAt - startsAt))
    , matchers = List.map validateMatcher matchers
    }


empty : SilenceForm
empty =
    { id = ""
    , createdBy = validateCreatedBy ""
    , comment = validateComment ""
    , startsAt = validateStartsAt ""
    , endsAt = validateEndsAt ""
    , duration = validateDuration ""
    , matchers = []
    }


fromMatchersAndTime : List Matcher -> Time -> SilenceForm
fromMatchersAndTime matchers now =
    let
        duration =
            2 * Time.hour

        -- If no matchers were specified, show a sample matcher
        enrichedMatchers =
            if List.length matchers == 0 then
                [ Matcher "env" "production" False ]
            else
                matchers
    in
        { empty
            | startsAt = validateStartsAt <| timeToString now
            , endsAt = validateEndsAt <| timeToString (now + duration)
            , duration = validateDuration <| durationFormat duration
            , matchers = List.map validateMatcher enrichedMatchers
        }


type alias Model =
    { silence : Result String Silence
    , form : SilenceForm
    }


type alias SilenceForm =
    { id : String
    , createdBy : Result ( String, String ) String
    , comment : Result ( String, String ) String
    , startsAt : Result ( String, String ) ( String, Time )
    , endsAt : Result ( String, String ) ( String, Time )
    , duration : Result ( String, String ) ( String, Time )
    , matchers : List ValidatedMatcher
    }


type SilenceFormMsg
    = UpdateField SilenceFormFieldMsg
    | CreateSilence Silence
    | PreviewSilence Silence
    | AlertGroupsPreview (ApiData (List Alert))
    | FetchSilence String
    | NewSilenceFromMatchers (List Matcher)
    | NewSilenceFromMatchersAndTime (List Matcher) Time
    | SilenceFetch (ApiData Silence)
    | SilenceCreate (ApiData String)


type SilenceFormFieldMsg
    = AddMatcher
    | UpdateStartsAt String
    | UpdateEndsAt String
    | UpdateDuration String
    | UpdateCreatedBy String
    | UpdateComment String
    | DeleteMatcher Int
    | UpdateMatcherName Int String
    | UpdateMatcherValue Int String
    | UpdateMatcherRegex Int Bool


validateCreatedBy : String -> Result ( String, String ) String
validateCreatedBy =
    stringNotEmpty


stringNotEmpty : String -> Result ( String, String ) String
stringNotEmpty string =
    if String.isEmpty string then
        Err ( string, "Should not be empty" )
    else
        Ok string


validateComment : String -> Result ( String, String ) String
validateComment =
    stringNotEmpty


validateMatcher : Matcher -> ValidatedMatcher
validateMatcher matcher =
    { name = validateMatcherName matcher.name, value = validateMatcherValue matcher.value, isRegex = matcher.isRegex }


validateMatcherName : String -> Result ( String, String ) String
validateMatcherName =
    stringNotEmpty


validateMatcherValue : String -> Result ( String, String ) String
validateMatcherValue =
    stringNotEmpty


validateDuration : String -> Result ( String, String ) ( String, Time )
validateDuration durationString =
    let
        parsedDuration =
            parseDuration durationString
    in
        case parsedDuration of
            Just d ->
                Ok ( durationString, d )

            Nothing ->
                Err ( durationString, "Invalid duration" )


validateStartsAt : String -> Result ( String, String ) ( String, Time )
validateStartsAt =
    validateDate


validateEndsAt : String -> Result ( String, String ) ( String, Time )
validateEndsAt =
    validateDate
