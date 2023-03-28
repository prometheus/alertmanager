module Utils.DateTimePicker.Views exposing (viewDateTimePicker)

import Html exposing (Html, br, button, div, i, input, p, strong, text)
import Html.Attributes exposing (class, maxlength, value)
import Html.Events exposing (on, onClick, onMouseOut, onMouseOver)
import Iso8601
import Json.Decode as Decode
import Time exposing (Posix, utc)
import Utils.DateTimePicker.Types exposing (DateTimePicker, InputHourOrMinute(..), Msg(..), StartOrEnd(..))
import Utils.DateTimePicker.Utils
    exposing
        ( FirstDayOfWeek(..)
        , floorDate
        , floorMonth
        , listDaysOfMonth
        , monthToString
        , splitWeek
        , targetValueIntParse
        )


viewDateTimePicker : DateTimePicker -> Html Msg
viewDateTimePicker dateTimePicker =
    div [ class "w-100 container" ]
        [ viewCalendar dateTimePicker
        , div [ class "pt-4 row justify-content-center" ]
            [ viewTimePicker dateTimePicker Start
            , viewTimePicker dateTimePicker End
            ]
        ]


viewCalendar : DateTimePicker -> Html Msg
viewCalendar dateTimePicker =
    let
        justViewTime =
            dateTimePicker.month
                |> Maybe.withDefault (Time.millisToPosix 0)
    in
    div [ class "calendar_ month" ]
        [ viewMonthHeader justViewTime
        , viewMonth dateTimePicker justViewTime
        ]


viewMonthHeader : Posix -> Html Msg
viewMonthHeader justViewTime =
    div [ class "row month-header" ]
        [ div
            [ class "prev-month d-flex-center"
            , onClick PrevMonth
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
            [ text (Time.toYear utc justViewTime |> String.fromInt)
            , br [] []
            , text (Time.toMonth utc justViewTime |> monthToString)
            ]
        , div
            [ class "next-month d-flex-center"
            , onClick NextMonth
            ]
            [ p
                [ class "arrow" ]
                [ i
                    [ class "fa fa-angle-right fa-3x cursor-pointer" ]
                    []
                ]
            ]
        ]


viewMonth : DateTimePicker -> Posix -> Html Msg
viewMonth dateTimePicker justViewTime =
    let
        days =
            listDaysOfMonth justViewTime dateTimePicker.firstDayOfWeek

        weeks =
            splitWeek days []
    in
    div [ class "row justify-content-center" ]
        [ div [ class "weekheader" ]
            (case dateTimePicker.firstDayOfWeek of
                Sunday ->
                    List.map viewWeekHeader [ "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat" ]

                Monday ->
                    List.map viewWeekHeader [ "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun" ]
            )
        , div
            [ class "date-container"
            , onMouseOut ClearMouseOverDay
            ]
            (List.map (viewWeek dateTimePicker justViewTime) weeks)
        ]


viewWeekHeader : String -> Html Msg
viewWeekHeader weekday =
    div [ class "date text-muted" ]
        [ text weekday ]


viewWeek : DateTimePicker -> Posix -> List Posix -> Html Msg
viewWeek dateTimePicker justViewTime days =
    div []
        [ div [] (List.map (viewDay dateTimePicker justViewTime) days) ]


viewDay : DateTimePicker -> Posix -> Posix -> Html Msg
viewDay dateTimePicker justViewTime day =
    let
        compareDate_ : Posix -> Posix -> Order
        compareDate_ a b =
            compare (floorDate a |> Time.posixToMillis)
                (floorDate b |> Time.posixToMillis)

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
            if floorMonth justViewTime == floorMonth day then
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
            Maybe.map2 (\_ _ -> ( startClass, endClass )) dateTimePicker.startDate dateTimePicker.endDate
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
            , onMouseOver <| MouseOverDay day
            , onClick OnClickDay
            ]
            [ text (Time.toDay utc day |> String.fromInt) ]
        ]


viewTimePicker : DateTimePicker -> StartOrEnd -> Html Msg
viewTimePicker dateTimePicker startOrEnd =
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
                , onClick <| IncrementTime startOrEnd InputHour 1
                ]
                [ i
                    [ class "fa fa-angle-up" ]
                    []
                ]
            , input
                [ on "blur" <| Decode.map (SetInputTime startOrEnd InputHour) targetValueIntParse
                , value
                    (case startOrEnd of
                        Start ->
                            case dateTimePicker.startTime of
                                Just t ->
                                    Time.toHour utc t |> String.fromInt

                                Nothing ->
                                    "0"

                        End ->
                            case dateTimePicker.endTime of
                                Just t ->
                                    Time.toHour utc t |> String.fromInt

                                Nothing ->
                                    "0"
                    )
                , maxlength 2
                , class "view d-flex-center"
                ]
                []
            , button
                [ class "down-button d-flex-center"
                , onClick <| IncrementTime startOrEnd InputHour -1
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
                , onClick <| IncrementTime startOrEnd InputMinute 1
                ]
                [ i
                    [ class "fa fa-angle-up" ]
                    []
                ]
            , input
                [ on "blur" <| Decode.map (SetInputTime startOrEnd InputMinute) targetValueIntParse
                , value
                    (case startOrEnd of
                        Start ->
                            case dateTimePicker.startTime of
                                Just t ->
                                    Time.toMinute utc t |> String.fromInt

                                Nothing ->
                                    "0"

                        End ->
                            case dateTimePicker.endTime of
                                Just t ->
                                    Time.toMinute utc t |> String.fromInt

                                Nothing ->
                                    "0"
                    )
                , maxlength 2
                , class "view"
                ]
                []
            , button
                [ class "down-button d-flex-center"
                , onClick <| IncrementTime startOrEnd InputMinute -1
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
                                    Just _ ->
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
