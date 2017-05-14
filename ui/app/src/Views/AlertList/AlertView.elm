module Views.AlertList.AlertView exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (..)
import Html.Attributes exposing (class, style, href)
import Html.Events exposing (onClick)
import Types exposing (Msg(CreateSilenceFromAlert, Noop, MsgForAlertList))
import Utils.Date
import Views.FilterBar.Types as FilterBarTypes
import Views.AlertList.Types exposing (AlertListMsg(MsgForFilterBar))
import Utils.Filter
import Time exposing (Time)


view : List ( String, String ) -> Alert -> Html Msg
view labels alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> List.filter ((flip List.member) labels >> not)
                |> List.partition (Tuple.first >> (==) "alertname")
                |> uncurry (++)
    in
        li
            [ class "align-items-start list-group-item border-0 alert-list-item p-0 mb-4"
            ]
            [ div [ class "w-100 mb-2 d-flex align-items-start" ]
                [ dateView alert.startsAt
                , generatorUrlButton alert.generatorUrl
                , silenceButton alert
                ]
            , div [] (List.map labelButton ungroupedLabels)
            ]


dateView : Time -> Html Msg
dateView time =
    span
        [ class "text-muted align-self-center mr-2"
        ]
        [ text (Utils.Date.timeFormat time ++ ", " ++ Utils.Date.dateFormat time)
        ]


nameLabelButton : ( String, String ) -> Html Msg
nameLabelButton ( key, value ) =
    button
        [ class "btn btn-outline-info mr-2"
        , onClick (addLabelMsg ( key, value ))
        ]
        [ text value ]


labelButton : ( String, String ) -> Html Msg
labelButton ( key, value ) =
    button
        [ class "btn btn-sm bg-faded btn-secondary border-0 mr-2 mb-2"
        , onClick (addLabelMsg ( key, value ))
        ]
        [ span [ class "text-muted" ] [ text (key ++ "=\"" ++ value ++ "\"") ] ]


addLabelMsg : ( String, String ) -> Msg
addLabelMsg ( key, value ) =
    (FilterBarTypes.AddFilterMatcher False
        { key = key
        , op = Utils.Filter.Eq
        , value = value
        }
        |> MsgForFilterBar
        |> MsgForAlertList
    )


silenceButton : Alert -> Html Msg
silenceButton alert =
    case alert.silenceId of
        Just sId ->
            a
                [ class "btn btn-outline-danger border-0"
                , href ("#/silences/" ++ sId)
                , onClick (CreateSilenceFromAlert alert)
                ]
                [ i [ class "fa fa-bell-slash mr-2" ] []
                , text "Silenced"
                ]

        Nothing ->
            a
                [ class "btn btn-outline-info border-0"
                , href "#/silences/new?keep=1"
                , onClick (CreateSilenceFromAlert alert)
                ]
                [ i [ class "fa fa-bell-slash-o mr-2" ] []
                , text "Silence"
                ]


generatorUrlButton : String -> Html Msg
generatorUrlButton url =
    a
        [ class "btn btn-outline-info border-0", href url ]
        [ i [ class "fa fa-line-chart mr-2" ] []
        , text "Source"
        ]
