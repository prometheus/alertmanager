module Alerts.Update exposing (..)

import Alerts.Types exposing (..)
import Alerts.Api as Api


update : AlertsMsg -> List AlertGroup -> ( List AlertGroup, Maybe Bool, Cmd AlertsMsg )
update msg groups =
    case msg of
        AlertGroupsFetch (Ok alertGroups) ->
            ( alertGroups, Just False, Cmd.none )

        AlertGroupsFetch (Err err) ->
            ( groups, Just False, Cmd.none )

        FetchAlertGroups ->
            ( groups, Just True, Api.getAlertGroups )

        Noop ->
            ( groups, Nothing, Cmd.none )


urlUpdate : Route -> AlertsMsg
urlUpdate _ =
    FetchAlertGroups
