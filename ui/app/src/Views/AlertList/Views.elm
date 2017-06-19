module Views.AlertList.Views exposing (view)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Utils.Filter exposing (Filter)
import Views.FilterBar.Views as FilterBar
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
view { alerts, groupBar, filterBar, receivers, showRecievers, tab, activeId } filter =
    div []
        [ div
            [ class "card mb-5" ]
            [ div [ class "card-header" ]
                [ ul [ class "nav nav-tabs card-header-tabs" ]
                    [ Utils.Views.tab FilterTab tab (SetTab >> MsgForAlertList) [ text "Filter" ]
                    , Utils.Views.tab GroupTab tab (SetTab >> MsgForAlertList) [ text "Group" ]
                    , renderReceivers filter.receiver receivers showRecievers
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
alertGroups activeId filter groupBar alerts =
    let
        grouped =
            alerts
                |> Utils.List.groupBy
                    (.labels >> List.filter (\( key, _ ) -> List.member key groupBar.fields))
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
        , if List.isEmpty alerts then
            div [] [ text "no alerts found" ]
          else
            ul [ class "list-group mb-4" ] (List.map (AlertView.view labels activeId) alerts)
        ]


renderReceivers : Maybe String -> List String -> Bool -> Html Msg
renderReceivers receiver receivers opened =
    let
        autoCompleteClass =
            if opened then
                "show"
            else
                ""

        navLinkClass =
            if opened then
                "active"
            else
                ""
    in
        li
            [ class ("nav-item ml-auto autocomplete-menu " ++ autoCompleteClass)
            , onBlur (ToggleReceivers False |> MsgForAlertList)
            , tabindex 1
            , style
                [ ( "position", "relative" )
                , ( "outline", "none" )
                ]
            ]
            [ div
                [ onClick (ToggleReceivers (not opened) |> MsgForAlertList)
                , class "mt-1 mr-4"
                , style [ ( "cursor", "pointer" ) ]
                ]
                [ text ("Receiver: " ++ Maybe.withDefault "All" receiver) ]
            , receivers
                |> List.map Just
                |> (::) Nothing
                |> List.map (receiverField receiver)
                |> div [ class "dropdown-menu dropdown-menu-right" ]
            ]


receiverField : Maybe String -> Maybe String -> Html Msg
receiverField selected maybeReceiver =
    let
        attrs =
            if selected == maybeReceiver then
                [ class "dropdown-item active" ]
            else
                [ class "dropdown-item"
                , style [ ( "cursor", "pointer" ) ]
                , onClick (SelectReceiver maybeReceiver |> MsgForAlertList)
                ]
    in
        div
            attrs
            [ text (Maybe.withDefault "All" maybeReceiver) ]
