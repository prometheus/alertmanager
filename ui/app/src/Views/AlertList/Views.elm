module Views.AlertList.Views exposing (view)

import Data.AlertGroup exposing (AlertGroup)
import Data.GettableAlert exposing (GettableAlert)
import Data.Receiver exposing (Receiver)
import Dict
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Set exposing (Set)
import Types exposing (Msg(..))
import Utils.Filter exposing (Filter)
import Utils.Types exposing (ApiData(..), Labels)
import Utils.Views
import Views.AlertList.AlertView as AlertView
import Views.AlertList.Types exposing (AlertListMsg(..), Model, Tab(..))
import Views.FilterBar.Views as FilterBar
import Views.GroupBar.Views as GroupBar
import Views.ReceiverBar.Views as ReceiverBar


renderCheckbox : String -> Maybe Bool -> (Bool -> AlertListMsg) -> Html Msg
renderCheckbox textLabel maybeChecked toggleMsg =
    let
        checkboxId = "checkbox-" ++ textLabel
    in
    li [ class "nav-item" ]
        [ div [ class "form-check mt-1 ms-1" ]
            [ input
                [ type_ "checkbox"
                , class "form-check-input"
                , id checkboxId
                , checked (Maybe.withDefault False maybeChecked)
                , onCheck (toggleMsg >> MsgForAlertList)
                ]
                []
            , label
                [ class "form-check-label ms-1"
                , for checkboxId
                ]
                [ text textLabel ]
            ]
        ]


groupTabName : Bool -> Html msg
groupTabName customGrouping =
    if customGrouping then
        text "Group (custom)"

    else
        text "Group"


view : Model -> Filter -> Html Msg
view { alertGroups, groupBar, filterBar, receiverBar, tab, activeId, activeGroups, expandAll } filter =
    div []
        [ div
            [ class "card mb-3" ]
            [ div [ class "card-header" ]
                [ ul [ class "nav nav-tabs card-header-tabs" ]
                    [ Utils.Views.tab FilterTab tab (SetTab >> MsgForAlertList) [ text "Filter" ]
                    , Utils.Views.tab GroupTab tab (SetTab >> MsgForAlertList) [ groupTabName filter.customGrouping ]
                    , receiverBar
                        |> ReceiverBar.view filter.receiver
                        |> Html.map (MsgForReceiverBar >> MsgForAlertList)
                    , renderCheckbox "Silenced" filter.showSilenced ToggleSilenced
                    , renderCheckbox "Inhibited" filter.showInhibited ToggleInhibited
                    , renderCheckbox "Muted" filter.showMuted ToggleMuted
                    ]
                ]
            , div [ class "card-body" ]
                [ case tab of
                    FilterTab ->
                        Html.map (MsgForFilterBar >> MsgForAlertList) (FilterBar.view { showSilenceButton = True } filterBar)

                    GroupTab ->
                        Html.map (MsgForGroupBar >> MsgForAlertList) (GroupBar.view groupBar filter.customGrouping)
                ]
            ]
        , div []
            [ button
                [ class "btn btn-outline-secondary border-0 me-1 mb-3"
                , onClick (MsgForAlertList (ToggleExpandAll (not expandAll)))
                ]
                (if expandAll then
                    [ i [ class "fa fa-minus me-3" ] []
                    , text "Collapse all groups"
                    ]
                 else
                    [ i [ class "fa fa-plus me-3" ] []
                    , text "Expand all groups"
                    ]
                )
            ]
        , Utils.Views.apiData (defaultAlertGroups activeId activeGroups expandAll) alertGroups
        ]


defaultAlertGroups : Maybe String -> Set Int -> Bool -> List AlertGroup -> Html Msg
defaultAlertGroups activeId activeGroups expandAll groups =
    case groups of
        [] ->
            Utils.Views.error "No alert groups found"

        [ { labels, receiver, alerts } ] ->
            let
                labels_ =
                    Dict.toList labels
            in
            alertGroup activeId (Set.singleton 0) receiver labels_ alerts 0 expandAll

        _ ->
            div [ class "ps-5" ]
                (List.indexedMap
                    (\index group ->
                        alertGroup activeId activeGroups group.receiver (Dict.toList group.labels) group.alerts index expandAll
                    )
                    groups
                )


alertGroup : Maybe String -> Set Int -> Receiver -> Labels -> List GettableAlert -> Int -> Bool -> Html Msg
alertGroup activeId activeGroups receiver labels alerts groupId expandAll =
    let
        groupActive =
            expandAll || Set.member groupId activeGroups

        labels_ =
            case labels of
                [] ->
                    [ span [ class "btn btn-secondary me-1 mb-1" ] [ text "Not grouped" ] ]

                _ ->
                    List.map
                        (\( key, value ) ->
                            div [ class "btn-group me-1 mb-1" ]
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
            expandAlertGroup groupActive groupId receiver
                |> Html.map (\msg -> MsgForAlertList (ActiveGroups msg))

        alertCount =
            List.length alerts

        alertText =
            if alertCount == 1 then
                String.fromInt alertCount ++ " alert"

            else
                String.fromInt alertCount ++ " alerts"

        alertEl =
            [ span [ class "ms-1 mb-0", style "white-space" "nowrap" ] [ text alertText ] ]
    in
    div []
        [ div [ class "mb-3" ] (expandButton :: labels_ ++ alertEl)
        , if groupActive then
            ul [ class "list-group mb-0" ] (List.map (AlertView.view labels activeId) alerts)

          else
            text ""
        ]


expandAlertGroup : Bool -> Int -> Receiver -> Html Int
expandAlertGroup expanded groupId receiver =
    let
        icon =
            if expanded then
                "fa-minus"

            else
                "fa-plus"
    in
    button
        [ onClick groupId
        , class "btn btn-outline-info border-0 me-1 mb-1"
        , style "margin-left" "-3rem"
        ]
        [ i [ class ("fa " ++ icon ++ " me-2") ] []
        , text receiver.name
        ]
