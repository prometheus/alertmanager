module Utils.DateTimePicker.Utils exposing
    ( addHour
    , addMinute
    , firstDayOfNextMonth
    , firstDayOfPrevMonth
    , floorDate
    , floorMonth
    , listDaysOfMonth
    , monthToString
    , splitWeek
    , targetValueIntParse
    , trimTime
    , updateHour
    , updateMinute
    )

import Html.Events exposing (targetValue)
import Json.Decode as Decode
import Time exposing (Month(..), Posix, Weekday(..), Zone, utc)
import Time.Extra as Time exposing (Interval(..))


listDaysOfMonth : Zone -> Posix -> List Posix
listDaysOfMonth zone time =
    let
        firstOfMonth =
            Time.floor Time.Month zone time

        firstOfNextMonth =
            firstDayOfNextMonth zone time

        padFront =
            weekToInt (Time.toWeekday zone firstOfMonth)
                |> (\wd ->
                        if wd == 7 then
                            0

                        else
                            wd
                   )
                |> (\w -> Time.add Time.Day -w zone firstOfMonth)
                |> (\d -> Time.range Time.Day 1 zone d firstOfMonth)

        padBack =
            weekToInt (Time.toWeekday zone firstOfNextMonth)
                |> (\w -> Time.add Time.Day (7 - w) zone firstOfNextMonth)
                |> Time.range Time.Day 1 zone firstOfNextMonth
    in
    Time.range Time.Day 1 zone firstOfMonth firstOfNextMonth
        |> (\m -> padFront ++ m ++ padBack)


firstDayOfNextMonth : Zone -> Posix -> Posix
firstDayOfNextMonth zone time =
    Time.floor Time.Month zone time
        |> Time.add Time.Day 1 zone
        |> Time.ceiling Time.Month zone


firstDayOfPrevMonth : Zone -> Posix -> Posix
firstDayOfPrevMonth zone time =
    Time.floor Time.Month zone time
        |> Time.add Time.Day -1 zone
        |> Time.floor Time.Month zone


splitWeek : List Posix -> List (List Posix) -> List (List Posix)
splitWeek days weeks =
    if List.length days < 7 then
        weeks

    else
        List.append weeks [ List.take 7 days ]
            |> splitWeek (List.drop 7 days)


floorDate : Zone -> Posix -> Posix
floorDate zone time =
    Time.floor Time.Day zone time


floorMonth : Zone -> Posix -> Posix
floorMonth zone time =
    Time.floor Time.Month zone time


trimTime : Zone -> Posix -> Posix
trimTime zone time =
    Time.floor Time.Day zone time
        |> Time.posixToMillis
        |> (\d ->
                Time.posixToMillis time - d
           )
        |> Time.millisToPosix


updateHour : Zone -> Int -> Posix -> Posix
updateHour zone n time =
    let
        diff =
            n - Time.toHour zone time
    in
    Time.add Hour diff zone time


updateMinute : Zone -> Int -> Posix -> Posix
updateMinute zone n time =
    let
        diff =
            n - Time.toMinute zone time
    in
    Time.add Minute diff zone time


addHour : Zone -> Int -> Posix -> Posix
addHour zone n time =
    Time.add Hour n zone time


addMinute : Zone -> Int -> Posix -> Posix
addMinute zone n time =
    Time.add Minute n zone time


weekToInt : Weekday -> Int
weekToInt weekday =
    case weekday of
        Mon ->
            1

        Tue ->
            2

        Wed ->
            3

        Thu ->
            4

        Fri ->
            5

        Sat ->
            6

        Sun ->
            7


monthToString : Month -> String
monthToString month =
    case month of
        Jan ->
            "January"

        Feb ->
            "February"

        Mar ->
            "March"

        Apr ->
            "April"

        May ->
            "May"

        Jun ->
            "June"

        Jul ->
            "July"

        Aug ->
            "August"

        Sep ->
            "September"

        Oct ->
            "October"

        Nov ->
            "November"

        Dec ->
            "December"


targetValueIntParse : Decode.Decoder Int
targetValueIntParse =
    customDecoder targetValue (String.toInt >> maybeStringToResult)


maybeStringToResult : Maybe a -> Result String a
maybeStringToResult =
    Result.fromMaybe "could not convert string"


customDecoder : Decode.Decoder a -> (a -> Result String b) -> Decode.Decoder b
customDecoder d f =
    let
        resultDecoder x =
            case x of
                Ok a ->
                    Decode.succeed a

                Err e ->
                    Decode.fail e
    in
    Decode.map f d |> Decode.andThen resultDecoder
