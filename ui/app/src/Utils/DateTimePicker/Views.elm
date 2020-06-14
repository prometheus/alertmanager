module Utils.DateTimePicker.Views exposing (viewDateTimePicker)

import Html exposing (Html, button, div, h5, i, input, strong, text)
import Html.Attributes exposing (class, id, maxlength, style, value)
import Html.Events exposing (on, onClick, onInput, onMouseOver)
import Iso8601
import Json.Decode as Decode exposing (Decoder)
import List
import Time exposing (Month(..), Posix, Weekday(..), Zone)
import Time.Extra as Time exposing (Interval(..))
import Utils.DateTimePicker.Types exposing (DateTimePicker(..), InputHourOrMinute(..), Model, Msg(..), Settings, StartOrEnd(..), Status(..))
import Utils.DateTimePicker.Updates exposing (update)
import Utils.DateTimePicker.Utils
    exposing
        ( dayToNameString
        , determineDateTimeRange
        , durationDayClasses
        , durationDayPickedOrBetween
        , monthData
        , monthToNameString
        , splitIntoWeeks
        , targetValueIntParse
        )


classPrefix : String
classPrefix =
    "elm-datetimepicker-duration--"


viewDateTimePicker : Settings msg -> DateTimePicker -> Html msg
viewDateTimePicker settings (DateTimePicker model) =
    case model.status of
        Open baseTime ->
            div []
                [ div [ class "modal fade show", style "display" "block" ]
                    [ div [ class "modal-dialog modal-dialog-centered" ]
                        [ div [ class "modal-content" ]
                            [ div [ class "modal-header" ]
                                [ h5 [ class "modal-title" ] [ text "Start and End Date/Time" ]
                                , button
                                    [ class "close"
                                    , onClick <| settings.internalMsg <| update settings Cancel (DateTimePicker model)
                                    ]
                                    [ text "x" ]
                                ]
                            , div [ class "modal-body" ]
                                [ viewPickerContainer settings (DateTimePicker model) baseTime ]
                            , div [ class "modal-footer" ]
                                [ button
                                    [ class "ml-2 btn btn-outline-success pull-left"
                                    , onClick <| settings.internalMsg <| update settings Cancel (DateTimePicker model)
                                    ]
                                    [ text "Cancel" ]
                                , button
                                    [ class "ml-2 btn btn-primary"
                                    , onClick <| settings.internalMsg <| update settings Close (DateTimePicker model)
                                    ]
                                    [ text "Set Date/Time" ]
                                ]
                            ]
                        ]
                    ]
                , div [ class "modal-backdrop fade show" ] []
                ]

        Closed ->
            text ""


viewPickerContainer : Settings msg -> DateTimePicker -> Posix -> Html msg
viewPickerContainer settings (DateTimePicker model) baseTime =
    let
        leftViewTime =
            Time.add Month model.pickerOffset settings.zone baseTime

        rightViewTime =
            Time.add Month (model.pickerOffset + 1) settings.zone baseTime
    in
    div
        []
        [ viewPickerHeader settings model
        , div [ class (classPrefix ++ "calendars-container") ]
            [ div
                [ id "left-container", class (classPrefix ++ "calendar") ]
                [ viewCalendar settings model leftViewTime ]
            , div [ class (classPrefix ++ "calendar-spacer") ] []
            , div
                [ id "right-container", class (classPrefix ++ "calendar") ]
                [ viewCalendar settings model rightViewTime ]
            ]
        , div [ class (classPrefix ++ "footer-container") ] [ viewFooter settings model ]
        ]


