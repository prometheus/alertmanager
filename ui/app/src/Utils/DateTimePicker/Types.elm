module Utils.DateTimePicker.Types exposing
    ( DateTimePicker(..)
    , InputHourOrMinute(..)
    , Model
    , Msg(..)
    , Settings
    , StartOrEnd(..)
    , Status(..)
    , defaultSettings
    , init
    , initFromMaybePosix
    , openPicker
    )

import Time exposing (Posix, Zone)
import Time.Extra as Time exposing (Interval(..))
import Utils.DateTimePicker.Utils exposing (calculatePickerOffset, floorMinute, pickUpTimeFromDateTimePosix)


type alias Model =
    { status : Status
    , tmpStart : Maybe Posix
    , pickedStart : Maybe Posix
    , timeStart : Maybe Posix
    , tmpEnd : Maybe Posix
    , pickedEnd : Maybe Posix
    , timeEnd : Maybe Posix
    , hovered : Maybe Posix
    , pickerOffset : Int
    }


type DateTimePicker
    = DateTimePicker Model


init : DateTimePicker
init =
    DateTimePicker
        { status = Closed
        , tmpStart = Nothing
        , pickedStart = Nothing
        , timeStart = Nothing
        , tmpEnd = Nothing
        , pickedEnd = Nothing
        , timeEnd = Nothing
        , hovered = Nothing
        , pickerOffset = 0
        }


initFromMaybePosix : Zone -> Maybe Posix -> Maybe Posix -> DateTimePicker
initFromMaybePosix zone start end =
    DateTimePicker
        { status = Closed
        , tmpStart = start
        , pickedStart = start
        , timeStart = pickUpTimeFromDateTimePosix zone start
        , tmpEnd = end
        , pickedEnd = end
        , timeEnd = pickUpTimeFromDateTimePosix zone end
        , hovered = Nothing
        , pickerOffset = 0
        }


type Msg
    = SetRange
    | SetHoveredDay Posix
    | NextMonth
    | PrevMonth
    | NextYear
    | PrevYear
    | SetInputTime StartOrEnd InputHourOrMinute Int
    | Close
    | Cancel
    | IncrementTime StartOrEnd InputHourOrMinute Int


type Status
    = Closed
    | Open Posix


type StartOrEnd
    = Start
    | End


type InputHourOrMinute
    = InputHour
    | InputMinute


type alias Settings msg =
    { zone : Zone
    , internalMsg : ( DateTimePicker, Maybe ( Posix, Posix ) ) -> msg
    }


defaultSettings : Zone -> (( DateTimePicker, Maybe ( Posix, Posix ) ) -> msg) -> Settings msg
defaultSettings zone internalMsg =
    { zone = zone
    , internalMsg = internalMsg
    }


openPicker : Zone -> Maybe Posix -> Maybe Posix -> Maybe Posix -> DateTimePicker -> DateTimePicker
openPicker zone maybeBaseTime start end (DateTimePicker model) =
    let
        baseTime =
            case maybeBaseTime of
                Just time ->
                    time

                Nothing ->
                    case model.pickedStart of
                        Just time ->
                            time

                        Nothing ->
                            Time.millisToPosix 0

        pickerOffset =
            calculatePickerOffset zone baseTime start
    in
    DateTimePicker
        { model
            | status = Open baseTime
            , tmpStart = start
            , pickedStart = floorMinute zone start
            , timeStart = pickUpTimeFromDateTimePosix zone start
            , tmpEnd = end
            , pickedEnd = floorMinute zone end
            , timeEnd = pickUpTimeFromDateTimePosix zone end
            , pickerOffset = pickerOffset
        }
