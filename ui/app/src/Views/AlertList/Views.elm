module Views.AlertList.Views exposing (view)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Html exposing (..)
import Html.Attributes exposing (..)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Utils.Filter exposing (Filter)
import Views.FilterBar.Views as FilterBar
import Views.FilterBar.Types as FilterBarTypes
import Utils.Types exposing (ApiResponse(Success, Loading, Failure), Labels)
import Utils.Views exposing (buttonLink, listButton)
import Utils.List
import Views.AlertList.AlertView as AlertView
import Views.AlertList.Filter exposing (silenced, matchers)
import Utils.Views exposing (buttonLink, listButton)
import Views.AlertList.Types exposing (AlertListMsg(MsgForFilterBar, MsgForAutoComplete), Model)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Views.AutoComplete.Views as AutoComplete
import Dict exposing (Dict)


view : Model -> Filter -> Html Msg
view { alerts, autoComplete, filterBar } filter =
    div []
        [ Html.map (MsgForFilterBar >> MsgForAlertList) (FilterBar.view filterBar)
        , Html.map (MsgForAutoComplete >> MsgForAlertList) (AutoComplete.view autoComplete)
        , case alerts of
            Success groups ->
                let
                    g =
                        silenced filter.showSilenced groups
                            |> Utils.List.groupBy
                                (\alert ->
                                    List.filterMap
                                        (\( key, value ) ->
                                            -- Find the correct keys and return
                                            -- their values
                                            if List.member key autoComplete.fields then
                                                Just ( key, value )
                                            else
                                                Nothing
                                        )
                                        alert.labels
                                )
                in
                    div []
                        (List.filterMap
                            (\k ->
                                let
                                    alerts =
                                        (Dict.get k g)
                                in
                                    case alerts of
                                        Just alts ->
                                            Just (alertList k alts filter)

                                        Nothing ->
                                            Nothing
                            )
                            (Dict.keys g)
                        )

            Loading ->
                Utils.Views.loading

            Failure msg ->
                Utils.Views.error msg
        ]


alertList : Labels -> List Alert -> Filter -> Html Msg
alertList labels alerts filter =
    (List.map
        (\( key, value ) ->
            span [ class "badge badge-info mr-1 mb-1" ] [ text (key ++ ":" ++ value) ]
        )
        labels
    )
        ++ [ if List.isEmpty alerts then
                div [] [ text "no alerts found" ]
             else
                ul [ class "list-group" ] (List.map AlertView.view alerts)
           ]
        |> div []
