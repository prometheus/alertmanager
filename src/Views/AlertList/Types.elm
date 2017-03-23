module Views.AlertList.Types exposing (AlertListMsg(..), Route(..))

import Utils.Types exposing (ApiData, Filter)
import Alerts.Types exposing (Alert, AlertGroup)


type Route
    = Receiver (Maybe String) (Maybe Bool) (Maybe String)


type AlertListMsg
    = AlertGroupsFetch (ApiData (List AlertGroup))
    | FetchAlertGroups
    | FilterAlerts
