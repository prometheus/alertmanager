module Utils.DateTimePicker.Views exposing (viewDateTimePicker)

import Html exposing (Html, br, button, div, i, input, p, strong, text)
import Html.Attributes exposing (class, maxlength, value)
import Html.Events exposing (on, onClick, onMouseOut, onMouseOver)
import Iso8601
import Json.Decode as Decode exposing (Decoder)
import Time exposing (Posix, Zone, utc)
import Utils.DateTimePicker.Types exposing (DateTimePicker, InputHourOrMinute(..), Msg(..), PickerConfig, StartOrEnd(..))
import Utils.DateTimePicker.Updates exposing (update)
import Utils.DateTimePicker.Utils
    exposing
        ( floorDate
        , floorMonth
        , listDaysOfMonth
        , monthToString
        , splitWeek
        , targetValueIntParse
        )


viewDateTimePicker : PickerConfig msg -> DateTimePicker -> Html msg
viewDateTimePicker pickerConfig dateTimePicker =
    div [ class "w-100 container" ]
        [ viewCalendar pickerConfig dateTimePicker
        , div [ class "pt-4 row justify-content-center" ]
            [ viewTimePicker pickerConfig dateTimePicker Start
            , viewTimePicker pickerConfig dateTimePicker End
            ]
        ]


viewCalendar : PickerConfig msg -> DateTimePicker -> Html msg
viewCalendar pickerConfig dateTimePicker =
    let
        justViewTime =
            dateTimePicker.month
                |> Maybe.withDefault (Time.millisToPosix 0)
    in
    div [ class "calendar_ month" ]
        [ viewMonthHeader pickerConfig dateTimePicker justViewTime
        , viewMonth pickerConfig dateTimePicker justViewTime
        ]


viewMonthHeader : PickerConfig msg -> DateTimePicker -> Posix -> Html msg
viewMonthHeader pickerConfig dateTimePicker justViewTime =
    div [ class "row month-header" ]
        [ div
            [ class "prev-month d-flex-center"
            , onClick <| pickerConfig.pickerMsg <| update pickerConfig PrevMonth dateTimePicker
            ]
            [ p
                [ class "arrow" ]
                [ i
                    [ class "fa fa-angle-left fa-3x cursor-pointer" ]
                    []
                ]
            ]
        , div
            [ class "month-text d-flex-center" ]
            [ text (Time.toYear pickerConfig.zone justViewTime |> String.fromInt)
            , br [] []
            , text (Time.toMonth pickerConfig.zone justViewTime |> monthToString)
            ]
        , div
            [ class "next-month d-flex-center"
            , onClick <| pickerConfig.pickerMsg <| update pickerConfig NextMonth dateTimePicker
            ]
            [ p
                [ class "arrow" ]
                [ i
                    [ class "fa fa-angle-right fa-3x cursor-pointer" ]
                    []
                ]
            ]
        ]


viewMonth : PickerConfig msg -> DateTimePicker -> Posix -> Html msg
viewMonth pickerConfig dateTimePicker justViewTime =
    let
        days =
            listDaysOfMonth pickerConfig.zone justViewTime

        weeks =
            splitWeek days []
    in
    div [ class "row justify-content-center" ]
        [ div [ class "weekheader" ]
            (List.map viewWeekHeader [ "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat" ])
        , div
            [ class "date-container"
            , onMouseOut <| pickerConfig.pickerMsg (update pickerConfig ClearMouseOverDay dateTimePicker)
            ]
            (List.map (viewWeek pickerConfig dateTimePicker justViewTime) weeks)
        ]


viewWeekHeader : String -> Html msg
viewWeekHeader weekday =
    div [ class "date text-muted" ]
        [ text weekday ]


viewWeek : PickerConfig msg -> DateTimePicker -> Posix -> List Posix -> Html msg
viewWeek pickerConfig dateTimePicker justViewTime days =
    div []
        [ div [] (List.map (viewDay pickerConfig dateTimePicker justViewTime) days) ]


