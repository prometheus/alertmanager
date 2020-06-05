module Views.SilenceForm.Types exposing
    ( MatcherForm
    , Model
    , SilenceForm
    , SilenceFormFieldMsg(..)
    , SilenceFormMsg(..)
    , calendarConfig
    , emptyMatcher
    , fromMatchersAndCommentAndTime
    , fromSilence
    , initSilenceForm
    , parseEndsAt
    , timePickerConfig
    , toSilence
    , validateForm
    )

import Browser.Navigation exposing (Key)
import Clock
import Data.GettableAlert exposing (GettableAlert)
import Data.GettableSilence exposing (GettableSilence)
import Data.Matcher exposing (Matcher)
import Data.PostableSilence exposing (PostableSilence)
import DatePicker
import DatePicker.Types exposing (DateLimit(..), ViewType(..))
import DateTime exposing (DateTime)
import DateTimeLocal
import Html.Attributes exposing (hidden)
import Iso8601
import Json.Decode as Decode
import Silences.Types exposing (nullSilence)
import Time exposing (Posix, utc)
import TimePicker.Types as TimePicker
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
    , showStartsAtPicker : Bool
    , startsAtDateTime : Maybe DateTime
    , startsAtPicker : DatePicker.Model
    , showEndsAtPicker : Bool
    , endsAtDateTime : Maybe DateTime
    , endsAtPicker : DatePicker.Model
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
    | StartsAtPicker DatePicker.Msg
    | UpdateStartsAtFromDatePicker
    | EndsAtPicker DatePicker.Msg
    | UpdateEndsAtFromDatePicker
    | OpenStartsAtPicker
    | OpenEndsAtPicker
    | CloseStartsAtPicker
    | CloseEndsAtPicker


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
    let
        startsPosix =
            case Utils.Date.timeFromString (DateTimeLocal.toString startsAt) of
                Ok starts ->
                    Just starts

                _ ->
                    Nothing

        endsPosix =
            case Utils.Date.timeFromString (DateTimeLocal.toString endsAt) of
                Ok ends ->
                    Just ends

                _ ->
                    Nothing
    in
    { id = Just id
    , createdBy = initialField createdBy
    , comment = initialField comment
    , startsAt = initialField (timeToString startsAt)
    , endsAt = initialField (timeToString endsAt)
    , duration = initialField (durationFormat (timeDifference startsAt endsAt) |> Maybe.withDefault "")
    , matchers = List.map fromMatcher matchers
    , showStartsAtPicker = False
    , startsAtDateTime = Nothing
    , startsAtPicker = DatePicker.initialise Single (calendarConfig startsPosix) (timePickerConfig startsPosix)
    , showEndsAtPicker = False
    , endsAtDateTime = Nothing
    , endsAtPicker = DatePicker.initialise Single (calendarConfig endsPosix) (timePickerConfig endsPosix)
    }


validateForm : SilenceForm -> SilenceForm
validateForm { id, createdBy, comment, startsAt, endsAt, duration, matchers, showStartsAtPicker, startsAtDateTime, startsAtPicker, showEndsAtPicker, endsAtDateTime, endsAtPicker } =
    { id = id
    , createdBy = validate stringNotEmpty createdBy
    , comment = validate stringNotEmpty comment
    , startsAt = validate timeFromString startsAt
    , endsAt = validate (parseEndsAt startsAt.value) endsAt
    , duration = validate parseDuration duration
    , matchers = List.map validateMatcherForm matchers
    , showStartsAtPicker = showStartsAtPicker
    , startsAtDateTime = startsAtDateTime
    , startsAtPicker = startsAtPicker
    , showEndsAtPicker = showEndsAtPicker
    , endsAtDateTime = endsAtDateTime
    , endsAtPicker = endsAtPicker
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
    , showStartsAtPicker = False
    , startsAtDateTime = Nothing
    , startsAtPicker = DatePicker.initialise Single (calendarConfig Nothing) (timePickerConfig Nothing)
    , showEndsAtPicker = False
    , endsAtDateTime = Nothing
    , endsAtPicker = DatePicker.initialise Single (calendarConfig Nothing) (timePickerConfig Nothing)
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
        , showStartsAtPicker = False
        , startsAtDateTime = Just (DateTime.fromPosix now)
        , startsAtPicker = DatePicker.initialise Single (Just now |> calendarConfig) (Just now |> timePickerConfig)
        , showEndsAtPicker = False
        , endsAtDateTime = Nothing
        , endsAtPicker = DatePicker.initialise Single (Just now |> calendarConfig) (Just now |> timePickerConfig)
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


incrementMonths : Int -> DateTime -> DateTime
incrementMonths count dateTime =
    if count > 0 then
        incrementMonths (count - 1) (DateTime.incrementMonth dateTime)

    else
        dateTime


decrementMonths : Int -> DateTime -> DateTime
decrementMonths count dateTime =
    if count > 0 then
        decrementMonths (count - 1) (DateTime.decrementMonth dateTime)

    else
        dateTime


calendarConfig : Maybe Posix -> DatePicker.Types.CalendarConfig
calendarConfig maybeTime =
    let
        today =
            case maybeTime of
                Just time ->
                    DateTime.fromPosix time

                Nothing ->
                    DateTime.fromPosix (Time.millisToPosix 0)

        ( minDate, maxDate ) =
            let
                ( past, future ) =
                    ( decrementMonths 6 today
                    , incrementMonths 6 today
                    )
            in
            ( Maybe.withDefault past <| DateTime.setDay 1 past
            , Maybe.withDefault future <| DateTime.setDay (DateTime.lastDayOf future) future
            )
    in
    { today = today
    , primaryDate = Nothing
    , dateLimit = DateLimit { minDate = minDate, maxDate = maxDate }
    }


timePickerConfig : Maybe Posix -> Maybe DatePicker.Types.TimePickerConfig
timePickerConfig maybeTime =
    Just
        { pickerType = TimePicker.HH_MM { hoursStep = 1, minutesStep = 1 }
        , defaultTime =
            case maybeTime of
                Just time ->
                    Clock.fromPosix time

                Nothing ->
                    Clock.midnight
        , pickerTitle = ""
        }
