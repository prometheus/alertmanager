module Views.AlertList.Types exposing (AlertListMsg(..), Model, initAlertList)

import Utils.Types exposing (ApiData, ApiResponse(Loading))
import Alerts.Types exposing (Alert)
import Views.FilterBar.Types as FilterBar
import Utils.Filter exposing (Filter)
import Set exposing (Set)


type AlertListMsg
    = AlertsFetched (ApiData (List Alert))
    | FetchAlerts
    | MsgForFilterBar FilterBar.Msg


type alias Model =
    { alerts : ApiData (List Alert)
    , labelKeys : Set String
    , filterBar : FilterBar.Model
    }


initAlertList : Model
initAlertList =
    { alerts = Loading
    , labelKeys = Set.empty
    , filterBar = FilterBar.initFilterBar
    }
