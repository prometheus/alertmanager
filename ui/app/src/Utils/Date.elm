module Utils.Date exposing
    ( addDuration
    , dateTimeFormat
    , durationFormat
    , parseDuration
    , timeDifference
    , timeFromString
    , timeToString
    )

import Iso8601
import Parser exposing ((|.), (|=), Parser)
import Time exposing (Posix)
import Tuple


parseDuration : String -> Result String Float
parseDuration =
    Parser.run durationParser >> Result.mapError (always "Wrong duration format")


durationParser : Parser Float
durationParser =
    Parser.succeed identity
        |= Parser.loop 0 durationHelp
        |. Parser.spaces
        |. Parser.end


durationHelp : Float -> Parser (Parser.Step Float Float)
durationHelp duration =
    Parser.oneOf
        [ Parser.succeed (\d -> Parser.Loop (d + duration))
            |= term
            |. Parser.spaces
        , Parser.succeed (Parser.Done duration)
        ]


units : List ( String, number )
units =
    [ ( "w", 604800000 )
    , ( "d", 86400000 )
    , ( "h", 3600000 )
    , ( "m", 60000 )
    , ( "s", 1000 )
    ]


timeToString : Posix -> String
timeToString =
    Iso8601.fromTime


term : Parser Float
term =
    Parser.succeed (*)
        |= Parser.float
        |= (units
                |> List.map (\( unit, ms ) -> Parser.succeed ms |. Parser.symbol unit)
                |> Parser.oneOf
           )


addDuration : Float -> Posix -> Posix
addDuration duration time =
    Time.millisToPosix <|
        (Time.posixToMillis time + round duration)


timeDifference : Posix -> Posix -> Float
timeDifference startsAt endsAt =
    toFloat <|
        (Time.posixToMillis endsAt - Time.posixToMillis startsAt)


durationFormat : Float -> Maybe String
durationFormat duration =
    if duration >= 0 then
        List.foldl
            (\( unit, ms ) ( result, curr ) ->
                ( if curr // ms == 0 then
                    result

                  else
                    result ++ String.fromInt (curr // ms) ++ unit ++ " "
                , modBy ms curr
                )
            )
            ( "", round duration )
            units
            |> Tuple.first
            |> String.trim
            |> Just

    else
        Nothing


dateTimeFormat : Posix -> String
dateTimeFormat =
    Iso8601.fromTime


timeFromString : String -> Result String Posix
timeFromString string =
    if string == "" then
        Err "Should not be empty"

    else
        Iso8601.toTime string
            |> Result.mapError (always "Wrong ISO8601 format")
