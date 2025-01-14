module Utils.DateTimePicker.Views exposing (viewDateTimePicker)

import Html exposing (Html, br, button, div, i, input, p, strong, text)
import Html.Attributes as Attr exposing (class, value, type_, maxlength)
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
    div [ class "container" ]
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
    div [ class "row" ]
        [ div [ class "col-12" ]
            [ viewMonthHeader justViewTime
            , viewMonth dateTimePicker justViewTime
            ]
        ]


viewMonthHeader : Posix -> Html Msg
viewMonthHeader justViewTime =
    div [ class "row align-items-center mb-3 month-header" ]
        [ div
            [ class "col text-start prev-month"
            , onClick PrevMonth
            ]
            [ button [ class "btn" ]
                [ i [ class "fa fa-angle-left fa-3x" ] [] ]
            ]
        , div
            [ class "col text-center month-text" ]
            [ text (Time.toYear utc justViewTime |> String.fromInt)
            , br [] []
            , text (Time.toMonth utc justViewTime |> monthToString)
            ]
        , div
            [ class "col text-end next-month"
            , onClick NextMonth
            ]
            [ button [ class "btn" ]
                [ i [ class "fa fa-angle-right fa-3x" ] [] ]
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
    div []
        [ div [ class "row mb-2" ]
            (case dateTimePicker.firstDayOfWeek of
                Sunday ->
                    List.map viewWeekHeader [ "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat" ]

                Monday ->
                    List.map viewWeekHeader [ "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun" ]
            )
        , div
            [ onMouseOut ClearMouseOverDay ]
            (List.map (viewWeek dateTimePicker justViewTime) weeks)
        ]


viewWeekHeader : String -> Html Msg
viewWeekHeader weekday =
    div [ class "col text-center text-muted" ]
        [ text weekday ]


viewWeek : DateTimePicker -> Posix -> List Posix -> Html Msg
viewWeek dateTimePicker justViewTime days =
    div [ class "row" ]
        (List.map (\day -> div [ class "col p-1" ] [ viewDay dateTimePicker justViewTime day ]) days)


viewDay : DateTimePicker -> Posix -> Posix -> Html Msg
viewDay dateTimePicker justViewTime day =
    let
        compareDate_ : Posix -> Posix -> Order
        compareDate_ a b =
            compare (floorDate a |> Time.posixToMillis)
                (floorDate b |> Time.posixToMillis)

        isSameDate maybeDate =
            Maybe.map (\m -> compareDate_ m day == EQ) maybeDate
                |> Maybe.withDefault False

        thisMonth =
            floorMonth justViewTime == floorMonth day

        between =
            case ( dateTimePicker.startDate, dateTimePicker.endDate ) of
                ( Just start, Just end ) ->
                    compareDate_ start day == LT && compareDate_ end day == GT

                _ ->
                    False

        classes =
            [ "btn", "w-100", "h-100", "text-center" ]
                ++
                (if isSameDate dateTimePicker.startDate then
                    [ "btn-primary" ]
                 else if isSameDate dateTimePicker.endDate then
                    [ "btn-primary" ]
                 else if between then
                    [ "btn-outline-secondary" ]
                 else
                    [ "btn-light" ])
                ++
                (if not thisMonth then
                    [ "text-muted" ]
                 else
                    [])
    in
    button
        [ class (String.join " " classes)
        , onMouseOver <| MouseOverDay day
        , onClick <| OnClickDay
        ]
        [ text (Time.toDay utc day |> String.fromInt) ]


viewTimePicker : DateTimePicker -> StartOrEnd -> Html Msg
viewTimePicker dateTimePicker startOrEnd =
    div
        [ class "col-12 col-md-6 mb-3" ]
        [ strong [ class "d-block mb-2" ]
            [ text
                (case startOrEnd of
                    Start ->
                        "Start"

                    End ->
                        "End"
                )
            ]
        , div [ class "input-group" ]
            [ button
                [ class "btn btn-sm"
                , onClick <| IncrementTime startOrEnd InputHour 1
                ]
                [ i [ class "fa fa-angle-up" ] [] ]
            , input
                [ Attr.type_ "number"
                , on "blur" (Decode.map (SetInputTime startOrEnd InputHour) targetValueIntParse)
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
                , Attr.maxlength 2
                , class "form-control text-center"
                , Attr.min (String.fromInt 0)
                , Attr.max (String.fromInt 23)
                ]
                []
            , button
                [ class "btn btn-sm"
                , onClick <| IncrementTime startOrEnd InputHour -1
                ]
                [ i [ class "fa fa-angle-down" ] [] ]
            , div [ class "input-group-text" ] [ text ":" ]
            , button
                [ class "btn btn-sm"
                , onClick <| IncrementTime startOrEnd InputMinute 1
                ]
                [ i [ class "fa fa-angle-up" ] [] ]
            , input
                [ Attr.type_ "number"
                , on "blur" (Decode.map (SetInputTime startOrEnd InputMinute) targetValueIntParse)
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
                , Attr.maxlength 2
                , class "form-control text-center"
                , Attr.min (String.fromInt 0)
                , Attr.max (String.fromInt 59)
                ]
                []
            , button
                [ class "btn btn-sm"
                , onClick <| IncrementTime startOrEnd InputMinute -1
                ]
                [ i [ class "fa fa-angle-down" ] [] ]
            ]
        , div [ class "mt-2 text-center" ]
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
