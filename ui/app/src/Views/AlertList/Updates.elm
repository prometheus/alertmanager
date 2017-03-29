module Views.AlertList.Updates exposing (..)

import Alerts.Api as Api
import Views.AlertList.Types exposing (AlertListMsg(..))
import Alerts.Types exposing (AlertGroup)
import Navigation
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Utils.Filter exposing (generateQueryString)
import Types exposing (Msg(MsgForAlertList))


update : AlertListMsg -> ApiData (List AlertGroup) -> Filter -> ( ApiData (List AlertGroup), Cmd Types.Msg )
update msg groups filter =
    case msg of
        AlertGroupsFetch alertGroups ->
            ( alertGroups, Cmd.none )

        FetchAlertGroups ->
            ( groups, Api.alertGroups filter |> Cmd.map (AlertGroupsFetch >> MsgForAlertList) )

        FilterAlerts ->
            ( groups, Navigation.newUrl ("/#/alerts" ++ generateQueryString filter) )
