module Utils.Date exposing (..)

import ISO8601
import Parser exposing (Parser, (|.), (|=))
import Time
import Utils.Types as Types
import Tuple


parseDuration : String -> Result Parser.Error Time.Time
parseDuration =
    Parser.run durationParser


durationParser : Parser Time.Time
durationParser =
    Parser.succeed (List.foldr (+) 0)
        |= Parser.zeroOrMore term
        |. Parser.end


units : List ( String, number )
units =
    [ ( "y", 31556926000 )
    , ( "d", 86400000 )
    , ( "h", 3600000 )
    , ( "m", 60000 )
    , ( "s", 1000 )
    ]


term : Parser Time.Time
term =
    Parser.map2 (*)
        Parser.float
        (units
            |> List.map (\( unit, ms ) -> Parser.succeed ms |. Parser.symbol unit)
            |> Parser.oneOf
        )
        |. Parser.ignoreWhile ((==) ' ')


durationFormat : Time.Time -> String
durationFormat time =
    List.foldl
        (\( unit, ms ) ( result, curr ) ->
            ( if curr // ms == 0 then
                result
              else
                result ++ toString (curr // ms) ++ unit ++ " "
            , curr % ms
            )
        )
        ( "", round time )
        units
        |> Tuple.first
        |> String.trim


dateFormat : ISO8601.Time -> String
dateFormat t =
    String.join "/" <| List.map toString [ ISO8601.month t, ISO8601.day t, ISO8601.year t ]


timeFormat : Types.Time -> String
timeFormat { t, s } =
    t
        |> Maybe.map (round >> ISO8601.fromTime >> dateFormat)
        |> Maybe.withDefault s


encode : Types.Time -> String
encode { t, s } =
    t
        |> Maybe.map (round >> ISO8601.fromTime >> ISO8601.toString)
        |> Maybe.withDefault s


timeFromString : String -> Types.Time
timeFromString toParse =
    { s = toParse
    , t =
        toParse
            |> ISO8601.fromString
            |> Result.toMaybe
            |> Maybe.map (ISO8601.toTime >> toFloat)
    }


durationFromString : String -> Types.Duration
durationFromString toParse =
    { s = toParse
    , d =
        toParse
            |> parseDuration
            |> Result.mapError (Debug.log "error")
            |> Result.toMaybe
    }


duration : Time.Time -> Types.Duration
duration time =
    { d = Just time, s = durationFormat time }


fromTime : Time.Time -> Types.Time
fromTime time =
    { s = round time |> ISO8601.fromTime |> ISO8601.toString
    , t = Just time
    }
