module Iso8601 exposing (fromTime, toTime, decoder, encode)

{-| Convert between ISO-8601 date strings and POSIX times.

@docs fromTime, toTime, decoder, encode

-}

import Json.Decode as Decode exposing (Decoder)
import Json.Encode as Encode
import Parser exposing ((|.), (|=), Parser, andThen, end, int, map, oneOf, succeed, symbol)
import Time exposing (Month(..), utc)


{-| Decode an ISO-8601 date string to a `Time.Posix` value using [`toTime`](#toTime).
-}
decoder : Decoder Time.Posix
decoder =
    Decode.string
        |> Decode.andThen
            (\str ->
                case toTime str of
                    Err deadEnds ->
                        Decode.fail <| Parser.deadEndsToString deadEnds

                    Ok time ->
                        Decode.succeed time
            )


{-| Encode a `Time.Posix` value as an ISO-8601 date string using
[`fromTime`](#fromTime).
-}
encode : Time.Posix -> Encode.Value
encode =
    fromTime >> Encode.string


{-| Convert from an ISO-8601 date string to a `Time.Posix` value.

ISO-8601 date strings sometimes specify things in UTC. Other times, they specify
a non-UTC time as well as a UTC offset. Regardless of which format the ISO-8601
string uses, this function normalizes it and returns a time in UTC.

-}
toTime : String -> Result (List Parser.DeadEnd) Time.Posix
toTime str =
    Parser.run iso8601 str


{-| A fixed-length integer padded with zeroes.
-}
paddedInt : Int -> Parser Int
paddedInt quantity =
    Parser.chompWhile Char.isDigit
        |> Parser.getChompedString
        |> Parser.andThen
            (\str ->
                if String.length str == quantity then
                    -- StringtoInt works on zero-padded integers
                    case String.toInt str of
                        Just intVal ->
                            Parser.succeed intVal

                        Nothing ->
                            Parser.problem ("Invalid integer: \"" ++ str ++ "\"")

                else
                    Parser.problem
                        ("Expected "
                            ++ String.fromInt quantity
                            ++ " digits, but got "
                            ++ String.fromInt (String.length str)
                        )
            )


msPerYear : Int
msPerYear =
    --365 * 24 * 60 * 60 * 1000
    31536000000


msPerDay : Int
msPerDay =
    -- 24 * 60 * 60 * 1000
    86400000


{-| A parsed day was outside the valid month range. (e.g. 0 is never a valid
day in a month, and neither is 32.
-}
invalidDay : Int -> Parser Int
invalidDay day =
    Parser.problem ("Invalid day: " ++ String.fromInt day)


epochYear : Int
epochYear =
    1970


yearMonthDay : ( Int, Int, Int ) -> Parser Int
yearMonthDay ( year, month, dayInMonth ) =
    if dayInMonth < 0 then
        invalidDay dayInMonth

    else
        let
            succeedWith extraMs =
                let
                    days =
                        if month < 3 || not (isLeapYear year) then
                            -- If we're in January or February, it doesn't matter
                            -- if we're in a leap year from a days-in-month perspective.
                            -- Only possible impact of laep years in this scenario is
                            -- if we received February 29, which is checked later.
                            -- Also, this doesn't matter if we explicitly aren't
                            -- in a leap year.
                            dayInMonth - 1

                        else
                            -- We're in a leap year in March-December, so add an extra
                            -- day (for Feb 29) compared to what we'd usually do.
                            dayInMonth

                    dayMs =
                        -- one extra day for each leap year
                        msPerDay * (days + (leapYearsBefore year - leapYearsBefore epochYear))

                    yearMs =
                        msPerYear * (year - epochYear)
                in
                Parser.succeed (extraMs + yearMs + dayMs)
        in
        case month of
            1 ->
                -- 31 days in January
                if dayInMonth > 31 then
                    invalidDay dayInMonth

                else
                    -- Add 0 days when in the first month of the year
                    succeedWith 0

            2 ->
                -- 28 days in February unless it's a leap year; then 29)
                if (dayInMonth > 29) || (dayInMonth == 29 && not (isLeapYear year)) then
                    invalidDay dayInMonth

                else
                    -- 31 days in January
                    -- (31 * 24 * 60 * 60 * 1000)
                    succeedWith 2678400000

            3 ->
                -- 31 days in March
                if dayInMonth > 31 then
                    invalidDay dayInMonth

                else
                    -- 28 days in February (leap years are handled elsewhere)
                    -- ((28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 5097600000

            4 ->
                -- 30 days in April
                if dayInMonth > 30 then
                    invalidDay dayInMonth

                else
                    -- 31 days in March
                    -- ((31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 7776000000

            5 ->
                -- 31 days in May
                if dayInMonth > 31 then
                    invalidDay dayInMonth

                else
                    -- 30 days in April
                    -- ((30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 10368000000

            6 ->
                -- 30 days in June
                if dayInMonth > 30 then
                    invalidDay dayInMonth

                else
                    -- 31 days in May
                    -- ((31 + 30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 13046400000

            7 ->
                -- 31 days in July
                if dayInMonth > 31 then
                    invalidDay dayInMonth

                else
                    -- 30 days in June
                    -- ((30 + 31 + 30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 15638400000

            8 ->
                -- 31 days in August
                if dayInMonth > 31 then
                    invalidDay dayInMonth

                else
                    -- 31 days in July
                    -- ((31 + 30 + 31 + 30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 18316800000

            9 ->
                -- 30 days in September
                if dayInMonth > 30 then
                    invalidDay dayInMonth

                else
                    -- 31 days in August
                    -- ((31 + 31 + 30 + 31 + 30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 20995200000

            10 ->
                -- 31 days in October
                if dayInMonth > 31 then
                    invalidDay dayInMonth

                else
                    -- 30 days in September
                    -- ((30 + 31 + 31 + 30 + 31 + 30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 23587200000

            11 ->
                -- 30 days in November
                if dayInMonth > 30 then
                    invalidDay dayInMonth

                else
                    -- 31 days in October
                    -- ((31 + 30 + 31 + 31 + 30 + 31 + 30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 26265600000

            12 ->
                -- 31 days in December
                if dayInMonth > 31 then
                    invalidDay dayInMonth

                else
                    -- 30 days in November
                    -- ((30 + 31 + 30 + 31 + 31 + 30 + 31 + 30 + 31 + 28 + 31) * 24 * 60 * 60 * 1000)
                    succeedWith 28857600000

            _ ->
                Parser.problem ("Invalid month: \"" ++ String.fromInt month ++ "\"")


fromParts : Int -> Int -> Int -> Int -> Int -> Int -> Time.Posix
fromParts monthYearDayMs hour minute second ms utcOffsetMinutes =
    Time.millisToPosix
        (monthYearDayMs
            + (hour * 60 * 60 * 1000)
            -- Incoroprate and discard UTC offset
            + ((minute - utcOffsetMinutes) * 60 * 1000)
            + (second * 1000)
            + ms
        )


{-| From <https://www.timeanddate.com/date/leapyear.html>

In the Gregorian calendar three criteria must be taken into account to identify leap years:

  - The year can be evenly divided by 4;
  - If the year can be evenly divided by 100, it is NOT a leap year, unless;
  - The year is also evenly divisible by 400. Then it is a leap year.

This means that in the Gregorian calendar, the years 2000 and 2400 are leap years, while 1800, 1900, 2100, 2200, 2300 and 2500 are NOT leap years.

-}
isLeapYear : Int -> Bool
isLeapYear year =
    (modBy 4 year == 0) && ((modBy 100 year /= 0) || (modBy 400 year == 0))


leapYearsBefore : Int -> Int
leapYearsBefore y1 =
    let
        y =
            y1 - 1
    in
    (y // 4) - (y // 100) + (y // 400)


{-| YYYY-MM-DDTHH:mm:ss.sssZ or ±YYYYYY-MM-DDTHH:mm:ss.sssZ
-}
iso8601 : Parser Time.Posix
iso8601 =
    -- TODO account for format variations, including those with UTC offsets
    monthYearDayInMs
        -- YYYY-MM-DD
        |> andThen
            (\datePart ->
                oneOf
                    [ succeed (fromParts datePart)
                        |. symbol "T"
                        |= paddedInt 2
                        -- HH
                        |. symbol ":"
                        |= paddedInt 2
                        -- mm
                        |. symbol ":"
                        |= paddedInt 2
                        -- ss
                        |= oneOf
                            [ succeed identity
                                |. symbol "."
                                |= fractionsOfASecondInMs
                            , succeed 0
                            ]
                        -- SSS
                        |= oneOf
                            [ -- "Z" means UTC
                              map (\_ -> 0) (symbol "Z")

                            -- +05:00 means UTC+5 whereas -11:30 means UTC-11.5
                            , succeed utcOffsetMinutesFromParts
                                |= oneOf
                                    [ map (\_ -> 1) (symbol "+")
                                    , map (\_ -> -1) (symbol "-")
                                    ]
                                |= paddedInt 2
                                |. symbol ":"
                                |= paddedInt 2
                            ]
                    , succeed (fromParts datePart 0 0 0 0 0)
                        |. end
                    ]
            )


{-| Parse fractions of a second, and convert to milliseconds
-}
fractionsOfASecondInMs : Parser Int
fractionsOfASecondInMs =
    Parser.chompWhile Char.isDigit
        |> Parser.getChompedString
        |> Parser.andThen
            (\str ->
                if String.length str <= 9 then
                    case String.toFloat ("0." ++ str) of
                        Just floatVal ->
                            Parser.succeed (round (floatVal * 1000))

                        Nothing ->
                            Parser.problem ("Invalid float: \"" ++ str ++ "\"")

                else
                    Parser.problem
                        ("Expected at most 9 digits, but got "
                            ++ String.fromInt (String.length str)
                        )
            )


{-| Parse the year, month, and day, and convert to milliseconds since the epoch.

We need all three pieces information at once to do this conversion, because of
leap years. Without knowing Month, Year, and Day, we can't tell whether to
succeed or problem when we encounter February 29.

-}
monthYearDayInMs : Parser Int
monthYearDayInMs =
    Parser.succeed (\year month day -> ( year, month, day ))
        |= paddedInt 4
        -- YYYY
        |. symbol "-"
        |= paddedInt 2
        -- MM
        |. symbol "-"
        |= paddedInt 2
        -- DD
        |> Parser.andThen yearMonthDay


utcOffsetMinutesFromParts : Int -> Int -> Int -> Int
utcOffsetMinutesFromParts multiplier hours minutes =
    -- multiplier is either 1 or -1 (for negative UTC offsets)
    multiplier * ((hours * 60) + minutes)


{-| Inflate a Posix integer into a more memory-intensive ISO-8601 date string.

It's generally best to avoid doing this unless an external API requires it.

(UTC integers are less error-prone, take up less memory, and are more efficient
for time arithmetic.)

Format: YYYY-MM-DDTHH:mm:ss.SSSZ

-}
fromTime : Time.Posix -> String
fromTime time =
    ---- YYYY
    toPaddedString 4 (Time.toYear utc time)
        ++ "-"
        -- MM
        ++ toPaddedString 2 (fromMonth (Time.toMonth utc time))
        ++ "-"
        -- DD
        ++ toPaddedString 2 (Time.toDay utc time)
        ++ "T"
        -- HH
        ++ toPaddedString 2 (Time.toHour utc time)
        ++ ":"
        -- mm
        ++ toPaddedString 2 (Time.toMinute utc time)
        ++ ":"
        -- ss
        ++ toPaddedString 2 (Time.toSecond utc time)
        ++ "."
        -- SSS
        ++ toPaddedString 3 (Time.toMillis utc time)
        ++ "Z"


toPaddedString : Int -> Int -> String
toPaddedString digits time =
    String.padLeft digits '0' (String.fromInt time)


fromMonth : Time.Month -> Int
fromMonth month =
    case month of
        Jan ->
            1

        Feb ->
            2

        Mar ->
            3

        Apr ->
            4

        May ->
            5

        Jun ->
            6

        Jul ->
            7

        Aug ->
            8

        Sep ->
            9

        Oct ->
            10

        Nov ->
            11

        Dec ->
            12
