module Alerts.Update exposing (..)

import Alerts.Api as Api
import Alerts.Types exposing (..)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..))


update : AlertsMsg -> ApiData (List AlertGroup) -> ( ApiData (List AlertGroup), Cmd Msg )
update msg groups =
    case msg of
        AlertGroupsFetch alertGroups ->
            ( alertGroups, Cmd.none )

        FetchAlertGroups ->
            ( groups, Api.getAlertGroups )

        Noop ->
            ( groups, Cmd.none )


urlUpdate : Route -> AlertsMsg
urlUpdate _ =
    FetchAlertGroups
