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
        , validMatcher
        , emptyMatcher
        )

import Silences.Types exposing (Silence, SilenceId, nullSilenceStatus)
import Alerts.Types exposing (Alert)
import Utils.Types exposing (Matcher, ApiData, Duration, ApiResponse(..))
import Time exposing (Time)
import Utils.Date exposing (timeToString, durationFormat)
import Time exposing (Time)
import Utils.FormValidation
    exposing
        ( initialField
        , validField
        , ValidationState(..)
        , ValidatedField
        , validate
        , stringNotEmpty
        )


initSilenceForm : Model
initSilenceForm =
    { form = empty
    , silence = toSilence empty
    }


toSilence : SilenceForm -> Result ValidationState Silence
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
        form.comment.validationResult
        (List.foldr appendMatcher (Ok []) form.matchers)
        form.createdBy.validationResult
        form.startsAt.validationResult
        form.endsAt.validationResult


fromSilence : Silence -> SilenceForm
fromSilence { id, createdBy, comment, startsAt, endsAt, matchers } =
    { id = id
    , createdBy = validField createdBy identity
    , comment = validField comment identity
    , startsAt = validField startsAt timeToString
    , endsAt = validField endsAt timeToString
    , duration = validField (endsAt - startsAt) durationFormat
    , matchers = List.map validMatcher matchers
    }


empty : SilenceForm
empty =
    { id = ""
    , createdBy = validate stringNotEmpty ""
    , comment = validate stringNotEmpty ""
    , startsAt = initialField
    , endsAt = initialField
    , duration = initialField
    , matchers = []
    }


emptyMatcher : MatcherForm
emptyMatcher =
    { isRegex = False
    , name = initialField
    , value = initialField
    }


fromMatchersAndTime : List Matcher -> Time -> SilenceForm
fromMatchersAndTime matchers now =
    let
        duration =
            2 * Time.hour

        -- If no matchers were specified, show a sample matcher
        enrichedMatchers =
            if List.length matchers == 0 then
                [ Matcher False "env" "production" ]
            else
                matchers
    in
        { empty
            | startsAt = validField now timeToString
            , endsAt = validField (now + duration) timeToString
            , duration = validField duration durationFormat
            , matchers = List.map validMatcher enrichedMatchers
        }


type alias Model =
    { silence : Result ValidationState Silence
    , form : SilenceForm
    }


type alias MatcherForm =
    { name : ValidatedField String
    , value : ValidatedField String
    , isRegex : Bool
    }


appendMatcher : MatcherForm -> Result ValidationState (List Matcher) -> Result ValidationState (List Matcher)
appendMatcher { isRegex, name, value } =
    Result.map2 (::)
        (Result.map2 (Matcher isRegex) name.validationResult value.validationResult)


type alias SilenceForm =
    { id : String
    , createdBy : ValidatedField String
    , comment : ValidatedField String
    , startsAt : ValidatedField Time
    , endsAt : ValidatedField Time
    , duration : ValidatedField Time
    , matchers : List MatcherForm
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


validMatcher : Matcher -> MatcherForm
validMatcher matcher =
    { name = validField matcher.name identity
    , value = validField matcher.value identity
    , isRegex = matcher.isRegex
    }
