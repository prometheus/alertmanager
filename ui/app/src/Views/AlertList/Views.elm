module Views.AlertList.Views exposing (view)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Utils.Filter exposing (Filter)
import Views.FilterBar.Views as FilterBar
import Utils.Types exposing (ApiResponse(Initial, Success, Loading, Failure), Labels)
import Utils.Views exposing (buttonLink, listButton)
import Utils.List
import Views.AlertList.AlertView as AlertView
import Views.AlertList.Filter exposing (silenced, matchers)
import Views.GroupBar.Types as GroupBar
import Utils.Views exposing (buttonLink, listButton)
import Views.AlertList.Types exposing (AlertListMsg(MsgForFilterBar, MsgForGroupBar, SetTab, ToggleSilenced), Model, Tab(..))
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Views.GroupBar.Views as GroupBar
import Dict exposing (Dict)


renderTab : String -> Tab -> Tab -> Html Msg
renderTab title tab currentTab =
    li [ class "nav-item" ]
        [ (if tab == currentTab then
            span
                [ class "nav-link active" ]
           else
            a
                [ class "nav-link"
                , onClick (SetTab tab |> MsgForAlertList)
                ]
          )
            [ text title ]
        ]


renderSilenced : Maybe Bool -> Html Msg
renderSilenced maybeShowSilenced =
    li [ class "nav-item ml-auto " ]
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
view { alerts, groupBar, filterBar, tab } filter =
    div []
        [ div
            [ class "card mb-5" ]
            [ div [ class "card-header" ]
                [ ul [ class "nav nav-tabs card-header-tabs" ]
                    [ renderTab "Filter" FilterTab tab
                    , renderTab "Group" GroupTab tab
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
                alertGroups filter groupBar alerts

            Loading ->
                Utils.Views.loading

            Initial ->
                Utils.Views.loading

            Failure msg ->
                Utils.Views.error msg
        ]


alertGroups : Filter -> GroupBar.Model -> List Alert -> Html Msg
alertGroups filter groupBar alerts =
    let
        grouped =
            alerts
                |> silenced filter.showSilenced
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
                        (alertList labels filter)
                        (Dict.get labels grouped)
                )
            |> div []


alertList : Labels -> Filter -> List Alert -> Html Msg
alertList labels filter alerts =
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
            ul [ class "list-group mb-4" ] (List.map (AlertView.view labels) alerts)
        ]
