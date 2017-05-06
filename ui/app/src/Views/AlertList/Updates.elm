module Views.AlertList.Updates exposing (..)

import Alerts.Api as Api
import Views.AlertList.Types exposing (AlertListMsg(..), Model)
import Views.FilterBar.Updates as FilterBar
import Navigation
import Utils.Filter exposing (Filter, parseFilter)
import Utils.Types exposing (ApiData, ApiResponse(Loading, Success, Failure))
import Types exposing (Msg(MsgForAlertList, Noop))
import Dom
import Task
import Set
import Views.AutoComplete.Updates as AutoComplete


update : AlertListMsg -> Model -> Filter -> ( Model, Cmd Types.Msg )
update msg model filter =
    case msg of
        AlertsFetched listOfAlerts ->
            ( { model
                | alerts = listOfAlerts
                , autoComplete =
                    case listOfAlerts of
                        Success alerts ->
                            { list =
                                List.concatMap .labels alerts
                                    |> List.map Tuple.first
                                    |> Set.fromList
                            , fieldText = ""
                            , fields = []
                            }

                        Loading ->
                            model.autoComplete

                        Failure _ ->
                            model.autoComplete
              }
            , Cmd.none
            )

        FetchAlerts ->
            ( { model
                | filterBar = FilterBar.setMatchers filter model.filterBar
                , alerts = Loading
              }
            , Api.fetchAlerts filter |> Cmd.map (AlertsFetched >> MsgForAlertList)
            )

        MsgForFilterBar msg ->
            let
                ( filterBar, cmd ) =
                    FilterBar.update "/#/alerts" filter msg model.filterBar
            in
                ( { model | filterBar = filterBar }, Cmd.map (MsgForFilterBar >> MsgForAlertList) cmd )

        MsgForAutoComplete msg ->
            let
                ( autoComplete, cmd ) =
                    AutoComplete.update msg model.autoComplete
            in
                ( { model | autoComplete = autoComplete }, Cmd.map (MsgForAutoComplete >> MsgForAlertList) cmd )
