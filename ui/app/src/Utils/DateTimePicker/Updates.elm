module Utils.DateTimePicker.Updates exposing (update)

import Time exposing (Posix)
import Utils.DateTimePicker.Types
    exposing
        ( DateTimePicker
        , InputHourOrMinute(..)
        , Msg(..)
        , PickerConfig
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


update : PickerConfig msg -> Msg -> DateTimePicker -> ( DateTimePicker, Maybe ( Posix, Posix ) )
update pickerConfig msg dateTimePicker =
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
            ( { dateTimePicker | month = Just (firstDayOfNextMonth pickerConfig.zone justMonth) }, Nothing )

        PrevMonth ->
            ( { dateTimePicker | month = Just (firstDayOfPrevMonth pickerConfig.zone justMonth) }, Nothing )

        MouseOverDay time ->
            ( { dateTimePicker | mouseOverDay = Just time }, Nothing )

        ClearMouseOverDay ->
            ( { dateTimePicker | mouseOverDay = Nothing }, Nothing )

        OnClickDay ->
            let
                addDateTime_ : Posix -> Maybe Posix -> Posix
                addDateTime_ date maybeTime =
                    case maybeTime of
                        Just time ->
                            floorDate pickerConfig.zone date
                                |> Time.posixToMillis
                                |> (\d ->
                                        trimTime pickerConfig.zone time
                                            |> Time.posixToMillis
                                            |> (\t -> d + t)
                                   )
                                |> Time.millisToPosix

                        Nothing ->
                            floorDate pickerConfig.zone date

                updateTime_ : Maybe Posix -> Maybe Posix -> Maybe Posix
                updateTime_ maybeDate maybeTime =
                    case maybeDate of
                        Just date ->
                            Just <| addDateTime_ date maybeTime

                        Nothing ->
                            maybeTime

                ( startDate, endDate, selectedDate ) =
                    case dateTimePicker.mouseOverDay of
                        Just m ->
                            case ( dateTimePicker.startDate, dateTimePicker.endDate ) of
                                ( Nothing, Nothing ) ->
                                    ( Just m
                                    , Nothing
                                    , Nothing
                                    )

                                ( Just start, Nothing ) ->
                                    case
                                        compare (floorDate pickerConfig.zone m |> Time.posixToMillis)
                                            (floorDate pickerConfig.zone start |> Time.posixToMillis)
                                    of
                                        LT ->
                                            ( Just m
                                            , Just start
                                            , Just ( addDateTime_ m dateTimePicker.startTime, addDateTime_ start dateTimePicker.endTime )
                                            )

                                        _ ->
                                            ( Just start
                                            , Just m
                                            , Just ( addDateTime_ start dateTimePicker.startTime, addDateTime_ m dateTimePicker.endTime )
                                            )

                                ( Nothing, Just end ) ->
                                    ( Just m
                                    , Just end
                                    , Just ( addDateTime_ m dateTimePicker.startTime, addDateTime_ end dateTimePicker.endTime )
                                    )

                                ( Just start, Just end ) ->
                                    ( Just m
                                    , Nothing
                                    , Nothing
                                    )

                        _ ->
                            ( dateTimePicker.startDate
                            , dateTimePicker.endDate
                            , Nothing
                            )
            in
            ( { dateTimePicker
                | startDate = startDate
                , endDate = endDate
                , startTime = updateTime_ startDate dateTimePicker.startTime
                , endTime = updateTime_ endDate dateTimePicker.endTime
              }
            , selectedDate
            )

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
                            updateHour pickerConfig.zone (limit_ 24 num) s

                        InputMinute ->
                            updateMinute pickerConfig.zone (limit_ 60 num) s

                ( startTime, endTime ) =
                    setTime_ startOrEnd inputHourOrMinute updateHourOrMinute_

                selectedTime =
                    Maybe.map2 (\s e -> ( s, e )) startTime endTime
            in
            ( { dateTimePicker | startTime = startTime, endTime = endTime }, selectedTime )

        IncrementTime startOrEnd inputHourOrMinute num ->
            let
                updateHourOrMinute_ : InputHourOrMinute -> Posix -> Posix
                updateHourOrMinute_ ihom s =
                    let
                        compare_ : Posix -> Posix
                        compare_ a =
                            if
                                (floorDate pickerConfig.zone s |> Time.posixToMillis)
                                    == (floorDate pickerConfig.zone a |> Time.posixToMillis)
                            then
                                a

                            else
                                s
                    in
                    case ihom of
                        InputHour ->
                            addHour pickerConfig.zone num s
                                |> compare_

                        InputMinute ->
                            addMinute pickerConfig.zone num s
                                |> compare_

                ( startTime, endTime ) =
                    setTime_ startOrEnd inputHourOrMinute updateHourOrMinute_

                selectedTime =
                    Maybe.map2 (\s e -> ( s, e )) startTime endTime
            in
            ( { dateTimePicker | startTime = startTime, endTime = endTime }, selectedTime )
