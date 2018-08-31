module Utils.Date exposing (dateFormat, dateTimeFormat, durationFormat, durationParser, encode, fromTime, parseDuration, term, timeFormat, timeFromString, timeToString, units)

import Date
import Date.Extra.Config.Config_en_us exposing (config)
import Date.Extra.Format
import ISO8601
import Parser exposing ((|.), (|=), Parser)
import Time
import Tuple
import Utils.Types as Types


parseDuration : String -> Result String Time.Time
parseDuration =
    Parser.run durationParser >> Result.mapError (always "Wrong duration format")


durationParser : Parser Time.Time
durationParser =
    Parser.succeed (List.foldr (+) 0)
        |= Parser.repeat Parser.zeroOrMore term
        |. Parser.end


units : List ( String, number )
units =
    [ ( "y", 31556926000 )
    , ( "d", 86400000 )
    , ( "h", 3600000 )
    , ( "m", 60000 )
    , ( "s", 1000 )
    ]


timeToString : Time.Time -> String
timeToString =
    round >> ISO8601.fromTime >> ISO8601.toString


term : Parser Time.Time
term =
    Parser.map2 (*)
        Parser.float
        (units
            |> List.map (\( unit, ms ) -> Parser.succeed ms |. Parser.symbol unit)
            |> Parser.oneOf
        )
        |. Parser.ignore Parser.zeroOrMore ((==) ' ')


durationFormat : Time.Time -> Maybe String
durationFormat time =
    if time >= 0 then
        List.foldl
            (\( unit, ms ) ( result, curr ) ->
                ( if curr // ms == 0 then
                    result

                  else
                    result ++ toString (curr // ms) ++ unit ++ " "
                , modBy ms curr
                )
            )
            ( "", round time )
            units
            |> Tuple.first
            |> String.trim
            |> Just

    else
        Nothing


dateFormat : Time.Time -> String
dateFormat =
    Date.fromTime >> Date.Extra.Format.formatUtc config Date.Extra.Format.isoDateFormat


timeFormat : Time.Time -> String
timeFormat =
    Date.fromTime >> Date.Extra.Format.formatUtc config Date.Extra.Format.isoTimeFormat


dateTimeFormat : Time.Time -> String
dateTimeFormat t =
    dateFormat t ++ " " ++ timeFormat t


encode : Time.Time -> String
encode =
    round >> ISO8601.fromTime >> ISO8601.toString


timeFromString : String -> Result String Time.Time
timeFromString string =
    if string == "" then
        Err "Should not be empty"

    else
        ISO8601.fromString string
            |> Result.map (ISO8601.toTime >> toFloat)
            |> Result.mapError (always "Wrong ISO8601 format")


fromTime : Time.Time -> Types.Time
fromTime time =
    { s = round time |> ISO8601.fromTime |> ISO8601.toString
    , t = Just time
    }
