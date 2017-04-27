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
        )

import Silences.Types exposing (Silence, SilenceId)
import Alerts.Types exposing (Alert)
import Utils.Types exposing (Matcher, ApiData, Duration, ApiResponse(..))
import Time exposing (Time)
import Utils.Date exposing (timeToString, timeFromString, durationFormat)


initSilenceForm : Model
initSilenceForm =
    { form = empty
    , silence = Err "Empty"
    }


toSilence : SilenceForm -> Result String Silence
toSilence { createdBy, comment, startsAt, endsAt, matchers } =
    Maybe.map2
        (\parsedStartsAt parsedEndsAt ->
            { comment = comment
            , matchers = matchers
            , createdBy = createdBy
            , startsAt = parsedStartsAt
            , endsAt = parsedEndsAt

            {- ignored -}
            , silencedAlerts = Success []

            {- ignored -}
            , updatedAt = 0

            {- ignored -}
            , id = ""
            }
        )
        (timeFromString startsAt)
        (timeFromString endsAt)
        |> Result.fromMaybe "Wrong datetime format"


fromSilence : Silence -> SilenceForm
fromSilence { createdBy, comment, startsAt, endsAt, matchers } =
    { createdBy = createdBy
    , comment = comment
    , startsAt = timeToString startsAt
    , endsAt = timeToString endsAt
    , duration = durationFormat (endsAt - startsAt)
    , matchers = matchers
    }


empty : SilenceForm
empty =
    { createdBy = ""
    , comment = ""
    , startsAt = ""
    , endsAt = ""
    , duration = ""
    , matchers = []
    }


fromMatchersAndTime : List Matcher -> Time -> SilenceForm
fromMatchersAndTime matchers now =
    let
        duration =
            2 * Time.hour
    in
        { empty
            | startsAt = timeToString now
            , endsAt = timeToString (now + duration)
            , duration = durationFormat duration
            , matchers = matchers
        }


type alias Model =
    { silence : Result String Silence
    , form : SilenceForm
    }


type alias SilenceForm =
    { createdBy : String
    , comment : String
    , startsAt : String
    , endsAt : String
    , duration : String
    , matchers : List Matcher
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
