module Alerts.Views exposing (view)

import Alerts.Types exposing (Alert, AlertGroup, Block, Route(..))
import Alerts.Types exposing (AlertsMsg(..), Msg(..), OutMsg(..))
import Alerts.Filter exposing (silenced, receiver, matchers)
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick)
import Utils.Date
import Utils.Types exposing (Filter)
import Utils.Views exposing (..)


view : Route -> List AlertGroup -> Filter -> Html Msg
view route alertGroups filter =
    let
        filteredGroups =
            case route of
                Receiver maybeReceiver maybeShowSilenced maybeFilter ->
                    receiver maybeReceiver alertGroups
                        |> silenced maybeShowSilenced
                        |> matchers filter.matchers

        filterText =
            Maybe.withDefault "" filter.text

        alertHtml =
            if List.isEmpty filteredGroups then
                div [ class "mt2" ] [ text "no alerts found found" ]
            else
                ul
                    [ classList
                        [ ( "list", True )
                        , ( "pa0", True )
                        ]
                    ]
                    (List.map alertGroupView filteredGroups)
    in
        div []
            [ Html.map ForParent (textField "Filter" filterText (UpdateFilter filter))
            , a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", onClick (ForSelf FilterAlerts) ] [ text "Filter Alerts" ]
            , alertHtml
            ]


alertGroupView : AlertGroup -> Html Msg
alertGroupView alertGroup =
    li [ class "pa3 pa4-ns bb b--black-10" ]
        [ div [ class "mb3" ] (List.map alertHeader <| List.sort alertGroup.labels)
        , div [] (List.map blockView alertGroup.blocks)
        ]


blockView : Block -> Html Msg
blockView block =
    div [] (List.map alertView block.alerts)


alertView : Alert -> Html Msg
alertView alert =
    let
        id =
            case alert.silenceId of
                Just id ->
                    id

                Nothing ->
                    ""

        b =
            if alert.silenced then
                buttonLink "fa-deaf" ("#/silences/" ++ id) "blue" (ForSelf Noop)
            else
                buttonLink "fa-exclamation-triangle" "#/silences/new" "dark-red" (ForParent (SilenceFromAlert alert))
    in
        div [ class "f6 mb3" ]
            [ div [ class "mb1" ]
                [ b
                , buttonLink "fa-bar-chart" alert.generatorUrl "black" (ForSelf Noop)
                , p [ class "dib mr2" ] [ text <| Utils.Date.dateFormat alert.startsAt ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map labelButton <| List.filter (\( k, v ) -> k /= "alertname") alert.labels)
            ]


alertHeader : ( String, String ) -> Html Msg
alertHeader ( key, value ) =
    if key == "alertname" then
        b [ class "db f4 mr2 dark-red dib" ] [ text value ]
    else
        listButton "ph1 pv1" ( key, value )
