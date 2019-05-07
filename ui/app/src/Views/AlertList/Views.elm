module Views.AlertList.Views exposing (view)

import Data.AlertGroup exposing (AlertGroup)
import Data.GettableAlert exposing (GettableAlert)
import Dict exposing (Dict)
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Set exposing (Set)
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
view { alerts, alertGroups, groupBar, filterBar, receiverBar, tab, activeId, activeGroups } filter =
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
        , Utils.Views.apiData (defaultAlertGroups activeId activeGroups) alertGroups
        ]


defaultAlertGroups : Maybe String -> Set Labels -> List AlertGroup -> Html Msg
defaultAlertGroups activeId activeGroups groups =
    case groups of
        [] ->
            Utils.Views.error "No alert groups found"

        [ { labels, alerts } ] ->
            let
                labels_ =
                    Dict.toList labels
            in
            alertGroup activeId (Set.singleton labels_) labels_ alerts

        _ ->
            div []
                (List.map
                    (\{ labels, alerts } ->
                        alertGroup activeId activeGroups (Dict.toList labels) alerts
                    )
                    groups
                )


alertGroup : Maybe String -> Set Labels -> Labels -> List GettableAlert -> Html Msg
alertGroup activeId activeGroups labels alerts =
    let
        groupActive =
            Set.member labels activeGroups

        labels_ =
            case labels of
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

        expandButton =
            expandAlertGroup groupActive labels
                |> Html.map (\msg -> MsgForAlertList (ActiveGroups msg))

        alertCount =
            List.length alerts

        alertText =
            if alertCount == 1 then
                String.fromInt alertCount ++ " alert"

            else
                String.fromInt alertCount ++ " alerts"

        alertEl =
            [ span [ class "ml-1 mb-0" ] [ text alertText ] ]
    in
    div []
        [ div [] (expandButton :: labels_ ++ alertEl)
        , if groupActive then
            ul [ class "list-group mb-0 ml-5" ] (List.map (AlertView.view labels activeId) alerts)

          else
            text ""
        ]


expandAlertGroup : Bool -> Labels -> Html Labels
expandAlertGroup expanded labels =
    let
        icon =
            if expanded then
                "fa-minus"

            else
                "fa-plus"
    in
    button
        [ onClick labels
        , class "btn btn-outline-info border-0 active mr-1 mb-3"
        ]
        [ i [ class ("fa " ++ icon) ] [] ]
