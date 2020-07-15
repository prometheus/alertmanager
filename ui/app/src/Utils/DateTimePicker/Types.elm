module Utils.DateTimePicker.Types exposing
    ( DateTimePicker
    , InputHourOrMinute(..)
    , Msg(..)
    , PickerConfig
    , StartOrEnd(..)
    , defaultPickerConfig
    , initDateTimePicker
    , initFromStartAndEndTime
    )

import Time exposing (Posix, Zone)


type alias DateTimePicker =
    { month : Maybe Posix
    , mouseOverDay : Maybe Posix
    , startDate : Maybe Posix
    , endDate : Maybe Posix
    , startTime : Maybe Posix
    , endTime : Maybe Posix
    }


type alias PickerConfig msg =
    { zone : Zone
    , pickerMsg : ( DateTimePicker, Maybe ( Posix, Posix ) ) -> msg
    }


type Msg
    = NextMonth
    | PrevMonth
    | MouseOverDay Posix
    | OnClickDay
    | ClearMouseOverDay
    | SetInputTime StartOrEnd InputHourOrMinute Int
    | IncrementTime StartOrEnd InputHourOrMinute Int


type StartOrEnd
    = Start
    | End


type InputHourOrMinute
    = InputHour
    | InputMinute


defaultPickerConfig : Zone -> (( DateTimePicker, Maybe ( Posix, Posix ) ) -> msg) -> PickerConfig msg
defaultPickerConfig zone pickerMsg =
    { zone = zone
    , pickerMsg = pickerMsg
    }


initDateTimePicker : DateTimePicker
initDateTimePicker =
    { month = Nothing
    , mouseOverDay = Nothing
    , startDate = Nothing
    , endDate = Nothing
    , startTime = Nothing
    , endTime = Nothing
    }


initFromStartAndEndTime : Zone -> Maybe Posix -> Maybe Posix -> DateTimePicker
initFromStartAndEndTime zone start end =
    { month = start
    , mouseOverDay = Nothing
    , startDate = start
    , endDate = end
    , startTime = start
    , endTime = end
    }
