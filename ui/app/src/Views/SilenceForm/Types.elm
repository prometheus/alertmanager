module Views.SilenceForm.Types exposing
    ( MatcherForm
    , Model
    , SilenceForm
    , SilenceFormFieldMsg(..)
    , SilenceFormMsg(..)
    , emptyMatcher
    , fromMatchersAndTime
    , fromSilence
    , initSilenceForm
    , parseEndsAt
    , toSilence
    , validateForm
    )

import Browser.Navigation exposing (Key)
import Data.GettableAlert exposing (GettableAlert)
import Data.GettableSilence exposing (GettableSilence)
import Data.Matcher exposing (Matcher)
import Data.PostableSilence exposing (PostableSilence)
import Silences.Types exposing (nullSilence)
import Time exposing (Posix)
import Utils.Date exposing (addDuration, durationFormat, parseDuration, timeDifference, timeFromString, timeToString)
import Utils.Filter
import Utils.FormValidation
    exposing
        ( ValidatedField
        , ValidationState(..)
        , initialField
        , stringNotEmpty
        , validate
        )
import Utils.Types exposing (ApiData(..), Duration)


type alias Model =
    { form : SilenceForm
    , silenceId : ApiData String
    , alerts : ApiData (List GettableAlert)
    , activeAlertId : Maybe String
    , key : Key
    }


type alias SilenceForm =
    { id : Maybe String
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
    | AlertGroupsPreview (ApiData (List GettableAlert))
    | SetActiveAlert (Maybe String)
    | FetchSilence String
    | NewSilenceFromMatchers String (List Utils.Filter.Matcher)
    | NewSilenceFromMatchersAndTime String (List Utils.Filter.Matcher) Posix
    | SilenceFetch (ApiData GettableSilence)
    | SilenceCreate (ApiData String)


type SilenceFormFieldMsg
    = AddMatcher
    | DeleteMatcher Int
    | UpdateStartsAt String
    | UpdateEndsAt String
    | UpdateDuration String
    | ValidateTime
    | UpdateCreatedBy String
    | ValidateCreatedBy
    | UpdateComment String
    | ValidateComment
    | UpdateMatcherName Int String
    | ValidateMatcherName Int
    | UpdateMatcherValue Int String
    | ValidateMatcherValue Int
    | UpdateMatcherRegex Int Bool


initSilenceForm : Key -> Model
initSilenceForm key =
    { form = empty
    , silenceId = Utils.Types.Initial
    , alerts = Utils.Types.Initial
    , activeAlertId = Nothing
    , key = key
    }


toSilence : SilenceForm -> Maybe PostableSilence
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
        (parseEndsAt startsAt.value endsAt.value)
        |> Result.toMaybe


fromSilence : GettableSilence -> SilenceForm
fromSilence { id, createdBy, comment, startsAt, endsAt, matchers } =
    { id = Just id
    , createdBy = initialField createdBy
    , comment = initialField comment
    , startsAt = initialField (timeToString startsAt)
    , endsAt = initialField (timeToString endsAt)
    , duration = initialField (durationFormat (timeDifference startsAt endsAt) |> Maybe.withDefault "")
    , matchers = List.map fromMatcher matchers
    }


validateForm : SilenceForm -> SilenceForm
validateForm { id, createdBy, comment, startsAt, endsAt, duration, matchers } =
    { id = id
    , createdBy = validate stringNotEmpty createdBy
    , comment = validate stringNotEmpty comment
    , startsAt = validate timeFromString startsAt
    , endsAt = validate (parseEndsAt startsAt.value) endsAt
    , duration = validate parseDuration duration
    , matchers = List.map validateMatcherForm matchers
    }


parseEndsAt : String -> String -> Result String Posix
parseEndsAt startsAt endsAt =
    case ( timeFromString startsAt, timeFromString endsAt ) of
        ( Ok starts, Ok ends ) ->
            if Time.posixToMillis starts > Time.posixToMillis ends then
                Err "Can't be in the past"

            else
                Ok ends

        ( _, endsResult ) ->
            endsResult


validateMatcherForm : MatcherForm -> MatcherForm
validateMatcherForm { name, value, isRegex } =
    { name = validate stringNotEmpty name
    , value = value
    , isRegex = isRegex
    }


empty : SilenceForm
empty =
    { id = Nothing
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


defaultDuration : Float
defaultDuration =
    -- 2 hours
    2 * 60 * 60 * 1000


fromMatchersAndTime : String -> List Utils.Filter.Matcher -> Posix -> SilenceForm
fromMatchersAndTime defaultCreator matchers now =
    { empty
        | startsAt = initialField (timeToString now)
        , endsAt = initialField (timeToString (addDuration defaultDuration now))
        , duration = initialField (durationFormat defaultDuration |> Maybe.withDefault "")
        , createdBy = initialField defaultCreator
        , matchers =
            -- If no matchers were specified, add an empty row
            if List.isEmpty matchers then
                [ emptyMatcher ]

            else
                List.filterMap (filterMatcherToMatcher >> Maybe.map fromMatcher) matchers
    }


appendMatcher : MatcherForm -> Result String (List Matcher) -> Result String (List Matcher)
appendMatcher { isRegex, name, value } =
    Result.map2 (::)
        (Result.map2 (\k v -> Matcher k v isRegex) (stringNotEmpty name.value) (Ok value.value))


filterMatcherToMatcher : Utils.Filter.Matcher -> Maybe Matcher
filterMatcherToMatcher { key, op, value } =
    Maybe.map (\operator -> Matcher key value operator) <|
        case op of
            Utils.Filter.Eq ->
                Just False

            Utils.Filter.RegexMatch ->
                Just True

            -- we don't support negative matchers
            _ ->
                Nothing


fromMatcher : Matcher -> MatcherForm
fromMatcher { name, value, isRegex } =
    { name = initialField name
    , value = initialField value
    , isRegex = isRegex
    }
