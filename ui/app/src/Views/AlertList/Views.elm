module Views.AlertList.Views exposing (view)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Html exposing (..)
import Html.Attributes exposing (..)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Utils.Filter exposing (Filter)
import Views.FilterBar.Views as FilterBar
import Utils.Types exposing (ApiResponse(Initial, Success, Loading, Failure), Labels)
import Utils.Views exposing (buttonLink, listButton)
import Utils.List
import Views.AlertList.AlertView as AlertView
import Views.AlertList.Filter exposing (silenced, matchers)
import Utils.Views exposing (buttonLink, listButton)
import Views.AlertList.Types exposing (AlertListMsg(MsgForFilterBar, MsgForGroupBar), Model)
import Types exposing (Msg(Noop, CreateSilenceFromAlert, MsgForAlertList))
import Views.GroupBar.Views as GroupBar
import Dict exposing (Dict)


view : Model -> Filter -> Html Msg
view { alerts, groupBar, filterBar } filter =
    div []
        [ div [ class "row" ]
            [ div [ class "col" ]
                [ Html.map (MsgForFilterBar >> MsgForAlertList) (FilterBar.view filterBar)
                ]
            , div [ class "col" ]
                [ Html.map (MsgForGroupBar >> MsgForAlertList) (GroupBar.view groupBar)
                ]
            ]
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
                                            if List.member key groupBar.fields then
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
                            (Dict.keys g
                                |> List.partition ((/=) [])
                                |> uncurry (++)
                            )
                        )

            Loading ->
                Utils.Views.loading

            Initial ->
                Utils.Views.loading

            Failure msg ->
                Utils.Views.error msg
        ]


alertList : Labels -> List Alert -> Filter -> Html Msg
alertList labels alerts filter =
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
