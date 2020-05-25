module Views.SilenceForm.Types exposing
    ( MatcherForm
    , Model
    , SilenceForm
    , SilenceFormFieldMsg(..)
    , SilenceFormMsg(..)
    , datePickerSettings
    , emptyMatcher
    , fromMatchersAndCommentAndTime
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
import Date exposing (Date, fromPosix)
import DatePicker exposing (defaultSettings)
import DateTime exposing (DateTime)
import Html.Attributes exposing (hidden)
import Silences.Types exposing (nullSilence)
import Time exposing (Posix, utc)
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
    , startsAtDate : Maybe Date
    , startsAtDatePicker : DatePicker.DatePicker
    , endsAtDate : Maybe Date
    , endsAtDatePicker : DatePicker.DatePicker
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
    | StartsAtDatePicker DatePicker.Msg
    | EndsAtDatePicker DatePicker.Msg
    | StartsAtDatePickerDisplayAction
    | EndsAtDatePickerDisplayAction


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
        date =
            fromPosix utc startsAt

        maybeDate =
            Just date

        datePicker =
            DatePicker.initFromDate date
    in
    { id = Just id
    , createdBy = initialField createdBy
    , comment = initialField comment
    , startsAt = initialField (timeToString startsAt)
    , endsAt = initialField (timeToString endsAt)
    , duration = initialField (durationFormat (timeDifference startsAt endsAt) |> Maybe.withDefault "")
    , matchers = List.map fromMatcher matchers
    , startsAtDate = maybeDate
    , startsAtDatePicker = datePicker
    , endsAtDate = maybeDate
    , endsAtDatePicker = datePicker
    }


validateForm : SilenceForm -> SilenceForm
validateForm { id, createdBy, comment, startsAt, endsAt, duration, matchers, startsAtDate, startsAtDatePicker, endsAtDate, endsAtDatePicker } =
    { id = id
    , createdBy = validate stringNotEmpty createdBy
    , comment = validate stringNotEmpty comment
    , startsAt = validate timeFromString startsAt
    , endsAt = validate (parseEndsAt startsAt.value) endsAt
    , duration = validate parseDuration duration
    , matchers = List.map validateMatcherForm matchers
    , startsAtDate = startsAtDate
    , startsAtDatePicker = startsAtDatePicker
    , endsAtDate = endsAtDate
    , endsAtDatePicker = endsAtDatePicker
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
    let
        ( datePicker, _ ) =
            DatePicker.init
    in
    { id = Nothing
    , createdBy = initialField ""
    , comment = initialField ""
    , startsAt = initialField ""
    , endsAt = initialField ""
    , duration = initialField ""
    , matchers = []
    , startsAtDate = Nothing
    , startsAtDatePicker = datePicker
    , endsAtDate = Nothing
    , endsAtDatePicker = datePicker
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
    let
        datePicker =
            DatePicker.initFromDate (fromPosix utc now)

        maybeDate =
            Just (fromPosix utc now)
    in
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
        , startsAtDate = maybeDate
        , startsAtDatePicker = datePicker
        , endsAtDate = maybeDate
        , endsAtDatePicker = datePicker
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


datePickerSettings : DatePicker.Settings
datePickerSettings =
    let
        inputAttributes =
            [ hidden True ]

        containerClassList =
            [ ( "col-5", True ) ]

        placeholder =
            ""
    in
    { defaultSettings
        | inputAttributes = inputAttributes
        , containerClassList = containerClassList
        , placeholder = placeholder
    }
