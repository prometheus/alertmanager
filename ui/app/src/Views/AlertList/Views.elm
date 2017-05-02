module Views.AlertList.Views exposing (view)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Html exposing (..)
import Html.Attributes exposing (..)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Utils.Filter exposing (Filter)
import Utils.Types exposing (ApiResponse(Success, Loading, Failure))
import Utils.Views exposing (buttonLink, listButton)
import Views.AlertList.AlertView as AlertView
import Views.AlertList.Filter exposing (silenced, matchers)
import Views.AlertList.FilterBar
import Views.AlertList.Types exposing (AlertListMsg(AddFilterMatcher), Model)


view : Model -> Filter -> Html Msg
view { alerts, matchers, matcherText, backspacePressed } filter =
    div []
        [ Views.AlertList.FilterBar.view matchers matcherText backspacePressed
        , case alerts of
            Success groups ->
                alertList groups filter

            Loading ->
                Utils.Views.loading

            Failure msg ->
                Utils.Views.error msg
        ]


alertList : List Alert -> Filter -> Html Msg
alertList alerts filter =
    let
        filteredAlerts =
            silenced filter.showSilenced alerts
    in
        if List.isEmpty filteredAlerts then
            div [ class "mt2" ] [ text "no alerts found" ]
        else
            ul [ class "list-group" ] (List.map AlertView.view filteredAlerts)
