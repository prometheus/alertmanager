module Utils.DateTimePicker.Utils exposing
    ( addDateAndTime
    , addMaybePosix
    , calculatePickerOffset
    , dayToNameString
    , determineDateTimeRange
    , doDaysMatch
    , durationDayClasses
    , durationDayPickedOrBetween
    , floorMinute
    , monthData
    , monthToNameString
    , pickUpTimeFromDateTimePosix
    , splitIntoWeeks
    , targetValueIntParse
    , validRuntimeOrNothing
    )

import Html exposing (Html, div, text)
import Html.Attributes exposing (class)
import Html.Events exposing (targetValue)
import Json.Decode as Decode
import List
import Time exposing (Month(..), Posix, Weekday(..), Zone)
import Time.Extra as Time exposing (Interval(..))


doDaysMatch : Zone -> Posix -> Posix -> Bool
doDaysMatch zone dateTimeOne dateTimeTwo =
    let
        oneParts =
            Time.posixToParts zone dateTimeOne

        twoParts =
            Time.posixToParts zone dateTimeTwo
    in
    oneParts.day == twoParts.day && oneParts.month == twoParts.month && oneParts.year == twoParts.year


monthData : Zone -> Posix -> List Posix
monthData zone time =
    let
        monthStart =
            Time.floor Time.Month zone time

        monthStartDay =
            Time.toWeekday zone monthStart

        nextMonthStart =
            Time.ceiling Time.Month zone (Time.add Time.Day 1 zone monthStart)

        nextMonthStartDay =
            Time.toWeekday zone nextMonthStart

        frontPad =
            case monthStartDay of
                Mon ->
                    Time.range Time.Day 1 zone (Time.add Time.Day -1 zone monthStart) monthStart

                Tue ->
                    Time.range Time.Day 1 zone (Time.add Time.Day -2 zone monthStart) monthStart

                Wed ->
                    Time.range Time.Day 1 zone (Time.add Time.Day -3 zone monthStart) monthStart

                Thu ->
                    Time.range Time.Day 1 zone (Time.add Time.Day -4 zone monthStart) monthStart

                Fri ->
                    Time.range Time.Day 1 zone (Time.add Time.Day -5 zone monthStart) monthStart

                Sat ->
                    Time.range Time.Day 1 zone (Time.add Time.Day -6 zone monthStart) monthStart

                Sun ->
                    []

        endPad =
            case nextMonthStartDay of
                Mon ->
                    Time.range Time.Day 1 zone nextMonthStart (Time.add Time.Day 6 zone nextMonthStart)

                Tue ->
                    Time.range Time.Day 1 zone nextMonthStart (Time.add Time.Day 5 zone nextMonthStart)

                Wed ->
                    Time.range Time.Day 1 zone nextMonthStart (Time.add Time.Day 4 zone nextMonthStart)

                Thu ->
                    Time.range Time.Day 1 zone nextMonthStart (Time.add Time.Day 3 zone nextMonthStart)

                Fri ->
                    Time.range Time.Day 1 zone nextMonthStart (Time.add Time.Day 2 zone nextMonthStart)

                Sat ->
                    Time.range Time.Day 1 zone nextMonthStart (Time.add Time.Day 1 zone nextMonthStart)

                Sun ->
                    []
    in
    frontPad ++ Time.range Time.Day 1 zone monthStart nextMonthStart ++ endPad


splitIntoWeeks : List Posix -> List (List Posix) -> List (List Posix)
splitIntoWeeks days weeks =
    if List.length days <= 7 then
        days :: weeks

    else
        let
            ( week, restOfDays ) =
                splitAt 7 days

            newWeeks =
                week :: weeks
        in
        splitIntoWeeks restOfDays newWeeks


monthToNameString : Month -> String
monthToNameString month =
    case month of
        Jan ->
            "Jan"

        Feb ->
            "Feb"

        Mar ->
            "Mar"

        Apr ->
            "Apr"

        May ->
            "May"

        Jun ->
            "Jun"

        Jul ->
            "Jul"

        Aug ->
            "Aug"

        Sep ->
            "Sep"

        Oct ->
            "Oct"

        Nov ->
            "Nov"

        Dec ->
            "december"


dayToNameString : Weekday -> String
dayToNameString day =
    case day of
        Mon ->
            "Mo"

        Tue ->
            "Tu"

        Wed ->
            "We"

        Thu ->
            "Th"

        Fri ->
            "Fr"

        Sat ->
            "Sa"

        Sun ->
            "Su"


splitAt : Int -> List a -> ( List a, List a )
splitAt n xs =
    ( List.take n xs, List.drop n xs )


validRuntimeOrNothing : Maybe Posix -> Maybe Posix -> Maybe ( Posix, Posix )
validRuntimeOrNothing start end =
    Maybe.map2
        (\s e ->
            Just ( s, e )
        )
        start
        end
        |> Maybe.withDefault Nothing


