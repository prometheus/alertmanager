module Alerts.Views exposing (view)

import Alerts.Types exposing (Alert, AlertGroup, Block, Route(..))
import Alerts.Types exposing (AlertsMsg(..), Msg(..), OutMsg(..))
import Html exposing (..)
import Html.Attributes exposing (..)
import Utils.Date
import Utils.Views exposing (..)


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
                    0

        b =
            if alert.silenced then
                buttonLink "fa-deaf" ("#/silences/" ++ toString id) "blue" (ForSelf Noop)
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


view : Route -> List AlertGroup -> Html Msg
view route alertGroups =
    let
        ( groups, emptyMessage ) =
            case route of
                Receiver maybeReceiver ->
                    case maybeReceiver of
                        Just receiver ->
                            ( filterByReceiver receiver alertGroups, "no receivers matching " ++ receiver )

                        Nothing ->
                            ( alertGroups, "" )
    in
        if List.isEmpty groups then
            div [] [ text emptyMessage ]
        else
            ul
                [ classList
                    [ ( "list", True )
                    , ( "pa0", True )
                    ]
                ]
                (List.map alertGroupView groups)


filterByReceiver : String -> List AlertGroup -> List AlertGroup
filterByReceiver receiver groups =
    List.filterMap (filterAlertGroup receiver) groups


filterAlertGroup : String -> AlertGroup -> Maybe AlertGroup
filterAlertGroup receiver alertGroup =
    let
        blocks =
            List.filter (\b -> receiver == b.routeOpts.receiver) alertGroup.blocks
    in
        if not <| List.isEmpty blocks then
            Just { alertGroup | blocks = blocks }
        else
            Nothing