viewDay : PickerConfig msg -> DateTimePicker -> Posix -> Posix -> Html msg
viewDay pickerConfig dateTimePicker justViewTime day =
    let
        compareDate_ : Posix -> Posix -> Order
        compareDate_ a b =
            compare (floorDate pickerConfig.zone a |> Time.posixToMillis)
                (floorDate pickerConfig.zone b |> Time.posixToMillis)

        setClass_ : Maybe Posix -> String -> String
        setClass_ d s =
            case d of
                Just m ->
                    case compareDate_ m day of
                        EQ ->
                            s

                        _ ->
                            ""

                Nothing ->
                    ""

        thisMonthClass =
            if
                floorMonth pickerConfig.zone justViewTime
                    == floorMonth pickerConfig.zone day
            then
                " thismonth"

            else
                ""

        mouseoverClass =
            setClass_ dateTimePicker.mouseOverDay " mouseover"

        startClass =
            setClass_ dateTimePicker.startDate " start"

        endClass =
            setClass_ dateTimePicker.endDate " end"

        ( startClassBack, endClassBack ) =
            Maybe.map2 (\sd ed -> ( startClass, endClass )) dateTimePicker.startDate dateTimePicker.endDate
                |> Maybe.withDefault ( "", "" )

        betweenClass =
            case ( dateTimePicker.startDate, dateTimePicker.endDate ) of
                ( Just start, Just end ) ->
                    case ( compareDate_ start day, compareDate_ end day ) of
                        ( LT, GT ) ->
                            " between"

                        _ ->
                            ""

                _ ->
                    ""
    in
    div [ class ("date back" ++ startClassBack ++ endClassBack ++ betweenClass) ]
        [ div
            [ class ("date front" ++ mouseoverClass ++ startClass ++ endClass ++ thisMonthClass)
            , onMouseOver <| pickerConfig.pickerMsg (update pickerConfig (MouseOverDay day) dateTimePicker)
            , onClick <| pickerConfig.pickerMsg (update pickerConfig OnClickDay dateTimePicker)
            ]
            [ text (Time.toDay pickerConfig.zone day |> String.fromInt) ]
        ]


viewTimePicker : PickerConfig msg -> DateTimePicker -> StartOrEnd -> Html msg
viewTimePicker pickerConfig dateTimePicker startOrEnd =
    div
        [ class "row timepicker" ]
        [ strong [ class "subject" ]
            [ text
                (case startOrEnd of
                    Start ->
                        "Start"

                    End ->
                        "End"
                )
            ]
        , div [ class "hour" ]
            [ button
                [ class "up-button d-flex-center"
                , onClick <| pickerConfig.pickerMsg <| update pickerConfig (IncrementTime startOrEnd InputHour 1) dateTimePicker
                ]
                [ i
                    [ class "fa fa-angle-up" ]
                    []
                ]
            , input
                [ on "blur" (Decode.map pickerConfig.pickerMsg (Decode.map (\msg -> update pickerConfig msg dateTimePicker) (Decode.map (SetInputTime startOrEnd InputHour) targetValueIntParse)))
                , value
                    (case startOrEnd of
                        Start ->
                            case dateTimePicker.startTime of
                                Just t ->
                                    Time.toHour pickerConfig.zone t |> String.fromInt

                                Nothing ->
                                    "0"

                        End ->
                            case dateTimePicker.endTime of
                                Just t ->
                                    Time.toHour pickerConfig.zone t |> String.fromInt

                                Nothing ->
                                    "0"
                    )
                , maxlength 2
                , class "view d-flex-center"
                ]
                []
            , button
                [ class "down-button d-flex-center"
                , onClick <| pickerConfig.pickerMsg <| update pickerConfig (IncrementTime startOrEnd InputHour -1) dateTimePicker
                ]
                [ i
                    [ class "fa fa-angle-down" ]
                    []
                ]
            ]
        , div [ class "colon d-flex-center" ] [ text ":" ]
        , div [ class "minute" ]
            [ button
                [ class "up-button d-flex-center"
                , onClick <| pickerConfig.pickerMsg <| update pickerConfig (IncrementTime startOrEnd InputMinute 1) dateTimePicker
                ]
                [ i
                    [ class "fa fa-angle-up" ]
                    []
                ]
            , input
                [ on "blur" (Decode.map pickerConfig.pickerMsg (Decode.map (\msg -> update pickerConfig msg dateTimePicker) (Decode.map (SetInputTime startOrEnd InputMinute) targetValueIntParse)))
                , value
                    (case startOrEnd of
                        Start ->
                            case dateTimePicker.startTime of
                                Just t ->
                                    Time.toMinute pickerConfig.zone t |> String.fromInt

                                Nothing ->
                                    "0"

                        End ->
                            case dateTimePicker.endTime of
                                Just t ->
                                    Time.toMinute pickerConfig.zone t |> String.fromInt

                                Nothing ->
                                    "0"
                    )
                , maxlength 2
                , class "view"
                ]
                []
            , button
                [ class "down-button d-flex-center"
                , onClick <| pickerConfig.pickerMsg <| update pickerConfig (IncrementTime startOrEnd InputMinute -1) dateTimePicker
                ]
                [ i
                    [ class "fa fa-angle-down" ]
                    []
                ]
            ]
        , div [ class "timeview d-flex-center" ]
            [ text
                (let
                    toString_ : Maybe Posix -> Maybe Posix -> String
                    toString_ maybeTime maybeDate =
                        Maybe.map
                            (\t ->
                                case maybeDate of
                                    Just d ->
                                        Iso8601.fromTime t
                                            |> String.dropRight 8

                                    Nothing ->
                                        ""
                            )
                            maybeTime
                            |> Maybe.withDefault ""

                    selectedTime =
                        case startOrEnd of
                            Start ->
                                toString_ dateTimePicker.startTime dateTimePicker.startDate

                            End ->
                                toString_ dateTimePicker.endTime dateTimePicker.endDate
                 in
                 selectedTime
                )
            ]
        ]
