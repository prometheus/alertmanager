module Views.AlertList.Updates exposing (..)

import Alerts.Api as Api
import Views.AlertList.Types exposing (AlertListMsg(..), Route(Receiver))
import Alerts.Types exposing (AlertGroup)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter)
import Utils.Filter exposing (generateQueryString)
import Types exposing (Msg(MsgForAlertList, NewUrl))


update : AlertListMsg -> ApiData (List AlertGroup) -> Filter -> ( ApiData (List AlertGroup), Cmd Types.Msg )
update msg groups filter =
    case msg of
        AlertGroupsFetch alertGroups ->
            ( alertGroups, Cmd.none )

        FetchAlertGroups ->
            ( groups, Api.getAlertGroups filter (AlertGroupsFetch >> MsgForAlertList) )

        FilterAlerts ->
            let
                url =
                    "/#/alerts" ++ generateQueryString filter
            in
                ( groups, Task.perform identity (Task.succeed (Types.NewUrl url)) )


updateFilter : Route -> Filter
updateFilter route =
    case route of
        Receiver maybeReceiver maybeShowSilenced maybeFilter ->
            { receiver = maybeReceiver
            , showSilenced = maybeShowSilenced
            , text = maybeFilter
            }


urlUpdate : Route -> ( AlertListMsg, Filter )
urlUpdate route =
    ( FetchAlertGroups, updateFilter route )
