module Views.AlertList.Views exposing (view)

import Data.GettableAlert exposing (GettableAlert)
import Dict exposing (Dict)
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Types exposing (Msg(..))
import Utils.Filter exposing (Filter)
import Utils.List
import Utils.Types exposing (ApiData(..), Labels)
import Utils.Views
import Views.AlertList.AlertView as AlertView
import Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(..))
import Views.FilterBar.Views as FilterBar
import Views.GroupBar.Types as GroupBar
import Views.GroupBar.Views as GroupBar
import Views.ReceiverBar.Views as ReceiverBar


renderCheckbox : String -> Maybe Bool -> (Bool -> AlertListMsg) -> Html Msg
renderCheckbox textLabel maybeShowSilenced toggleMsg =
    li [ class "nav-item" ]
        [ label [ class "mt-1 ml-1 custom-control custom-checkbox" ]
            [ input
                [ type_ "checkbox"
                , class "custom-control-input"
                , checked (Maybe.withDefault False maybeShowSilenced)
                , onCheck (toggleMsg >> MsgForAlertList)
                ]
                []
            , span [ class "custom-control-indicator" ] []
            , span [ class "custom-control-description" ] [ text textLabel ]
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
                    , receiverBar
                        |> ReceiverBar.view filter.receiver
                        |> Html.map (MsgForReceiverBar >> MsgForAlertList)
                    , renderCheckbox "Silenced" filter.showSilenced ToggleSilenced
                    , renderCheckbox "Inhibited" filter.showInhibited ToggleInhibited
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
        , Utils.Views.apiData (customAlertGroups activeId groupBar) alerts
        ]


customAlertGroups : Maybe String -> GroupBar.Model -> List GettableAlert -> Html Msg
customAlertGroups activeId { fields } ungroupedAlerts =
    ungroupedAlerts
        |> Utils.List.groupBy
            (.labels >> Dict.toList >> List.filter (\( key, _ ) -> List.member key fields))
        |> (\groupsDict ->
                case Dict.toList groupsDict of
                    [] ->
                        Utils.Views.error "No alerts found"

                    groups ->
                        div []
                            (List.map
                                (\( labels, alerts ) ->
                                    alertGroup activeId labels alerts
                                )
                                groups
                            )
           )


alertGroup : Maybe String -> Labels -> List GettableAlert -> Html Msg
alertGroup activeId labels alerts =
    div []
        [ div []
            (case labels of
                [] ->
                    [ span [ class "btn btn-secondary mr-1 mb-3" ] [ text "Not grouped" ] ]

                _ ->
                    List.map
                        (\( key, value ) ->
                            div [ class "btn-group mr-1 mb-3" ]
                                [ span
                                    [ class "btn text-muted"
                                    , style "user-select" "initial"
                                    , style "-moz-user-select" "initial"
                                    , style "-webkit-user-select" "initial"
                                    , style "border-color" "#5bc0de"
                                    ]
                                    [ text (key ++ "=\"" ++ value ++ "\"") ]
                                , button
                                    [ class "btn btn-outline-info"
                                    , onClick (AlertView.addLabelMsg ( key, value ))
                                    , title "Filter by this label"
                                    ]
                                    [ text "+" ]
                                ]
                        )
                        labels
            )
        , ul [ class "list-group mb-4" ] (List.map (AlertView.view labels activeId) alerts)
        ]
