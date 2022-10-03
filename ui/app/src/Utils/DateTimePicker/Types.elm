module Utils.DateTimePicker.Types exposing
    ( DateTimePicker
    , InputHourOrMinute(..)
    , Msg(..)
    , StartOrEnd(..)
    , initDateTimePicker
    , initFromStartAndEndTime
    )

import Time exposing (Posix)
import Utils.DateTimePicker.Utils exposing (floorMinute)


type alias DateTimePicker =
    { month : Maybe Posix
    , mouseOverDay : Maybe Posix
    , startDate : Maybe Posix
    , endDate : Maybe Posix
    , startTime : Maybe Posix
    , endTime : Maybe Posix
    , firstDayOfWeek : Int
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


initDateTimePicker : Int -> DateTimePicker
initDateTimePicker firstDayOfWeek =
    { month = Nothing
    , mouseOverDay = Nothing
    , startDate = Nothing
    , endDate = Nothing
    , startTime = Nothing
    , endTime = Nothing
    , firstDayOfWeek = firstDayOfWeek
    }


initFromStartAndEndTime : Maybe Posix -> Maybe Posix -> Int -> DateTimePicker
initFromStartAndEndTime start end firstDayOfWeek =
    let
        startTime =
            Maybe.map (\s -> floorMinute s) start

        endTime =
            Maybe.map (\e -> floorMinute e) end
    in
    { month = start
    , mouseOverDay = Nothing
    , startDate = start
    , endDate = end
    , startTime = startTime
    , endTime = endTime
    , firstDayOfWeek = firstDayOfWeek
    }
