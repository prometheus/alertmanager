module Views.AlertList.Types exposing (AlertListMsg(..), Model, initAlertList)

import Utils.Types exposing (ApiData, ApiResponse(Loading))
import Alerts.Types exposing (Alert)
import Views.FilterBar.Types as FilterBar
import Views.GroupBar.Types as GroupBar


type AlertListMsg
    = AlertsFetched (ApiData (List Alert))
    | FetchAlerts
    | MsgForFilterBar FilterBar.Msg
    | MsgForGroupBar GroupBar.Msg


type alias Model =
    { alerts : ApiData (List Alert)
    , autoComplete : GroupBar.Model
    , filterBar : FilterBar.Model
    }


initAlertList : Model
initAlertList =
    { alerts = Loading
    , autoComplete = GroupBar.initGroupBar
    , filterBar = FilterBar.initFilterBar
    }
