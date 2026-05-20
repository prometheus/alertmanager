module Views.SilenceForm.Types exposing
    ( Model
    , SilenceForm
    , SilenceFormFieldMsg(..)
    , SilenceFormMsg(..)
    , fromDateTimePicker
    , fromMatchersAndCommentAndTime
    , fromSilence
    , hasAnnotationKey
    , initSilenceForm
    , parseAnnotation
    , parseEndsAt
    , toSilence
    , validateAnnotations
    , validateForm
    , validateMatchers
    )

import Browser.Navigation exposing (Key)
import Data.GettableAlert exposing (GettableAlert)
import Data.GettableSilence exposing (GettableSilence)
import Data.Matcher
import Data.PostableSilence exposing (PostableSilence)
import DateTime
import Dict
import Silences.Types exposing (nullSilence)
import Time exposing (Posix)
import Utils.Date exposing (addDuration, durationFormat, parseDuration, timeDifference, timeFromString, timeToString)
import Utils.DateTimePicker.Types exposing (DateTimePicker, initDateTimePicker, initFromStartAndEndTime)
import Utils.DateTimePicker.Utils exposing (FirstDayOfWeek)
import Utils.Filter
import Utils.FormValidation
    exposing
        ( ValidatedField
        , ValidationState(..)
        , initialField
        , stringNotEmpty
        , validate
        )
import Utils.Types exposing (ApiData(..))
import Views.FilterBar.Types as FilterBar


type alias Model =
    { form : SilenceForm
    , filterBar : FilterBar.Model
    , filterBarValid : ValidationState
    , annotationsValid : ValidationState
    , silenceId : ApiData String
    , alerts : ApiData (List GettableAlert)
    , activeAlertId : Maybe String
    , key : Key
    , firstDayOfWeek : FirstDayOfWeek
    }


