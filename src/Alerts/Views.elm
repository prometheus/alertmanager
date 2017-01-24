module Alerts.Views exposing (..)

import Alerts.Types exposing (AlertGroup, Block, Alert, AlertsMsg(Noop, SendAlert), Route)
import Html exposing (..)
import Html.Attributes exposing (..)
import Utils.Date
import Utils.Views exposing (..)


alertGroupView : AlertGroup -> Html AlertsMsg
alertGroupView alertGroup =
    li [ class "pa3 pa4-ns bb b--black-10" ]
        [ div [ class "mb3" ] (List.map alertHeader <| List.sort alertGroup.labels)
        , div [] (List.map blockView alertGroup.blocks)
        ]


blockView : Block -> Html AlertsMsg
blockView block =
    -- Block level
    div []
        (List.map alertView block.alerts)


alertView : Alert -> Html AlertsMsg
alertView alert =
    let
        id =
            case alert.silenceId of
                Just id ->
                    id

                Nothing ->
                    0

        b =
            if alert.silenced then
                buttonLink "fa-deaf" ("#/silences/" ++ toString id) "blue" Noop
            else
                buttonLink "fa-exclamation-triangle" "#/silences/new" "dark-red" (SendAlert alert)
    in
        div [ class "f6 mb3" ]
            [ div [ class "mb1" ]
                [ b
                , buttonLink "fa-bar-chart" alert.generatorUrl "black" Noop
                , p [ class "dib mr2" ] [ text <| Utils.Date.dateFormat alert.startsAt ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map labelButton <| List.filter (\( k, v ) -> k /= "alertname") alert.labels)
            ]


alertHeader : ( String, String ) -> Html AlertsMsg
alertHeader ( key, value ) =
    if key == "alertname" then
        b [ class "db f4 mr2 dark-red dib" ] [ text value ]
    else
        listButton "ph1 pv1" ( key, value )


view : Route -> List AlertGroup -> Html AlertsMsg
view _ alertGroups =
    ul
        [ classList
            [ ( "list", True )
            , ( "pa0", True )
            ]
        ]
        (List.map alertGroupView alertGroups)
