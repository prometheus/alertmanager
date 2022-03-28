module Utils.DateTimePicker.Updates exposing (update)

import Time exposing (Posix)
import Utils.DateTimePicker.Types
    exposing
        ( DateTimePicker
        , InputHourOrMinute(..)
        , Msg(..)
        , StartOrEnd(..)
        )
import Utils.DateTimePicker.Utils
    exposing
        ( addHour
        , addMinute
        , firstDayOfNextMonth
        , firstDayOfPrevMonth
        , floorDate
        , trimTime
        , updateHour
        , updateMinute
        )


update : Msg -> DateTimePicker -> DateTimePicker
update msg dateTimePicker =
    let
        justMonth =
            dateTimePicker.month
                |> Maybe.withDefault (Time.millisToPosix 0)

        setTime_ : StartOrEnd -> InputHourOrMinute -> (InputHourOrMinute -> Posix -> Posix) -> ( Maybe Posix, Maybe Posix )
        setTime_ soe ihom updateTime =
            let
                set_ : Maybe Posix -> Maybe Posix
                set_ a =
                    Maybe.map (\b -> updateTime ihom b) a
            in
            case soe of
                Start ->
                    ( set_ dateTimePicker.startTime, dateTimePicker.endTime )

                End ->
                    ( dateTimePicker.startTime, set_ dateTimePicker.endTime )
    in
    case msg of
        NextMonth ->
            { dateTimePicker | month = Just (firstDayOfNextMonth justMonth) }

        PrevMonth ->
            { dateTimePicker | month = Just (firstDayOfPrevMonth justMonth) }

        MouseOverDay time ->
            { dateTimePicker | mouseOverDay = Just time }

        ClearMouseOverDay ->
            { dateTimePicker | mouseOverDay = Nothing }

        OnClickDay ->
            let
                addDateTime_ : Posix -> Maybe Posix -> Posix
                addDateTime_ date maybeTime =
                    case maybeTime of
                        Just time ->
                            floorDate date
                                |> Time.posixToMillis
                                |> (\d ->
                                        trimTime time
                                            |> Time.posixToMillis
                                            |> (\t -> d + t)
                                   )
                                |> Time.millisToPosix

                        Nothing ->
                            floorDate date

                updateTime_ : Maybe Posix -> Maybe Posix -> Maybe Posix
                updateTime_ maybeDate maybeTime =
                    case maybeDate of
                        Just date ->
                            Just <| addDateTime_ date maybeTime

                        Nothing ->
                            maybeTime

                ( startDate, endDate ) =
                    case dateTimePicker.mouseOverDay of
                        Just m ->
                            case ( dateTimePicker.startDate, dateTimePicker.endDate ) of
                                ( Nothing, Nothing ) ->
                                    ( Just m
                                    , Nothing
                                    )

                                ( Just start, Nothing ) ->
                                    case
                                        compare (floorDate m |> Time.posixToMillis)
                                            (floorDate start |> Time.posixToMillis)
                                    of
                                        LT ->
                                            ( Just m
                                            , Just start
                                            )

                                        _ ->
                                            ( Just start
                                            , Just m
                                            )

                                ( Nothing, Just end ) ->
                                    ( Just m
                                    , Just end
                                    )

                                ( Just _, Just _ ) ->
                                    ( Just m
                                    , Nothing
                                    )

                        _ ->
                            ( dateTimePicker.startDate
                            , dateTimePicker.endDate
                            )
            in
            { dateTimePicker
                | startDate = startDate
                , endDate = endDate
                , startTime = updateTime_ startDate dateTimePicker.startTime
                , endTime = updateTime_ endDate dateTimePicker.endTime
            }

        SetInputTime startOrEnd inputHourOrMinute num ->
            let
                limit_ : Int -> Int -> Int
                limit_ limit n =
                    if n < 0 then
                        0

                    else
                        modBy limit n

                updateHourOrMinute_ : InputHourOrMinute -> Posix -> Posix
                updateHourOrMinute_ ihom s =
                    case ihom of
                        InputHour ->
                            updateHour (limit_ 24 num) s

                        InputMinute ->
                            updateMinute (limit_ 60 num) s

                ( startTime, endTime ) =
                    setTime_ startOrEnd inputHourOrMinute updateHourOrMinute_
            in
            { dateTimePicker | startTime = startTime, endTime = endTime }

        IncrementTime startOrEnd inputHourOrMinute num ->
            let
                updateHourOrMinute_ : InputHourOrMinute -> Posix -> Posix
                updateHourOrMinute_ ihom s =
                    let
                        compare_ : Posix -> Posix
                        compare_ a =
                            if
                                (floorDate s |> Time.posixToMillis)
                                    == (floorDate a |> Time.posixToMillis)
                            then
                                a

                            else
                                s
                    in
                    case ihom of
                        InputHour ->
                            addHour num s
                                |> compare_

                        InputMinute ->
                            addMinute num s
                                |> compare_

                ( startTime, endTime ) =
                    setTime_ startOrEnd inputHourOrMinute updateHourOrMinute_
            in
            { dateTimePicker | startTime = startTime, endTime = endTime }
