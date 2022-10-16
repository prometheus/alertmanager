module Utils.DateTimePicker.Utils exposing
    ( FirstDayOfWeek(..)
    , addHour
    , addMinute
    , firstDayOfNextMonth
    , firstDayOfPrevMonth
    , floorDate
    , floorMinute
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
import Time exposing (Month(..), Posix, Weekday(..), utc)
import Time.Extra as Time exposing (Interval(..))


type FirstDayOfWeek
    = Monday
    | Sunday


listDaysOfMonth : Posix -> FirstDayOfWeek -> List Posix
listDaysOfMonth time firstDayOfWeek =
    let
        firstOfMonth =
            Time.floor Time.Month utc time

        firstOfNextMonth =
            firstDayOfNextMonth time

        padFront =
            weekToInt (Time.toWeekday utc firstOfMonth)
                |> (\wd ->
                        case firstDayOfWeek of
                            Sunday ->
                                if wd == 7 then
                                    0

                                else
                                    wd

                            Monday ->
                                if wd == 1 then
                                    0

                                else
                                    wd - 1
                   )
                |> (\w -> Time.add Time.Day -w utc firstOfMonth)
                |> (\d -> Time.range Time.Day 1 utc d firstOfMonth)

        padBack =
            weekToInt (Time.toWeekday utc firstOfNextMonth)
                |> (\wd ->
                        case firstDayOfWeek of
                            Sunday ->
                                wd

                            Monday ->
                                if wd == 1 then
                                    7

                                else
                                    wd - 1
                   )
                |> (\w -> Time.add Time.Day (7 - w) utc firstOfNextMonth)
                |> Time.range Time.Day 1 utc firstOfNextMonth
    in
    Time.range Time.Day 1 utc firstOfMonth firstOfNextMonth
        |> (\m -> padFront ++ m ++ padBack)


firstDayOfNextMonth : Posix -> Posix
firstDayOfNextMonth time =
    Time.floor Time.Month utc time
        |> Time.add Time.Day 1 utc
        |> Time.ceiling Time.Month utc


firstDayOfPrevMonth : Posix -> Posix
firstDayOfPrevMonth time =
    Time.floor Time.Month utc time
        |> Time.add Time.Day -1 utc
        |> Time.floor Time.Month utc


splitWeek : List Posix -> List (List Posix) -> List (List Posix)
splitWeek days weeks =
    if List.length days < 7 then
        weeks

    else
        List.append weeks [ List.take 7 days ]
            |> splitWeek (List.drop 7 days)


floorDate : Posix -> Posix
floorDate time =
    Time.floor Time.Day utc time


floorMonth : Posix -> Posix
floorMonth time =
    Time.floor Time.Month utc time


floorMinute : Posix -> Posix
floorMinute time =
    Time.floor Time.Minute utc time


trimTime : Posix -> Posix
trimTime time =
    Time.floor Time.Day utc time
        |> Time.posixToMillis
        |> (\d ->
                Time.posixToMillis time - d
           )
        |> Time.millisToPosix


updateHour : Int -> Posix -> Posix
updateHour n time =
    let
        diff =
            n - Time.toHour utc time
    in
    Time.add Hour diff utc time


updateMinute : Int -> Posix -> Posix
updateMinute n time =
    let
        diff =
            n - Time.toMinute utc time
    in
    Time.add Minute diff utc time


addHour : Int -> Posix -> Posix
addHour n time =
    Time.add Hour n utc time


addMinute : Int -> Posix -> Posix
addMinute n time =
    Time.add Minute n utc time


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
