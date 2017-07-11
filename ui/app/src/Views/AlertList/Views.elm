module Views.AlertList.Views exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Utils.Filter exposing (Filter)
import Views.FilterBar.Views as FilterBar
import Views.ReceiverBar.Views as ReceiverBar
import Utils.Types exposing (ApiData(Initial, Success, Loading, Failure), Labels)
import Utils.Views
import Utils.List
import Views.AlertList.AlertView as AlertView
import Views.GroupBar.Types as GroupBar
import Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(..))
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Views.GroupBar.Views as GroupBar
import Dict exposing (Dict)


renderSilenced : Maybe Bool -> Html Msg
renderSilenced maybeShowSilenced =
    li [ class "nav-item" ]
        [ label [ class "mt-1 custom-control custom-checkbox" ]
            [ input
                [ type_ "checkbox"
                , class "custom-control-input"
                , checked (Maybe.withDefault False maybeShowSilenced)
                , onCheck (ToggleSilenced >> MsgForAlertList)
                ]
                []
            , span [ class "custom-control-indicator" ] []
            , span [ class "custom-control-description" ] [ text "Show Silenced" ]
            ]
        ]


view : Model -> Filter -> Html Msg
view { alerts, groupBar, filterBar, receiverBar, tab, activeId } filter =
    div []
        [ div
            [ class "card mb-5" ]
            [ div [ class "card-header" ]
                [ ul [ class "nav nav-tabs card-header-tabs" ]
                    [ Utils.Views.tab FilterTab tab (SetTab >> MsgForAlertList) [ text "Filter" ]
                    , Utils.Views.tab GroupTab tab (SetTab >> MsgForAlertList) [ text "Group" ]
                    , ReceiverBar.view filter.receiver receiverBar |> Html.map (MsgForReceiverBar >> MsgForAlertList)
                    , renderSilenced filter.showSilenced
                    ]
                ]
            , div [ class "card-block" ]
                [ case tab of
                    FilterTab ->
                        Html.map (MsgForFilterBar >> MsgForAlertList) (FilterBar.view filterBar)

                    GroupTab ->
                        Html.map (MsgForGroupBar >> MsgForAlertList) (GroupBar.view groupBar)
                ]
            ]
        , case alerts of
            Success alerts ->
                alertGroups activeId filter groupBar alerts

            Loading ->
                Utils.Views.loading

            Initial ->
                Utils.Views.loading

            Failure msg ->
                Utils.Views.error msg
        ]


alertGroups : Maybe String -> Filter -> GroupBar.Model -> List Alert -> Html Msg
alertGroups activeId filter { fields } alerts =
    let
        grouped =
            alerts
                |> Utils.List.groupBy
                    (.labels >> List.filter (\( key, _ ) -> List.member key fields))
    in
        grouped
            |> Dict.keys
            |> List.partition ((/=) [])
            |> uncurry (++)
            |> List.filterMap
                (\labels ->
                    Maybe.map
                        (alertList activeId labels filter)
                        (Dict.get labels grouped)
                )
            |> (\list ->
                    if List.isEmpty list then
                        [ Utils.Views.error "No alerts found" ]
                    else
                        list
               )
            |> div []


alertList : Maybe String -> Labels -> Filter -> List Alert -> Html Msg
alertList activeId labels filter alerts =
    div []
        [ div []
            (case labels of
                [] ->
                    [ span [ class "btn btn-secondary mr-1 mb-3" ] [ text "Not grouped" ] ]

                _ ->
                    List.map
                        (\( key, value ) ->
                            span [ class "btn btn-info mr-1 mb-3" ]
                                [ text (key ++ "=\"" ++ value ++ "\"") ]
                        )
                        labels
            )
        , ul [ class "list-group mb-4" ] (List.map (AlertView.view labels activeId) alerts)
        ]
