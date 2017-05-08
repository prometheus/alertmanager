module Views.AlertList.Types exposing (AlertListMsg(..), Model, initAlertList)

import Utils.Types exposing (ApiData, ApiResponse(Loading))
import Alerts.Types exposing (Alert)
import Views.FilterBar.Types as FilterBar
import Utils.Filter exposing (Filter)


type AlertListMsg
    = AlertsFetched (ApiData (List Alert))
    | FetchAlerts
    | MsgForFilterBar FilterBar.Msg


type alias Model =
    { alerts : ApiData (List Alert)
    , filterBar : FilterBar.Model
    }


initAlertList : Model
initAlertList =
    { alerts = Loading
    , filterBar = FilterBar.initFilterBar
    }
