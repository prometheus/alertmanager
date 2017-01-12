module Utils.Date exposing (..)

import Date exposing (Date, Month(..))
import Time
import ISO8601


dateFormat : ISO8601.Time -> String
dateFormat t =
    String.join "/" <| List.map toString [ ISO8601.month t, ISO8601.day t, ISO8601.year t ]


unixEpochStart : ISO8601.Time
unixEpochStart =
    ISO8601.fromTime 0


addTime : Time.Time -> Time.Time -> ISO8601.Time
addTime isoTime add =
    let
        ms =
            (Time.inMilliseconds isoTime) + (Time.inMilliseconds add)

        t =
            round ms
    in
        ISO8601.fromTime t


parseWithDefault : ISO8601.Time -> String -> ISO8601.Time
parseWithDefault default toParse =
    Result.withDefault default (ISO8601.fromString toParse)


toISO8601 : Time.Time -> ISO8601.Time
toISO8601 time =
    ISO8601.fromTime <| round (Time.inMilliseconds time)