type alias SilenceForm =
    { id : Maybe String
    , createdBy : ValidatedField
    , comment : ValidatedField
    , startsAt : ValidatedField
    , endsAt : ValidatedField
    , duration : ValidatedField
    , dateTimePicker : DateTimePicker
    , viewDateTimePicker : Bool
    , annotations : List ( String, String )
    , annotationText : String
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
    | UpdateDateTimePicker Utils.DateTimePicker.Types.Msg
    | MsgForFilterBar FilterBar.Msg
    | UpdateFirstDayOfWeek FirstDayOfWeek


type SilenceFormFieldMsg
    = UpdateStartsAt String
    | UpdateEndsAt String
    | UpdateDuration String
    | ValidateTime
    | UpdateCreatedBy String
    | ValidateCreatedBy
    | UpdateComment String
    | ValidateComment
    | UpdateTimesFromPicker
    | OpenDateTimePicker
    | CloseDateTimePicker
    | UpdateAnnotationText String
    | AddAnnotation
    | DeleteAnnotation Bool ( String, String )
    | Noop


initSilenceForm : Key -> FirstDayOfWeek -> Model
initSilenceForm key firstDayOfWeek =
    { form = empty firstDayOfWeek
    , filterBar = FilterBar.initFilterBar []
    , filterBarValid = Utils.FormValidation.Initial
    , annotationsValid = Utils.FormValidation.Valid
    , silenceId = Utils.Types.Initial
    , alerts = Utils.Types.Initial
    , activeAlertId = Nothing
    , key = key
    , firstDayOfWeek = firstDayOfWeek
    }


toSilence : FilterBar.Model -> SilenceForm -> Maybe PostableSilence
toSilence filterBar { id, comment, createdBy, startsAt, endsAt, annotations } =
    Result.map5
        (\nonEmptyMatchers nonEmptyComment nonEmptyCreatedBy parsedStartsAt parsedEndsAt ->
            { nullSilence
                | id = id
                , comment = nonEmptyComment
                , matchers = nonEmptyMatchers
                , createdBy = nonEmptyCreatedBy
                , startsAt = parsedStartsAt
                , endsAt = parsedEndsAt
                , annotations =
                    if List.isEmpty annotations then
                        Nothing

                    else
                        Just (Dict.fromList annotations)
            }
        )
        (validMatchers filterBar)
        (stringNotEmpty comment.value)
        (stringNotEmpty createdBy.value)
        (timeFromString startsAt.value)
        (parseEndsAt startsAt.value endsAt.value)
        |> Result.toMaybe


validMatchers : FilterBar.Model -> Result String (List Data.Matcher.Matcher)
validMatchers { matchers, matcherText } =
    if matcherText /= "" then
        Err "Please complete adding the matcher"

    else
        case matchers of
            [] ->
                Err "Matchers are required"

            nonEmptyMatchers ->
                Ok (List.map Utils.Filter.toApiMatcher nonEmptyMatchers)


parseAnnotation : String -> Maybe ( String, String )
parseAnnotation text =
    -- Split on the first equals sign only, allowing values to contain "="
    case String.indices "=" text of
        firstIndex :: _ ->
            let
                key =
                    String.left firstIndex text

                value =
                    String.dropLeft (firstIndex + 1) text
            in
            if String.isEmpty (String.trim key) || String.isEmpty (String.trim value) then
                Nothing

            else
                Just ( String.trim key, String.trim value )

        [] ->
            Nothing


hasAnnotationKey : String -> List ( String, String ) -> Bool
hasAnnotationKey key annotations =
    List.any (\( k, _ ) -> k == key) annotations


validateAnnotations : SilenceForm -> ValidationState
validateAnnotations { annotationText, annotations } =
    if annotationText == "" then
        Utils.FormValidation.Valid

    else
        case parseAnnotation annotationText of
            Just ( key, _ ) ->
                if hasAnnotationKey key annotations then
                    Utils.FormValidation.Invalid ("Key '" ++ key ++ "' already exists. Duplicate keys will result in only the last value being retained.")

                else
                    Utils.FormValidation.Valid

            Nothing ->
                Utils.FormValidation.Invalid "Please complete adding the annotation or clear the field"


fromSilence : GettableSilence -> FirstDayOfWeek -> SilenceForm
fromSilence { id, createdBy, comment, startsAt, endsAt, annotations } firstDayOfWeek =
    let
        startsPosix =
            Utils.Date.timeFromString (DateTime.toString startsAt)
                |> Result.toMaybe

        endsPosix =
            Utils.Date.timeFromString (DateTime.toString endsAt)
                |> Result.toMaybe
    in
    { id = Just id
    , createdBy = initialField createdBy
    , comment = initialField comment
    , startsAt = initialField (timeToString startsAt)
    , endsAt = initialField (timeToString endsAt)
    , duration = initialField (durationFormat (timeDifference startsAt endsAt) |> Maybe.withDefault "")
    , dateTimePicker = initFromStartAndEndTime startsPosix endsPosix firstDayOfWeek
    , viewDateTimePicker = False
    , annotations = annotations |> Maybe.map Dict.toList |> Maybe.withDefault []
    , annotationText = ""
    }


validateForm : SilenceForm -> SilenceForm
validateForm { id, createdBy, comment, startsAt, endsAt, duration, dateTimePicker, annotations, annotationText } =
    { id = id
    , createdBy = validate stringNotEmpty createdBy
    , comment = validate stringNotEmpty comment
    , startsAt = validate timeFromString startsAt
    , endsAt = validate (parseEndsAt startsAt.value) endsAt
    , duration = validate parseDuration duration
    , dateTimePicker = dateTimePicker
    , viewDateTimePicker = False
    , annotations = annotations
    , annotationText = annotationText
    }


validateMatchers : FilterBar.Model -> ValidationState
validateMatchers filter =
    case validMatchers filter of
        Err error ->
            Utils.FormValidation.Invalid error

        Ok _ ->
            Utils.FormValidation.Valid


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


empty : FirstDayOfWeek -> SilenceForm
empty firstDayOfWeek =
    { id = Nothing
    , createdBy = initialField ""
    , comment = initialField ""
    , startsAt = initialField ""
    , endsAt = initialField ""
    , duration = initialField ""
    , dateTimePicker = initDateTimePicker firstDayOfWeek
    , viewDateTimePicker = False
    , annotations = []
    , annotationText = ""
    }


defaultDuration : Float
defaultDuration =
    -- 2 hours
    2 * 60 * 60 * 1000


fromMatchersAndCommentAndTime : String -> String -> Posix -> FirstDayOfWeek -> SilenceForm
fromMatchersAndCommentAndTime defaultCreator comment now firstDayOfWeek =
    { id = Nothing
    , startsAt = initialField (timeToString now)
    , endsAt = initialField (timeToString (addDuration defaultDuration now))
    , duration = initialField (durationFormat defaultDuration |> Maybe.withDefault "")
    , createdBy = initialField defaultCreator
    , comment = initialField comment
    , dateTimePicker = initFromStartAndEndTime (Just now) (Just (addDuration defaultDuration now)) firstDayOfWeek
    , viewDateTimePicker = False
    , annotations = []
    , annotationText = ""
    }


fromDateTimePicker : SilenceForm -> DateTimePicker -> SilenceForm
fromDateTimePicker { id, createdBy, comment, startsAt, endsAt, duration, annotations, annotationText } newPicker =
    { id = id
    , createdBy = createdBy
    , comment = comment
    , startsAt = startsAt
    , endsAt = endsAt
    , duration = duration
    , dateTimePicker = newPicker
    , viewDateTimePicker = True
    , annotations = annotations
    , annotationText = annotationText
    }
