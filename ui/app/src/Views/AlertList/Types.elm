module Views.AlertList.Types exposing (AlertListMsg(..))

import Utils.Types exposing (ApiData, Filter)
import Alerts.Types exposing (Alert, AlertGroup)


type AlertListMsg
    = AlertGroupsFetch (ApiData (List AlertGroup))
    | FetchAlertGroups
    | FilterAlerts