durationDayClasses : String -> Bool -> Bool -> String
durationDayClasses classPrefix isPicked isBetween =
    if isPicked then
        classPrefix ++ "calendar-day " ++ classPrefix ++ "picked"

    else if isBetween then
        classPrefix ++ "calendar-day " ++ classPrefix ++ "between"

    else
        classPrefix ++ "calendar-day"


durationDayPickedOrBetween : Zone -> Posix -> Maybe Posix -> ( Maybe Posix, Maybe Posix ) -> ( Bool, Bool )
durationDayPickedOrBetween zone day hovered ( pickedStart, pickedEnd ) =
    case ( pickedStart, pickedEnd ) of
        ( Nothing, Nothing ) ->
            ( False, False )

        ( Just start, Nothing ) ->
            let
                picked =
                    doDaysMatch zone day start

                between =
                    case hovered of
                        Just hoveredTime ->
                            isDayBetweenDates day start hoveredTime

                        Nothing ->
                            False
            in
            ( picked, between )

        ( Nothing, Just end ) ->
            let
                picked =
                    doDaysMatch zone day end

                between =
                    case hovered of
                        Just hoveredTime ->
                            isDayBetweenDates day end hoveredTime

                        Nothing ->
                            False
            in
            ( picked, between )

        ( Just start, Just end ) ->
            let
                picked =
                    doDaysMatch zone day end || doDaysMatch zone day start

                between =
                    isDayBetweenDates day start end
            in
            ( picked, between )


isDayBetweenDates : Posix -> Posix -> Posix -> Bool
isDayBetweenDates day dateOne dateTwo =
    (Time.posixToMillis dateOne
        > Time.posixToMillis day
        && Time.posixToMillis day
        > Time.posixToMillis dateTwo
    )
        || (Time.posixToMillis dateOne
                < Time.posixToMillis day
                && Time.posixToMillis day
                < Time.posixToMillis dateTwo
           )


determineDateTimeRange : Zone -> Maybe Posix -> Maybe Posix -> Maybe Posix -> ( Maybe Posix, Maybe Posix )
determineDateTimeRange zone pickedStart pickedEnd hoveredDate =
    case hoveredDate of
        Just hovered ->
            case ( pickedStart, pickedEnd ) of
                ( Just start, Just end ) ->
                    ( Just start, Just end )

                ( Just start, Nothing ) ->
                    if firstLessThanOrEqualsSecond start hovered then
                        ( Just start, hoveredDate )

                    else
                        ( hoveredDate, Just start )

                ( Nothing, Just end ) ->
                    if firstLessThanOrEqualsSecond hovered end then
                        ( hoveredDate, Just end )

                    else
                        ( Just end, hoveredDate )

                ( Nothing, Nothing ) ->
                    ( hoveredDate, Nothing )

        Nothing ->
            ( pickedStart, pickedEnd )


firstLessThanOrEqualsSecond : Posix -> Posix -> Bool
firstLessThanOrEqualsSecond first second =
    Time.posixToMillis first <= Time.posixToMillis second


pickUpTimeFromDateTimePosix : Zone -> Maybe Posix -> Maybe Posix
pickUpTimeFromDateTimePosix zone maybeDate =
    case maybeDate of
        Just date ->
            (date |> Time.posixToMillis)
                - (Time.floor Day zone date |> Time.posixToMillis)
                |> Time.millisToPosix
                |> Time.floor Minute zone
                |> Just

        Nothing ->
            Nothing


addDateAndTime : Zone -> Maybe Posix -> Maybe Posix -> Maybe Posix
addDateAndTime zone date time =
    case ( date, time ) of
        ( Just d, Just t ) ->
            let
                tfloor =
                    Time.floor Day zone t |> Time.posixToMillis

                duration =
                    Time.posixToMillis t - tfloor

                dfloor =
                    Time.floor Day zone d |> Time.posixToMillis
            in
            Just (dfloor + duration |> Time.millisToPosix)

        _ ->
            Nothing


addMaybePosix : Maybe Posix -> Posix -> Maybe Posix
addMaybePosix maybeDate duration =
    case maybeDate of
        Just time ->
            Time.posixToMillis time
                |> (\l -> l + Time.posixToMillis duration)
                |> Time.millisToPosix
                |> Just

        Nothing ->
            Nothing


floorMinute : Zone -> Maybe Posix -> Maybe Posix
floorMinute zone maybeTime =
    case maybeTime of
        Just time ->
            Just (Time.floor Minute zone time)

        Nothing ->
            Nothing


calculatePickerOffset : Zone -> Posix -> Maybe Posix -> Int
calculatePickerOffset zone baseTime pickedTime =
    let
        flooredBase =
            Time.floor Month zone baseTime
    in
    case pickedTime of
        Nothing ->
            0

        Just time ->
            let
                flooredPick =
                    Time.floor Month zone time
            in
            if Time.posixToMillis flooredBase <= Time.posixToMillis flooredPick then
                Time.diff Month zone flooredBase flooredPick

            else
                0 - Time.diff Month zone flooredPick flooredBase


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
