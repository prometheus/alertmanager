module Views.SilenceForm.Types exposing
    ( MatcherForm
    , Model
    , SilenceForm
    , SilenceFormFieldMsg(..)
    , SilenceFormMsg(..)
    , dateTimePickerSettings
    , emptyMatcher
    , fromDateTimePicker
    , fromMatchersAndCommentAndTime
    , fromSilence
    , initSilenceForm
    , openDateTimePicker
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
import Time exposing (Posix, utc)
import Utils.Date exposing (addDuration, durationFormat, parseDuration, timeDifference, timeFromString, timeToString)
import Utils.DateTimePicker.Types exposing (DateTimePicker(..), Settings, defaultSettings, openPicker)
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
    , dateTimePicker : DateTimePicker
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
    | NewSilenceFromMatchersAndComment String Utils.Filter.SilenceFormGetParams
    | NewSilenceFromMatchersAndCommentAndTime String (List Utils.Filter.Matcher) String Posix
    | SilenceFetch (ApiData GettableSilence)
    | SilenceCreate (ApiData String)
    | UpdateDateTimePicker ( DateTimePicker, Maybe ( Posix, Posix ) )
    | OpenDateTimePicker


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
    , dateTimePicker = Utils.DateTimePicker.Types.initFromMaybePosix utc (Just startsAt) (Just endsAt)
    }


validateForm : SilenceForm -> SilenceForm
validateForm { id, createdBy, comment, startsAt, endsAt, duration, matchers } =
    let
        startsAtTime =
            case timeFromString startsAt.value of
                Ok time ->
                    Just time

                _ ->
                    Nothing

        endsAtTime =
            case timeFromString endsAt.value of
                Ok time ->
                    Just time

                _ ->
                    Nothing
    in
    { id = id
    , createdBy = validate stringNotEmpty createdBy
    , comment = validate stringNotEmpty comment
    , startsAt = validate timeFromString startsAt
    , endsAt = validate (parseEndsAt startsAt.value) endsAt
    , duration = validate parseDuration duration
    , matchers = List.map validateMatcherForm matchers
    , dateTimePicker = Utils.DateTimePicker.Types.initFromMaybePosix utc startsAtTime endsAtTime
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
    , dateTimePicker = Utils.DateTimePicker.Types.init
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


fromMatchersAndCommentAndTime : String -> List Utils.Filter.Matcher -> String -> Posix -> SilenceForm
fromMatchersAndCommentAndTime defaultCreator matchers comment now =
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
        , comment = initialField comment
        , dateTimePicker = Utils.DateTimePicker.Types.initFromMaybePosix utc (Just now) (Just (addDuration defaultDuration now))
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


dateTimePickerSettings : Settings SilenceFormMsg
dateTimePickerSettings =
    defaultSettings utc UpdateDateTimePicker


openDateTimePicker : SilenceForm -> SilenceForm
openDateTimePicker { id, createdBy, comment, startsAt, endsAt, duration, matchers, dateTimePicker } =
    let
        startsAtTime =
            case timeFromString startsAt.value of
                Ok time ->
                    Just time

                _ ->
                    Nothing

        endsAtTime =
            case timeFromString endsAt.value of
                Ok time ->
                    Just time

                _ ->
                    Nothing

        newPicker =
            openPicker utc startsAtTime startsAtTime endsAtTime dateTimePicker
    in
    { id = id
    , createdBy = createdBy
    , comment = comment
    , startsAt = startsAt
    , endsAt = endsAt
    , duration = duration
    , matchers = matchers
    , dateTimePicker = newPicker
    }


fromDateTimePicker : SilenceForm -> Maybe ( Posix, Posix ) -> DateTimePicker -> SilenceForm
fromDateTimePicker { id, createdBy, comment, startsAt, endsAt, duration, matchers, dateTimePicker } newTimes newPicker =
    let
        startsAtPicker =
            case newTimes of
                Just ( start, end ) ->
                    validate timeFromString (initialField (timeToString start))

                Nothing ->
                    startsAt

        endsAtPicker =
            case newTimes of
                Just ( start, end ) ->
                    validate (parseEndsAt (timeToString start)) (initialField (timeToString end))

                -- validate timeFromString (initialField (timeToString end))
                Nothing ->
                    endsAt

        durationPicker =
            case newTimes of
                Just ( start, end ) ->
                    initialField (durationFormat (timeDifference start end) |> Maybe.withDefault "")
                        |> validate parseDuration

                Nothing ->
                    duration
    in
    { id = id
    , createdBy = createdBy
    , comment = comment
    , startsAt = startsAtPicker
    , endsAt = endsAtPicker
    , duration = durationPicker
    , matchers = matchers
    , dateTimePicker = newPicker
    }