viewPickerHeader : Settings msg -> Model -> Html msg
viewPickerHeader settings model =
    div []
        [ div [ class (classPrefix ++ "picker-header-chevrons") ]
            [ div
                [ id "previous-month"
                , class (classPrefix ++ "picker-header-chevron")
                , onClick <| settings.internalMsg <| update settings PrevMonth (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-left" ]
                    []
                ]
            , div
                [ id "next-month"
                , class (classPrefix ++ "picker-header-chevron")
                , onClick <| settings.internalMsg <| update settings NextMonth (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-right" ]
                    []
                ]
            ]
        , div [ class (classPrefix ++ "picker-header-chevrons") ]
            [ div
                [ id "previous-year"
                , class (classPrefix ++ "picker-header-chevron")
                , onClick <| settings.internalMsg <| update settings PrevYear (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-double-left" ]
                    []
                ]
            , div
                [ id "next-year"
                , class (classPrefix ++ "picker-header-chevron")
                , onClick <| settings.internalMsg <| update settings NextYear (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-double-right" ]
                    []
                ]
            ]
        ]


viewCalendarHeader : Settings msg -> Posix -> Html msg
viewCalendarHeader settings viewTime =
    let
        monthName =
            Time.toMonth settings.zone viewTime |> monthToNameString

        year =
            Time.toYear settings.zone viewTime |> String.fromInt
    in
    div
        [ class (classPrefix ++ "calendar-header") ]
        [ div [ class (classPrefix ++ "calendar-header-row") ]
            [ div
                [ class (classPrefix ++ "calendar-header-text")
                ]
                [ div [ id "month" ] [ text monthName ] ]
            ]
        , div [ class (classPrefix ++ "calendar-header-row") ]
            [ div
                [ class (classPrefix ++ "calendar-header-text")
                ]
                [ div [ id "year" ] [ text year ] ]
            ]
        , viewWeekHeader settings [ Sun, Mon, Tue, Wed, Thu, Fri, Sat ]
        ]


viewWeekHeader : Settings msg -> List Weekday -> Html msg
viewWeekHeader settings days =
    div
        [ class (classPrefix ++ "calendar-header-week") ]
        (List.map (viewHeaderDay dayToNameString) days)


viewHeaderDay : (Weekday -> String) -> Weekday -> Html msg
viewHeaderDay formatDay day =
    div
        [ class (classPrefix ++ "calendar-header-day") ]
        [ text (formatDay day) ]


viewCalendar : Settings msg -> Model -> Posix -> Html msg
viewCalendar settings model viewTime =
    div
        []
        [ viewCalendarHeader settings viewTime
        , viewMonth settings model viewTime
        ]


viewMonth : Settings msg -> Model -> Posix -> Html msg
viewMonth settings model viewTime =
    let
        monthRenderData =
            monthData settings.zone viewTime

        currentMonth =
            Time.posixToParts settings.zone viewTime |> .month

        weeks =
            List.reverse (splitIntoWeeks monthRenderData [])
    in
    div
        [ class (classPrefix ++ "calendar-month") ]
        [ div [] (List.map (viewWeek settings model currentMonth) weeks)
        ]


viewWeek : Settings msg -> Model -> Month -> List Posix -> Html msg
viewWeek settings model currentMonth week =
    div [ class (classPrefix ++ "calendar-week") ]
        (List.map (viewDay settings model currentMonth) week)


viewDay : Settings msg -> Model -> Month -> Posix -> Html msg
viewDay settings model currentMonth day =
    let
        dayParts =
            Time.posixToParts settings.zone day

        ( isPicked, isBetween ) =
            durationDayPickedOrBetween settings.zone day model.hovered ( model.pickedStart, model.pickedEnd )

        dayClasses =
            durationDayClasses classPrefix isPicked isBetween

        attrs =
            [ class dayClasses
            , onClick <| settings.internalMsg (update settings SetRange (DateTimePicker model))
            , onMouseOver <| settings.internalMsg (update settings (SetHoveredDay day) (DateTimePicker model))
            ]
    in
    div
        attrs
        [ text (String.fromInt dayParts.day) ]


viewFooter : Settings msg -> Model -> Html msg
viewFooter settings model =
    let
        ( startDate, endDate ) =
            determineDateTimeRange settings.zone model.pickedStart model.pickedEnd model.hovered
    in
    div
        [ class (classPrefix ++ "footer") ]
        [ div [ class (classPrefix ++ "time-pickers-container") ]
            [ div
                [ class "container" ]
                [ viewTimePicker settings model Start startDate ]
            , div
                [ class "container" ]
                [ viewTimePicker settings model End endDate ]
            ]
        ]


viewTimePicker : Settings msg -> Model -> StartOrEnd -> Maybe Posix -> Html msg
viewTimePicker settings model startOrEnd pickedTime =
    div
        [ class "row" ]
        [ strong [ class "col-2 p-1 d-flex align-items-center" ]
            [ text
                (case startOrEnd of
                    Start ->
                        "Start"

                    End ->
                        "End"
                )
            ]
        , div [ class "col-1 p-1" ]
            [ button
                [ class "p-0 btn btn-default w-100"
                , onClick <| settings.internalMsg <| update settings (IncrementTime startOrEnd InputHour 1) (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-up" ]
                    []
                ]
            , input
                [ on "blur" (Decode.map settings.internalMsg (Decode.map (\msg -> update settings msg (DateTimePicker model)) (Decode.map (SetInputTime startOrEnd InputHour) targetValueIntParse)))
                , value
                    (case startOrEnd of
                        Start ->
                            case model.timeStart of
                                Just t ->
                                    Time.toHour settings.zone t |> String.fromInt

                                Nothing ->
                                    "0"

                        End ->
                            case model.timeEnd of
                                Just t ->
                                    Time.toHour settings.zone t |> String.fromInt

                                Nothing ->
                                    "0"
                    )
                , maxlength 2
                , class "w-100 text-center border-0"
                ]
                []
            , button
                [ class "p-0 btn btn-default w-100"
                , onClick <| settings.internalMsg <| update settings (IncrementTime startOrEnd InputHour -1) (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-down" ]
                    []
                ]
            ]
        , div [ class "col-xs-1 p-1 d-flex align-items-center" ] [ text ":" ]
        , div [ class "col-1 p-1" ]
            [ button
                [ class "p-0 btn btn-default w-100"
                , onClick <| settings.internalMsg <| update settings (IncrementTime startOrEnd InputMinute 1) (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-up" ]
                    []
                ]
            , input
                [ on "blur" (Decode.map settings.internalMsg (Decode.map (\msg -> update settings msg (DateTimePicker model)) (Decode.map (SetInputTime startOrEnd InputMinute) targetValueIntParse)))
                , value
                    (case startOrEnd of
                        Start ->
                            case model.timeStart of
                                Just t ->
                                    Time.toMinute settings.zone t |> String.fromInt

                                Nothing ->
                                    "00"

                        End ->
                            case model.timeEnd of
                                Just t ->
                                    Time.toMinute settings.zone t |> String.fromInt

                                Nothing ->
                                    "00"
                    )
                , maxlength 2
                , class "w-100 text-center border-0"
                ]
                []
            , button
                [ class "p-0 btn btn-default w-100"
                , onClick <| settings.internalMsg <| update settings (IncrementTime startOrEnd InputMinute -1) (DateTimePicker model)
                ]
                [ i
                    [ class "fa fa-angle-down" ]
                    []
                ]
            ]
        , div [ class "pl-5 col-7 p-2 d-flex align-items-center" ]
            [ text
                (let
                    picked =
                        case startOrEnd of
                            Start ->
                                model.pickedStart

                            End ->
                                model.pickedEnd
                 in
                 case picked of
                    Just time ->
                        Iso8601.fromTime time

                    Nothing ->
                        ""
                )
            ]
        ]
