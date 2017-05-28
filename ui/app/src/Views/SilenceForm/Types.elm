module Views.SilenceForm.Types
    exposing
        ( SilenceFormMsg(..)
        , SilenceFormFieldMsg(..)
        , Model
        , SilenceForm
        , MatcherForm
        , fromMatchersAndTime
        , fromSilence
        , toSilence
        , initSilenceForm
        , emptyMatcher
        , validateForm
        )

import Silences.Types exposing (Silence, SilenceId, nullSilence)
import Alerts.Types exposing (Alert)
import Utils.Types exposing (Matcher, Duration, ApiData(..))
import Time exposing (Time)
import Utils.Date exposing (timeFromString, timeToString, durationFormat, parseDuration)
import Time exposing (Time)
import Utils.FormValidation
    exposing
        ( initialField
        , ValidationState(..)
        , ValidatedField
        , validate
        , stringNotEmpty
        )


type alias Model =
    { form : SilenceForm
    , silenceId : ApiData String
    , alerts : ApiData (List Alert)
    }


type alias SilenceForm =
    { id : String
    , createdBy : ValidatedField
    , comment : ValidatedField
    , startsAt : ValidatedField
    , endsAt : ValidatedField
    , duration : ValidatedField
    , matchers : List MatcherForm
    }


type alias MatcherForm =
    { name : ValidatedField
    , value : ValidatedField
    , isRegex : Bool
    }


type SilenceFormMsg
    = UpdateField SilenceFormFieldMsg
    | CreateSilence
    | PreviewSilence
    | AlertGroupsPreview (ApiData (List Alert))
    | FetchSilence String
    | NewSilenceFromMatchers (List Matcher)
    | NewSilenceFromMatchersAndTime (List Matcher) Time
    | SilenceFetch (ApiData Silence)
    | SilenceCreate (ApiData SilenceId)


type SilenceFormFieldMsg
    = AddMatcher
    | DeleteMatcher Int
    | UpdateStartsAt String
    | ValidateStartsAt
    | UpdateEndsAt String
    | ValidateEndsAt
    | UpdateDuration String
    | ValidateDuration
    | UpdateCreatedBy String
    | ValidateCreatedBy
    | UpdateComment String
    | ValidateComment
    | UpdateMatcherName Int String
    | ValidateMatcherName Int
    | UpdateMatcherValue Int String
    | ValidateMatcherValue Int
    | UpdateMatcherRegex Int Bool


initSilenceForm : Model
initSilenceForm =
    { form = empty
    , silenceId = Utils.Types.Initial
    , alerts = Utils.Types.Initial
    }


toSilence : SilenceForm -> Maybe Silence
toSilence { id, comment, matchers, createdBy, startsAt, endsAt } =
    Result.map5
        (\nonEmptyComment validMatchers nonEmptyCreatedBy parsedStartsAt parsedEndsAt ->
            { nullSilence
                | id = id
                , comment = nonEmptyComment
                , matchers = validMatchers
                , createdBy = nonEmptyCreatedBy
                , startsAt = parsedStartsAt
                , endsAt = parsedEndsAt
            }
        )
        (stringNotEmpty comment.value)
        (List.foldr appendMatcher (Ok []) matchers)
        (stringNotEmpty createdBy.value)
        (timeFromString startsAt.value)
        (timeFromString endsAt.value)
        |> Result.toMaybe


fromSilence : Silence -> SilenceForm
fromSilence { id, createdBy, comment, startsAt, endsAt, matchers } =
    { id = id
    , createdBy = initialField createdBy
    , comment = initialField comment
    , startsAt = initialField (timeToString startsAt)
    , endsAt = initialField (timeToString endsAt)
    , duration = initialField (durationFormat (endsAt - startsAt))
    , matchers = List.map fromMatcher matchers
    }


validateForm : SilenceForm -> SilenceForm
validateForm { id, createdBy, comment, startsAt, endsAt, duration, matchers } =
    { id = id
    , createdBy = validate stringNotEmpty createdBy
    , comment = validate stringNotEmpty comment
    , startsAt = validate timeFromString startsAt
    , endsAt = validate timeFromString endsAt
    , duration = validate parseDuration duration
    , matchers = List.map validateMatcherForm matchers
    }


validateMatcherForm : MatcherForm -> MatcherForm
validateMatcherForm { name, value, isRegex } =
    { name = validate stringNotEmpty name
    , value = validate stringNotEmpty value
    , isRegex = isRegex
    }


empty : SilenceForm
empty =
    { id = ""
    , createdBy = initialField ""
    , comment = initialField ""
    , startsAt = initialField ""
    , endsAt = initialField ""
    , duration = initialField ""
    , matchers = []
    }


emptyMatcher : MatcherForm
emptyMatcher =
    { isRegex = False
    , name = initialField ""
    , value = initialField ""
    }


defaultDuration : Time
defaultDuration =
    2 * Time.hour


fromMatchersAndTime : List Matcher -> Time -> SilenceForm
fromMatchersAndTime matchers now =
    { empty
        | startsAt = initialField (timeToString now)
        , endsAt = initialField (timeToString (now + defaultDuration))
        , duration = initialField (durationFormat defaultDuration)
        , matchers =
            -- If no matchers were specified, add an empty row
            if List.isEmpty matchers then
                [ emptyMatcher ]
            else
                List.map fromMatcher matchers
    }


appendMatcher : MatcherForm -> Result String (List Matcher) -> Result String (List Matcher)
appendMatcher { isRegex, name, value } =
    Result.map2 (::)
        (Result.map2 (Matcher isRegex) (stringNotEmpty name.value) (stringNotEmpty value.value))


fromMatcher : Matcher -> MatcherForm
fromMatcher { name, value, isRegex } =
    { name = initialField name
    , value = initialField value
    , isRegex = isRegex
    }
