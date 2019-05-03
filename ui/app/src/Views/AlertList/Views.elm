module Views.AlertList.Views exposing (view)

import Data.AlertGroup exposing (AlertGroup)
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


groupTabName : Bool -> Html msg
groupTabName customGrouping =
    if customGrouping then
        text "Group (custom)"

    else
        text "Group"


view : Model -> Filter -> Html Msg
view { alerts, alertGroups, groupBar, filterBar, receiverBar, tab, activeId, activeLabels } filter =
    div []
        [ div
            [ class "card mb-5" ]
            [ div [ class "card-header" ]
                [ ul [ class "nav nav-tabs card-header-tabs" ]
                    [ Utils.Views.tab FilterTab tab (SetTab >> MsgForAlertList) [ text "Filter" ]
                    , Utils.Views.tab GroupTab tab (SetTab >> MsgForAlertList) [ groupTabName filter.customGrouping ]
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
                        Html.map (MsgForGroupBar >> MsgForAlertList) (GroupBar.view groupBar filter.customGrouping)
                ]
            ]
        , if filter.customGrouping then
            Utils.Views.apiData (customAlertGroups activeId activeLabels groupBar) alerts

          else
            Utils.Views.apiData (defaultAlertGroups activeId activeLabels) alertGroups
        ]


customAlertGroups : Maybe String -> Maybe Labels -> GroupBar.Model -> List GettableAlert -> Html Msg
customAlertGroups activeId activeLabels { fields } ungroupedAlerts =
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
                                    alertGroup activeId activeLabels labels alerts
                                )
                                groups
                            )
           )


defaultAlertGroups : Maybe String -> Maybe Labels -> List AlertGroup -> Html Msg
defaultAlertGroups activeId activeLabels groups =
    case groups of
        [] ->
            Utils.Views.error "No alert groups found"

        _ ->
            div []
                (List.map
                    (\{ labels, alerts } ->
                        alertGroup activeId activeLabels (Dict.toList labels) alerts
                    )
                    groups
                )


alertGroup : Maybe String -> Maybe Labels -> Labels -> List GettableAlert -> Html Msg
alertGroup activeId activeLabels labels alerts =
    let
        groupActive =
            activeLabels == Just labels
    in
    div []
        [ div []
            ((expandAlertGroup groupActive labels
                |> Html.map (\msg -> MsgForAlertList (SetGroup msg))
             )
                :: (case labels of
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
            )
        , if groupActive then
            ul [ class "list-group mb-0 ml-5" ] (List.map (AlertView.view labels activeId) alerts)

          else
            text ""
        ]


expandAlertGroup : Bool -> Labels -> Html (Maybe Labels)
expandAlertGroup expanded labels =
    if expanded then
        button
            [ onClick Nothing
            , class "btn btn-outline-info border-0 active mr-1 mb-3"
            ]
            [ i [ class "fa fa-minus" ] [] ]

    else
        button
            [ class "btn btn-outline-info border-0 mr-1 mb-3"
            , onClick (Just labels)
            ]
            [ i [ class "fa fa-plus" ] [] ]
